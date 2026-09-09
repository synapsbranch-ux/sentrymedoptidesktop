package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// The bundled reference is a working aid for entering a diagnosis, not a billing
// tabular list: it covers the ophthalmology-relevant chapters a clinic sees, with
// the lay terms patients use, and a clinic that must bill against the complete
// authoritative set imports its own file over the top.
//
//go:embed reference/icd10_ophthalmology.json
var bundledDiagnosisCodes []byte

type diagnosisCode struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Synonyms    string `json:"synonyms"`
}

// ensureDiagnosisReference loads the bundled code set into the searchable table
// the first time it is needed, and again after an upgrade changes it. Clinic-
// imported rows are left alone: only 'builtin' rows are replaced.
func (s *Server) ensureDiagnosisReference(ctx context.Context) {
	s.diagnosisReference.Do(func() {
		if err := s.syncDiagnosisReference(ctx); err != nil {
			s.logger.Error("load diagnosis reference", "error", err)
		}
	})
}

func (s *Server) syncDiagnosisReference(ctx context.Context) error {
	sum := sha256.Sum256(bundledDiagnosisCodes)
	checksum := hex.EncodeToString(sum[:])
	var loaded string
	_ = s.db.QueryRowContext(ctx, "SELECT checksum FROM reference_data_versions WHERE name='icd10_ophthalmology'").Scan(&loaded)
	if loaded == checksum {
		return nil
	}
	var codes []diagnosisCode
	if err := json.Unmarshal(bundledDiagnosisCodes, &codes); err != nil {
		return fmt.Errorf("parse bundled diagnosis codes: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM diagnosis_codes WHERE source='builtin'"); err != nil {
			return err
		}
		insert, err := tx.PrepareContext(ctx, "INSERT OR IGNORE INTO diagnosis_codes(code,system,description,category,synonyms,source,created_at,updated_at) VALUES(?,'ICD-10',?,?,?,'builtin',?,?)")
		if err != nil {
			return err
		}
		defer insert.Close()
		for _, code := range codes {
			if _, err := insert.ExecContext(ctx, code.Code, code.Description, code.Category, code.Synonyms, now, now); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO reference_data_versions(name,checksum,row_count,applied_at) VALUES('icd10_ophthalmology',?,?,?) ON CONFLICT(name) DO UPDATE SET checksum=excluded.checksum,row_count=excluded.row_count,applied_at=excluded.applied_at", checksum, len(codes), now)
		return err
	})
}

// ftsQuery turns what a clinician typed into an FTS5 MATCH expression. Every
// term is quoted so punctuation in a code ("H52.13") cannot be read as FTS
// syntax, and the last term gets a prefix wildcard so results narrow while the
// clinician is still typing.
func ftsQuery(input string) string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !(r == '.' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127)
	})
	if len(fields) == 0 {
		return ""
	}
	terms := make([]string, 0, len(fields))
	for index, field := range fields {
		quoted := `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
		if index == len(fields)-1 {
			quoted += "*"
		}
		terms = append(terms, quoted)
	}
	return strings.Join(terms, " ")
}

func (s *Server) registerDiagnosisReferenceRoutes(r chi.Router) {
	r.Get("/diagnosis-codes", s.handleDiagnosisCodeBrowse)
	r.Get("/diagnosis-codes/categories", s.handleDiagnosisCodeCategories)
	r.With(s.requireDoctor).Post("/diagnosis-codes/import", s.handleDiagnosisCodeImport)
}

func (s *Server) searchDiagnosisCodes(ctx context.Context, query, category string, limit, offset int) ([]map[string]any, int, error) {
	s.ensureDiagnosisReference(ctx)
	where, args := []string{"d.active=1"}, []any{}
	from := "diagnosis_codes d"
	order := "d.code"
	if match := ftsQuery(query); match != "" {
		// Ranked by FTS relevance, so "red eye" puts conjunctivitis above a code
		// that merely mentions redness in passing.
		from = "diagnosis_codes_search s JOIN diagnosis_codes d ON d.code=s.code"
		where = append(where, "diagnosis_codes_search MATCH ?")
		args = append(args, match)
		order = "rank, d.code"
	} else if strings.TrimSpace(query) != "" {
		// Every term was punctuation the tokenizer drops; nothing can match.
		return []map[string]any{}, 0, nil
	}
	if category != "" {
		where = append(where, "d.category=?")
		args = append(args, category)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+from+" WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT d.code,d.system,d.description,d.category,d.synonyms,d.source FROM "+from+" WHERE "+clause+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var code, system, description, category, synonyms, source string
		if err := rows.Scan(&code, &system, &description, &category, &synonyms, &source); err != nil {
			return nil, 0, err
		}
		items = append(items, map[string]any{"code": code, "system": system, "description": description, "category": category, "synonyms": splitSynonyms(synonyms), "source": source})
	}
	return items, total, rows.Err()
}

func splitSynonyms(raw string) []string {
	terms := []string{}
	for _, term := range strings.Split(raw, ";") {
		if trimmed := strings.TrimSpace(term); trimmed != "" {
			terms = append(terms, trimmed)
		}
	}
	return terms
}

// handleDiagnosisCodeBrowse backs both the type-ahead on the diagnosis field and
// the full reference library the clinician can page through by category.
func (s *Server) handleDiagnosisCodeBrowse(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 25
	}
	items, total, err := s.searchDiagnosisCodes(r.Context(), r.URL.Query().Get("q"), strings.TrimSpace(r.URL.Query().Get("category")), limit, (page-1)*limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DIAGNOSIS_CODE_SEARCH_FAILED", "Could not search the diagnosis reference.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "limit": limit, "total": total, "hasMore": page*limit < total})
}

func (s *Server) handleDiagnosisCodeCategories(w http.ResponseWriter, r *http.Request) {
	s.ensureDiagnosisReference(r.Context())
	rows, err := s.db.QueryContext(r.Context(), "SELECT category, COUNT(*) FROM diagnosis_codes WHERE active=1 AND category<>'' GROUP BY category ORDER BY category")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DIAGNOSIS_CODE_CATEGORIES_FAILED", "Could not load the diagnosis reference categories.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			writeError(w, http.StatusInternalServerError, "DIAGNOSIS_CODE_CATEGORIES_FAILED", "Could not load the diagnosis reference categories.")
			return
		}
		items = append(items, map[string]any{"category": category, "count": count})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleDiagnosisCodeImport loads a clinic's own code file — the complete
// authoritative ICD-10/ICD-11 tabular list, or a local code set — as CSV with a
// code,description[,category[,synonyms[,system]]] header. Imported rows own the
// codes they name and survive every later upgrade of the bundled reference.
func (s *Server) handleDiagnosisCodeImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "IMPORT_TOO_LARGE", "The code file must be 16 MB or smaller.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "IMPORT_FILE_REQUIRED", "A CSV file of diagnosis codes is required.")
		return
	}
	defer file.Close()
	system := strings.TrimSpace(r.FormValue("system"))
	if system == "" {
		system = "ICD-10"
	}
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "IMPORT_UNREADABLE", "The code file could not be read as CSV.")
		return
	}
	columns := map[string]int{}
	for index, name := range header {
		columns[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))] = index
	}
	codeAt, codeOK := columns["code"]
	descriptionAt, descriptionOK := columns["description"]
	if !codeOK || !descriptionOK {
		writeError(w, http.StatusUnprocessableEntity, "IMPORT_HEADER_REQUIRED", "The first row must name at least a code and a description column.")
		return
	}
	value := func(record []string, index int, present bool) string {
		if !present || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	categoryAt, categoryOK := columns["category"]
	synonymsAt, synonymsOK := columns["synonyms"]
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	imported, skipped := 0, 0
	err = s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		statement, err := tx.PrepareContext(r.Context(), `INSERT INTO diagnosis_codes(code,system,description,category,synonyms,source,created_at,updated_at) VALUES(?,?,?,?,?,'clinic',?,?)
			ON CONFLICT(code) DO UPDATE SET system=excluded.system,description=excluded.description,category=excluded.category,synonyms=excluded.synonyms,source='clinic',active=1,updated_at=excluded.updated_at`)
		if err != nil {
			return err
		}
		defer statement.Close()
		for {
			record, err := reader.Read()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			code, description := value(record, codeAt, true), value(record, descriptionAt, true)
			if code == "" || description == "" {
				skipped++
				continue
			}
			if _, err := statement.ExecContext(r.Context(), code, system, description, value(record, categoryAt, categoryOK), value(record, synonymsAt, synonymsOK), now, now); err != nil {
				return err
			}
			imported++
		}
	})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "IMPORT_FAILED", "The code file could not be imported. Check that it is CSV with code and description columns.")
		return
	}
	s.audit(r.Context(), &user, "import", "diagnosis_codes", system, fmt.Sprintf("Imported %d %s diagnosis codes (%d rows skipped)", imported, system, skipped), "", "", r)
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "system": system})
}
