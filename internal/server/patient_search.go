package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// B1: the patient list is queried on the server, page by page. Nothing here ever
// loads the whole patients table, and the free-text half runs against the FTS5
// index added in migration 012 rather than a LIKE '%term%' scan.

const patientSearchMaxTerms = 8

// buildMatchExpression turns what a user typed into an FTS5 query. Every term is
// double-quoted so that FTS operators typed by accident ("AND", "*", ":", "-")
// are matched literally, and each is prefix-matched so results narrow as the
// user types. Terms are ANDed: "jean pierre" must match both.
func buildMatchExpression(query string) string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || r == '"'
	})
	terms := make([]string, 0, patientSearchMaxTerms)
	for _, field := range fields {
		trimmed := strings.TrimFunc(field, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if trimmed == "" {
			continue
		}
		terms = append(terms, `"`+trimmed+`"*`)
		if len(terms) == patientSearchMaxTerms {
			break
		}
	}
	return strings.Join(terms, " AND ")
}

type patientFilters struct {
	query          string
	sex            string
	insurance      string
	practitioner   string
	status         string
	ageMin         string
	ageMax         string
	lastVisitFrom  string
	lastVisitUntil string
}

func readPatientFilters(r *http.Request) patientFilters {
	values := r.URL.Query()
	get := func(key string) string { return strings.TrimSpace(values.Get(key)) }
	return patientFilters{
		query:          get("q"),
		sex:            get("sex"),
		insurance:      get("insurance"),
		practitioner:   get("practitioner"),
		status:         get("status"),
		ageMin:         get("ageMin"),
		ageMax:         get("ageMax"),
		lastVisitFrom:  get("lastVisitFrom"),
		lastVisitUntil: get("lastVisitUntil"),
	}
}

// lastVisitExpression is the most recent consultation on record. It is selected
// for display and reused by the last-visit filter.
const lastVisitExpression = `(SELECT MAX(e.created_at) FROM encounters e WHERE e.patient_id = p.id)`

func (f patientFilters) conditions() (string, []any) {
	clauses := []string{}
	args := []any{}

	switch f.status {
	case "archived":
		clauses = append(clauses, "p.archived_at IS NOT NULL")
	case "all":
	default:
		clauses = append(clauses, "p.archived_at IS NULL")
	}
	if f.sex != "" {
		clauses = append(clauses, "p.sex = ?")
		args = append(args, f.sex)
	}
	// Age is derived from the date of birth so that no stored age can go stale.
	// A minimum age is an upper bound on the birth date, and vice versa.
	if minimum, err := strconv.Atoi(f.ageMin); err == nil && minimum >= 0 && minimum <= 150 {
		clauses = append(clauses, "p.date_of_birth IS NOT NULL AND p.date_of_birth <> '' AND p.date_of_birth <= ?")
		args = append(args, time.Now().UTC().AddDate(-minimum, 0, 0).Format("2006-01-02"))
	}
	if maximum, err := strconv.Atoi(f.ageMax); err == nil && maximum >= 0 && maximum <= 150 {
		clauses = append(clauses, "p.date_of_birth IS NOT NULL AND p.date_of_birth <> '' AND p.date_of_birth > ?")
		args = append(args, time.Now().UTC().AddDate(-(maximum+1), 0, 0).Format("2006-01-02"))
	}
	if f.insurance != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM patient_insurance pi WHERE pi.patient_id = p.id AND (pi.payer_id = ? OR pi.payer_name = ?))")
		args = append(args, f.insurance, f.insurance)
	}
	if f.practitioner != "" {
		// The schema has no "usual practitioner" column, so this means the
		// clinicians who have actually seen or are scheduled to see the patient.
		clauses = append(clauses, `(EXISTS (SELECT 1 FROM encounters e WHERE e.patient_id = p.id AND e.doctor_id = ?)
			OR EXISTS (SELECT 1 FROM appointments a WHERE a.patient_id = p.id AND a.practitioner_id = ?))`)
		args = append(args, f.practitioner, f.practitioner)
	}
	if f.lastVisitFrom != "" {
		clauses = append(clauses, lastVisitExpression+" >= ?")
		args = append(args, f.lastVisitFrom)
	}
	if f.lastVisitUntil != "" {
		clauses = append(clauses, lastVisitExpression+" <= ?")
		args = append(args, f.lastVisitUntil)
	}
	if len(clauses) == 0 {
		return "1=1", args
	}
	return strings.Join(clauses, " AND "), args
}

// digitsOnly is the SQL that strips the separators a phone number may be written
// with, so that a number stored as "+509 3456 7890" can be matched as digits.
const digitsOnly = `replace(replace(replace(replace(replace(replace(COALESCE(%s,''),' ',''),'-',''),'(',''),')',''),'+',''),'.','')`

var digitTerm = regexp.MustCompile(`^[0-9]{4,}$`)

// textCondition matches the typed query. Words go through the FTS5 index, which
// is accent- and case-insensitive and matches on a word prefix. A run of four or
// more digits is additionally matched anywhere inside the stored phone numbers,
// because staff type the tail of a number ("34567890") for a record stored with
// a country code, and a prefix match cannot find that.
func (f patientFilters) textCondition() (string, []any) {
	match := buildMatchExpression(f.query)
	if match == "" {
		return "", nil
	}
	clauses := []string{"p.id IN (SELECT patient_id FROM patients_search WHERE patients_search MATCH ?)"}
	args := []any{match}
	for _, field := range strings.Fields(f.query) {
		if !digitTerm.MatchString(field) {
			continue
		}
		clauses = append(clauses,
			fmt.Sprintf("%s LIKE ?", fmt.Sprintf(digitsOnly, "p.phone")),
			fmt.Sprintf("%s LIKE ?", fmt.Sprintf(digitsOnly, "p.alternate_phone")))
		args = append(args, "%"+field+"%", "%"+field+"%")
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

func (s *Server) handlePatientsList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}
	filters := readPatientFilters(r)
	text, textArgs := filters.textCondition()
	where, whereArgs := filters.conditions()
	args := append(append([]any{}, textArgs...), whereArgs...)
	if text != "" {
		where = text + " AND " + where
	}

	var total int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM patients p WHERE "+where, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_LIST_FAILED", "Could not load patients.")
		return
	}
	listArgs := append(append([]any{}, args...), limit, (page-1)*limit)
	query := fmt.Sprintf("SELECT %s, COALESCE(%s,'') FROM patients p WHERE %s ORDER BY p.updated_at DESC LIMIT ? OFFSET ?",
		prefixedPatientColumns, lastVisitExpression, where)
	rows, err := s.db.QueryContext(r.Context(), query, listArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_LIST_FAILED", "Could not load patients.")
		return
	}
	defer rows.Close()
	items := []patientListEntry{}
	for rows.Next() {
		var entry patientListEntry
		record, err := scanPatientWithExtras(rows, &entry.LastVisitAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "PATIENT_LIST_FAILED", "Could not load patients.")
			return
		}
		entry.patientRecord = record
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_LIST_FAILED", "Could not load patients.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "limit": limit, "total": total, "hasMore": page*limit < total})
}

type patientListEntry struct {
	patientRecord
	LastVisitAt string `json:"lastVisitAt"`
}

// handlePatientFilterOptions feeds the filter controls without the client having
// to load every patient to discover which insurers and practitioners exist.
func (s *Server) handlePatientFilterOptions(w http.ResponseWriter, r *http.Request) {
	payers := []map[string]string{}
	rows, err := s.db.QueryContext(r.Context(), `SELECT DISTINCT COALESCE(NULLIF(payer_id,''), payer_name) AS value, payer_name FROM patient_insurance ORDER BY payer_name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var value, name string
			if rows.Scan(&value, &name) == nil {
				payers = append(payers, map[string]string{"value": value, "label": name})
			}
		}
	}
	practitioners := []map[string]string{}
	staff, err := s.db.QueryContext(r.Context(), `SELECT id, display_name FROM users WHERE active=1 ORDER BY display_name`)
	if err == nil {
		defer staff.Close()
		for staff.Next() {
			var id, name string
			if staff.Scan(&id, &name) == nil {
				practitioners = append(practitioners, map[string]string{"value": id, "label": name})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"insurers": payers, "practitioners": practitioners})
}
