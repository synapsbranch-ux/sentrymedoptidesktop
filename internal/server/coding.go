package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type codeEntry struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// icd10Ophthalmology is a small offline reference of ICD-10-CM codes commonly used in an
// optical/ophthalmology practice. It exists purely to speed up and standardize diagnosis
// entry (search-as-you-type instead of free text) — it is not a complete or authoritative
// coding table and clinics should verify codes against their local billing requirements.
var icd10Ophthalmology = []codeEntry{
	{"H52.10", "Myopia, unspecified eye"},
	{"H52.11", "Myopia, right eye"},
	{"H52.12", "Myopia, left eye"},
	{"H52.13", "Myopia, bilateral"},
	{"H52.00", "Hypermetropia, unspecified eye"},
	{"H52.01", "Hypermetropia, right eye"},
	{"H52.02", "Hypermetropia, left eye"},
	{"H52.03", "Hypermetropia, bilateral"},
	{"H52.201", "Astigmatism, unspecified, right eye"},
	{"H52.202", "Astigmatism, unspecified, left eye"},
	{"H52.203", "Astigmatism, unspecified, bilateral"},
	{"H52.4", "Presbyopia"},
	{"H52.221", "Irregular astigmatism, right eye"},
	{"H52.222", "Irregular astigmatism, left eye"},
	{"H25.10", "Age-related nuclear cataract, unspecified eye"},
	{"H25.11", "Age-related nuclear cataract, right eye"},
	{"H25.12", "Age-related nuclear cataract, left eye"},
	{"H25.13", "Age-related nuclear cataract, bilateral"},
	{"H26.9", "Cataract, unspecified"},
	{"H40.9", "Unspecified glaucoma"},
	{"H40.11X0", "Primary open-angle glaucoma, stage unspecified"},
	{"H40.021", "Anatomically narrow angle, right eye"},
	{"H40.022", "Anatomically narrow angle, left eye"},
	{"H35.30", "Unspecified macular degeneration"},
	{"H35.31", "Nonexudative age-related macular degeneration"},
	{"H35.32", "Exudative age-related macular degeneration"},
	{"E11.319", "Type 2 diabetes with unspecified diabetic retinopathy without macular edema"},
	{"E11.311", "Type 2 diabetes with unspecified diabetic retinopathy with macular edema"},
	{"H35.00", "Background retinopathy, unspecified"},
	{"H10.9", "Unspecified conjunctivitis"},
	{"H10.13", "Acute atopic conjunctivitis, bilateral"},
	{"H10.401", "Chronic conjunctivitis, unspecified, right eye"},
	{"H10.501", "Blepharoconjunctivitis, unspecified, right eye"},
	{"H16.9", "Unspecified keratitis"},
	{"H18.601", "Keratoconus, unspecified, stable, right eye"},
	{"H18.602", "Keratoconus, unspecified, stable, left eye"},
	{"H04.121", "Dry eye syndrome of right lacrimal gland"},
	{"H04.123", "Dry eye syndrome, bilateral"},
	{"H53.2", "Diplopia"},
	{"H53.9", "Unspecified visual disturbance"},
	{"H53.40", "Unspecified visual field defect"},
	{"H50.00", "Unspecified esotropia"},
	{"H50.10", "Unspecified exotropia"},
	{"H02.401", "Unspecified ptosis of right eyelid"},
	{"H02.402", "Unspecified ptosis of left eyelid"},
	{"H00.011", "Hordeolum externum, right eye"},
	{"H00.012", "Hordeolum externum, left eye"},
	{"H11.001", "Pterygium of right eye, unspecified"},
	{"H11.002", "Pterygium of left eye, unspecified"},
	{"H43.10", "Vitreous hemorrhage, unspecified eye"},
	{"H33.20", "Serous retinal detachment, unspecified eye"},
	{"Z01.00", "Encounter for eye examination without abnormal findings"},
	{"Z01.01", "Encounter for eye examination with abnormal findings"},
	{"Z96.1", "Presence of intraocular lens"},
}

// procedureCodes is a small offline reference of CPT-style procedure and material codes
// commonly billed in an optical/ophthalmology practice, paired with short generic
// descriptions. Descriptions are written independently of any copyrighted coding manual;
// clinics remain responsible for confirming the exact code that applies to a given service.
var procedureCodes = []codeEntry{
	{"92004", "Comprehensive ophthalmological exam, new patient"},
	{"92014", "Comprehensive ophthalmological exam, established patient"},
	{"92002", "Intermediate ophthalmological exam, new patient"},
	{"92012", "Intermediate ophthalmological exam, established patient"},
	{"92015", "Refraction, determination of refractive state"},
	{"92081", "Visual field examination, limited"},
	{"92083", "Visual field examination, extended"},
	{"92133", "Scanning imaging of optic nerve (OCT)"},
	{"92134", "Scanning imaging of retina (OCT)"},
	{"92225", "Ophthalmoscopy, extended, initial"},
	{"92226", "Ophthalmoscopy, extended, subsequent"},
	{"92235", "Fluorescein angiography"},
	{"92310", "Contact lens fitting, corneal, both eyes"},
	{"92326", "Replacement of contact lens"},
	{"99213", "Established patient office visit, low-moderate complexity"},
	{"99203", "New patient office visit, low-moderate complexity"},
	{"V2020", "Frame, purchases"},
	{"V2100", "Single vision spherical lens, per lens"},
	{"V2200", "Bifocal spherical lens, per lens"},
	{"V2781", "Progressive lens, per lens"},
	{"V2500", "Contact lens, spherical, per lens"},
	{"V2521", "Contact lens, gas permeable, per lens"},
	{"S0620", "Routine ophthalmological exam, new patient"},
	{"S0621", "Routine ophthalmological exam, established patient"},
}

func searchCodes(codes []codeEntry, query string, limit int) []codeEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	matches := []codeEntry{}
	for _, entry := range codes {
		if query == "" || strings.Contains(strings.ToLower(entry.Code), query) || strings.Contains(strings.ToLower(entry.Description), query) {
			matches = append(matches, entry)
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}

func (s *Server) registerCodingRoutes(r chi.Router) {
	r.Get("/codes/icd10", s.handleICD10Search)
	r.Get("/codes/procedures", s.handleProcedureCodesSearch)
	r.Get("/encounters/{id}/superbill", s.handleSuperbillGet)
	r.Get("/encounters/{id}/superbill/candidate-invoices", s.handleSuperbillCandidateInvoices)
	r.With(s.requireDoctor).Post("/encounters/{id}/superbill/link-invoice", s.handleSuperbillLinkInvoice)
}

func (s *Server) handleICD10Search(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": searchCodes(icd10Ophthalmology, r.URL.Query().Get("q"), 25)})
}

func (s *Server) handleProcedureCodesSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": searchCodes(procedureCodes, r.URL.Query().Get("q"), 25)})
}

func (s *Server) handleSuperbillCandidateInvoices(w http.ResponseWriter, r *http.Request) {
	encounterID := chi.URLParam(r, "id")
	var patientID string
	if err := s.db.QueryRowContext(r.Context(), "SELECT patient_id FROM encounters WHERE id=?", encounterID).Scan(&patientID); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,invoice_number,total_minor,currency,created_at FROM invoices
		WHERE patient_id=? AND status<>'cancelled' AND (encounter_id IS NULL OR encounter_id=?) ORDER BY created_at DESC LIMIT 25`, patientID, encounterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPERBILL_CANDIDATES_FAILED", "Could not load billable invoices.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, currency, createdAt string
		var total int64
		if err := rows.Scan(&id, &number, &total, &currency, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "SUPERBILL_CANDIDATES_FAILED", "Could not load billable invoices.")
			return
		}
		items = append(items, map[string]any{"id": id, "invoiceNumber": number, "totalMinor": total, "currency": currency, "createdAt": createdAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSuperbillLinkInvoice(w http.ResponseWriter, r *http.Request) {
	var input struct {
		InvoiceID string `json:"invoiceId"`
	}
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.InvoiceID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "An invoice id is required.")
		return
	}
	encounterID := chi.URLParam(r, "id")
	var patientID string
	if err := s.db.QueryRowContext(r.Context(), "SELECT patient_id FROM encounters WHERE id=?", encounterID).Scan(&patientID); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), "UPDATE invoices SET encounter_id=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND patient_id=? AND archived_at IS NULL", encounterID, now, user.ID, input.InvoiceID, patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPERBILL_LINK_FAILED", "Could not link the invoice to this consultation.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusUnprocessableEntity, "INVOICE_PATIENT_MISMATCH", "That invoice does not belong to this patient.")
		return
	}
	s.audit(r.Context(), &user, "update", "invoice", input.InvoiceID, "Linked invoice to consultation for superbill", "", "", r)
	writeJSON(w, http.StatusOK, map[string]any{"linked": true})
}

func (s *Server) handleSuperbillGet(w http.ResponseWriter, r *http.Request) {
	encounterID := chi.URLParam(r, "id")
	var patientID, patientName, mrn, doctorName, visitReason, createdAt string
	err := s.db.QueryRowContext(r.Context(), `SELECT e.patient_id,p.first_name||' '||p.last_name,p.medical_record_number,COALESCE(u.display_name,''),COALESCE(e.visit_reason,''),e.created_at
		FROM encounters e JOIN patients p ON p.id=e.patient_id LEFT JOIN users u ON u.id=e.doctor_id WHERE e.id=?`, encounterID).
		Scan(&patientID, &patientName, &mrn, &doctorName, &visitReason, &createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	var clinicName, clinicAddress, clinicPhone, clinicNIF string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.name'),''),COALESCE(json_extract(value_json,'$.address'),''),COALESCE(json_extract(value_json,'$.phone'),''),COALESCE(json_extract(value_json,'$.nif'),'') FROM settings WHERE key='clinic'`).
		Scan(&clinicName, &clinicAddress, &clinicPhone, &clinicNIF)

	diagnoses := []map[string]any{}
	rows, err := s.db.QueryContext(r.Context(), "SELECT diagnosis,COALESCE(code,''),COALESCE(laterality,''),is_primary FROM diagnoses WHERE encounter_id=? ORDER BY is_primary DESC,created_at", encounterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPERBILL_FAILED", "Could not build the superbill.")
		return
	}
	if rows != nil {
		for rows.Next() {
			var diagnosis, code, laterality string
			var primary bool
			if err := rows.Scan(&diagnosis, &code, &laterality, &primary); err != nil {
				writeError(w, http.StatusInternalServerError, "SUPERBILL_FAILED", "Could not build the superbill.")
				return
			}
			diagnoses = append(diagnoses, map[string]any{"diagnosis": diagnosis, "code": code, "laterality": laterality, "primary": primary})
		}
		_ = rows.Close()
	}

	invoices := []map[string]any{}
	invoiceRows, err := s.db.QueryContext(r.Context(), "SELECT id,invoice_number,total_minor,currency,created_at FROM invoices WHERE encounter_id=? AND archived_at IS NULL ORDER BY created_at", encounterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPERBILL_FAILED", "Could not build the superbill.")
		return
	}
	if invoiceRows != nil {
		for invoiceRows.Next() {
			var invoiceID, number, currency, invoiceCreatedAt string
			var total int64
			if err := invoiceRows.Scan(&invoiceID, &number, &total, &currency, &invoiceCreatedAt); err != nil {
				writeError(w, http.StatusInternalServerError, "SUPERBILL_FAILED", "Could not build the superbill.")
				return
			}
			{
				lineItems := []map[string]any{}
				lineRows, err := s.db.QueryContext(r.Context(), "SELECT description,COALESCE(procedure_code,''),quantity,unit_price_minor,line_total_minor FROM invoice_items WHERE invoice_id=?", invoiceID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "SUPERBILL_FAILED", "Could not build the superbill.")
					return
				}
				if lineRows != nil {
					for lineRows.Next() {
						var description, procedureCode string
						var quantity int
						var unitPrice, lineTotal int64
						if err := lineRows.Scan(&description, &procedureCode, &quantity, &unitPrice, &lineTotal); err != nil {
							writeError(w, http.StatusInternalServerError, "SUPERBILL_FAILED", "Could not build the superbill.")
							return
						}
						lineItems = append(lineItems, map[string]any{"description": description, "procedureCode": procedureCode, "quantity": quantity, "unitPriceMinor": unitPrice, "lineTotalMinor": lineTotal})
					}
					_ = lineRows.Close()
				}
				invoices = append(invoices, map[string]any{"id": invoiceID, "invoiceNumber": number, "totalMinor": total, "currency": currency, "createdAt": invoiceCreatedAt, "items": lineItems})
			}
		}
		_ = invoiceRows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"encounterId": encounterID, "generatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"clinic":     map[string]any{"name": clinicName, "address": clinicAddress, "phone": clinicPhone, "nif": clinicNIF},
		"patient":    map[string]any{"id": patientID, "name": patientName, "medicalRecordNumber": mrn},
		"doctorName": doctorName, "visitReason": visitReason, "visitDate": createdAt,
		"diagnoses": diagnoses, "invoices": invoices,
	})
}
