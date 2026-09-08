package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// Consultation audio never leaves the clinic machine: it is stored on local disk and,
// if a transcription command is configured (System -> Clinic), transcribed by shelling
// out to that local, doctor-configured tool. No audio or transcript is ever sent to a
// third-party or cloud service by this server.
const maxRecordingBytes = 100 << 20 // 100 MB (~decent length at typical browser opus bitrates)

var allowedRecordingTypes = map[string]bool{
	"audio/webm": true, "audio/ogg": true, "audio/mp4": true, "audio/mpeg": true, "audio/wav": true, "audio/x-wav": true,
}

func (s *Server) registerRecordingRoutes(r chi.Router) {
	r.Get("/encounters/{id}/recordings", s.handleRecordingsList)
	r.Post("/encounters/{id}/recordings", s.handleRecordingCreate)
	r.Get("/recordings/{id}/audio", s.handleRecordingAudio)
	r.Put("/recordings/{id}/transcript", s.handleRecordingTranscriptUpdate)
	r.With(s.requireDoctor).Delete("/recordings/{id}", s.handleRecordingArchive)
}

func (s *Server) handleRecordingsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT cr.id,cr.media_type,cr.size_bytes,cr.duration_seconds,cr.consent_confirmed,cr.transcript_status,COALESCE(cr.transcript_text,''),cr.transcript_edited,COALESCE(cr.transcript_error,''),u.display_name,cr.created_at
		FROM consultation_recordings cr JOIN users u ON u.id=cr.created_by WHERE cr.encounter_id=? AND cr.archived_at IS NULL ORDER BY cr.created_at`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECORDING_LIST_FAILED", "Could not load consultation recordings.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, mediaType, status, transcript, transcriptError, createdBy, createdAt string
		var size, duration int64
		var consent, edited bool
		if err := rows.Scan(&id, &mediaType, &size, &duration, &consent, &status, &transcript, &edited, &transcriptError, &createdBy, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "RECORDING_LIST_FAILED", "Could not load consultation recordings.")
			return
		}
		items = append(items, map[string]any{"id": id, "mediaType": mediaType, "sizeBytes": size, "durationSeconds": duration, "consentConfirmed": consent, "transcriptStatus": status, "transcriptText": transcript, "transcriptEdited": edited, "transcriptError": transcriptError, "createdBy": createdBy, "createdAt": createdAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRecordingCreate(w http.ResponseWriter, r *http.Request) {
	encounterID := chi.URLParam(r, "id")
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=? AND archived_at IS NULL", encounterID).Scan(&status); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	if status == "finalized" {
		writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "This consultation is finalized and locked.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRecordingBytes)
	if err := r.ParseMultipartForm(maxRecordingBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "RECORDING_TOO_LARGE", "Recording must be 100 MB or smaller.")
		return
	}
	if r.FormValue("consentConfirmed") != "true" {
		writeError(w, http.StatusUnprocessableEntity, "CONSENT_REQUIRED", "Patient consent must be confirmed before recording is saved.")
		return
	}
	duration, _ := strconv.Atoi(r.FormValue("durationSeconds"))
	file, header, err := r.FormFile("audio")
	if err != nil {
		writeError(w, http.StatusBadRequest, "AUDIO_REQUIRED", "An audio recording is required.")
		return
	}
	defer file.Close()
	prefix := make([]byte, 512)
	prefixLength, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_RECORDING", "The recording could not be read.")
		return
	}
	prefix = prefix[:prefixLength]
	mediaType := header.Header.Get("Content-Type")
	if idx := strings.Index(mediaType, ";"); idx >= 0 {
		mediaType = mediaType[:idx]
	}
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))
	if !allowedRecordingTypes[mediaType] {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_RECORDING_TYPE", "Supported audio formats are WebM, Ogg, MP4, MP3 and WAV.")
		return
	}
	extension := map[string]string{"audio/webm": ".webm", "audio/ogg": ".ogg", "audio/mp4": ".m4a", "audio/mpeg": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav"}[mediaType]
	recordingsDir := filepath.Join(s.config.DataDir, "documents", "recordings")
	if err := os.MkdirAll(recordingsDir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "RECORDING_STORE_FAILED", "Could not prepare recording storage.")
		return
	}
	storageName := uuid.NewString() + extension
	path := filepath.Join(recordingsDir, storageName)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECORDING_STORE_FAILED", "Could not store the recording.")
		return
	}
	hash := sha256.New()
	content := io.MultiReader(bytes.NewReader(prefix), file)
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(content, maxRecordingBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > maxRecordingBytes {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "RECORDING_STORE_FAILED", "Could not store the recording or it exceeds 100 MB.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO consultation_recordings(id,encounter_id,storage_name,media_type,size_bytes,duration_seconds,checksum_sha256,consent_confirmed,transcript_status,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,1,'pending',?,?,?,?)`,
		id, encounterID, storageName, mediaType, size, duration, hex.EncodeToString(hash.Sum(nil)), now, now, user.ID, user.ID)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "RECORDING_METADATA_FAILED", "Recording was not added to the consultation.")
		return
	}
	s.audit(r.Context(), &user, "record", "consultation_recording", id, "Recorded consultation audio with confirmed patient consent", "", "", r)
	s.broker.Publish(realtime.Event{Type: "recording.created", EntityType: "consultation_recording", EntityID: id})
	go s.transcribeRecordingAsync(id, path)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "transcriptStatus": "pending", "createdAt": now})
}

func (s *Server) handleRecordingAudio(w http.ResponseWriter, r *http.Request) {
	var storage, mediaType string
	err := s.db.QueryRowContext(r.Context(), "SELECT storage_name,media_type FROM consultation_recordings WHERE id=? AND archived_at IS NULL", chi.URLParam(r, "id")).Scan(&storage, &mediaType)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "RECORDING_NOT_FOUND", "Recording was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECORDING_LOAD_FAILED", "Could not load the recording.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	http.ServeFile(w, r, filepath.Join(s.config.DataDir, "documents", "recordings", filepath.Base(storage)))
}

func (s *Server) handleRecordingTranscriptUpdate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), "UPDATE consultation_recordings SET transcript_text=?,transcript_edited=1,updated_at=?,updated_by=? WHERE id=? AND archived_at IS NULL", strings.TrimSpace(input.Text), now, user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TRANSCRIPT_UPDATE_FAILED", "Could not save the transcript correction.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "RECORDING_NOT_FOUND", "Recording was not found.")
		return
	}
	s.audit(r.Context(), &user, "update", "consultation_recording", id, "Corrected consultation transcript", "", "", r)
	s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) handleRecordingArchive(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), "UPDATE consultation_recordings SET archived_at=?,updated_at=?,updated_by=? WHERE id=? AND archived_at IS NULL", now, now, user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECORDING_ARCHIVE_FAILED", "Could not delete the recording.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "RECORDING_NOT_FOUND", "Recording was not found.")
		return
	}
	s.audit(r.Context(), &user, "archive", "consultation_recording", id, "Deleted consultation recording", "", "", r)
	s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}

// transcriptionSettings mirrors the subset of the "clinical" settings blob this file cares
// about; the rest of that JSON object (appointmentDuration, enabledSections, ...) is
// preserved untouched by handleSettingsUpdate since it stores the whole blob as one value.
type transcriptionSettings struct {
	TranscriptionEnabled bool   `json:"transcriptionEnabled"`
	TranscriptionCommand string `json:"transcriptionCommand"`
}

func (s *Server) transcriptionCommand(ctx context.Context) string {
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='clinical'").Scan(&raw); err != nil {
		return ""
	}
	var value transcriptionSettings
	if json.Unmarshal([]byte(raw), &value) != nil || !value.TranscriptionEnabled {
		return ""
	}
	return strings.TrimSpace(value.TranscriptionCommand)
}

// transcribeRecordingAsync runs entirely against the local filesystem and a locally
// configured command: no network call is made here. If no command is configured the
// recording is simply marked unavailable rather than silently left pending forever.
func (s *Server) transcribeRecordingAsync(recordingID, audioPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	now := func() string { return time.Now().UTC().Format(time.RFC3339Nano) }
	command := s.transcriptionCommand(ctx)
	if command == "" {
		_, _ = s.db.ExecContext(ctx, "UPDATE consultation_recordings SET transcript_status='unavailable',updated_at=? WHERE id=?", now(), recordingID)
		s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: recordingID})
		return
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE consultation_recordings SET transcript_status='processing',updated_at=? WHERE id=?", now(), recordingID)
	s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: recordingID})

	parts := strings.Fields(command)
	if len(parts) == 0 {
		_, _ = s.db.ExecContext(ctx, "UPDATE consultation_recordings SET transcript_status='failed',transcript_error='Transcription command is empty.',updated_at=? WHERE id=?", now(), recordingID)
		s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: recordingID})
		return
	}
	cmd := exec.CommandContext(ctx, parts[0], append(append([]string{}, parts[1:]...), audioPath)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		if len(message) > 500 {
			message = message[:500]
		}
		s.logger.Error("consultation transcription failed", "recording_id", recordingID, "error", err, "stderr", stderr.String())
		_, _ = s.db.ExecContext(ctx, "UPDATE consultation_recordings SET transcript_status='failed',transcript_error=?,updated_at=? WHERE id=?", message, now(), recordingID)
		s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: recordingID})
		return
	}
	text := strings.TrimSpace(stdout.String())
	_, _ = s.db.ExecContext(ctx, "UPDATE consultation_recordings SET transcript_status='done',transcript_text=?,updated_at=? WHERE id=?", text, now(), recordingID)
	s.broker.Publish(realtime.Event{Type: "recording.updated", EntityType: "consultation_recording", EntityID: recordingID})
}
