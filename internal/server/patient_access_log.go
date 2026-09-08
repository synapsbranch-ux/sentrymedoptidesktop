package server

import (
	"net/http"
	"sync"
	"time"
)

// Safety: this application has no per-patient authorisation — any signed-in
// clinician can open any patient, which is deliberate for a single small
// practice where whoever is free sees whoever walks in. Restricting access by
// assignment would break that workflow, so the control here is accountability
// instead: every time an individual patient's record or one of their documents
// is opened, who opened it is written to the audit log a doctor can already read.
//
// Only record-level reads are logged. Listing or searching patients is not,
// because a search is not a record view and logging it would bury the entries
// that matter under polling noise.

// A record left open re-fetches on every live update, so the same clinician
// looking at the same patient is recorded once per window rather than per
// request. The window is short enough that returning to a record later is a
// fresh entry.
const patientAccessLogWindow = 10 * time.Minute

type patientAccessLog struct {
	mutex  sync.Mutex
	recent map[string]time.Time
}

func newPatientAccessLog() *patientAccessLog {
	return &patientAccessLog{recent: map[string]time.Time{}}
}

// shouldRecord reports whether this access is outside the window for this
// user/patient/kind, and marks it. It also drops entries that have aged out, so
// the map cannot grow without bound on a long-running clinic server.
func (l *patientAccessLog) shouldRecord(key string, now time.Time) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for existing, seen := range l.recent {
		if now.Sub(seen) > patientAccessLogWindow {
			delete(l.recent, existing)
		}
	}
	if seen, ok := l.recent[key]; ok && now.Sub(seen) <= patientAccessLogWindow {
		return false
	}
	l.recent[key] = now
	return true
}

// recordPatientAccess writes an audit entry naming who read this patient's data.
func (s *Server) recordPatientAccess(r *http.Request, patientID, kind, summary string) {
	if patientID == "" {
		return
	}
	user, ok := userFromContext(r.Context())
	if !ok {
		return
	}
	if !s.patientAccess.shouldRecord(user.ID+"|"+patientID+"|"+kind, time.Now()) {
		return
	}
	s.audit(r.Context(), &user, "read", kind, patientID, summary, "", "", r)
}
