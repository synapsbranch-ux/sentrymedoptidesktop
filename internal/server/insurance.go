package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

var claimStatuses = map[string]bool{"draft": true, "submitted": true, "pending": true, "approved": true, "partially_paid": true, "paid": true, "rejected": true, "cancelled": true}

const payerColumns = `id,name,COALESCE(contact_name,''),COALESCE(phone,''),COALESCE(email,''),COALESCE(address,''),COALESCE(website,''),COALESCE(notes,''),COALESCE(accepted_coverage,''),COALESCE(billing_info,''),COALESCE(claim_instructions,''),default_coverage_percent,active,version,created_at,updated_at`

func scanPayer(row interface{ Scan(...any) error }) (map[string]any, error) {
	var id, name, contact, phone, email, address, website, notes, acceptedCoverage, billingInfo, claimInstructions, created, updated string
	var defaultCoverage float64
	var active bool
	var version int
	if err := row.Scan(&id, &name, &contact, &phone, &email, &address, &website, &notes, &acceptedCoverage, &billingInfo, &claimInstructions, &defaultCoverage, &active, &version, &created, &updated); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "name": name, "contactName": contact, "phone": phone, "email": email, "address": address, "website": website, "notes": notes, "acceptedCoverage": acceptedCoverage, "billingInfo": billingInfo, "claimInstructions": claimInstructions, "defaultCoveragePercent": defaultCoverage, "active": active, "version": version, "createdAt": created, "updatedAt": updated}, nil
}

func (s *Server) handlePayersList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+payerColumns+` FROM payers ORDER BY active DESC,name`)
	if err != nil {
		writeError(w, 500, "PAYERS_FAILED", "Could not load insurers.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item, err := scanPayer(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "PAYERS_FAILED", "Could not load insurers.")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handlePayerGet(w http.ResponseWriter, r *http.Request) {
	item, err := scanPayer(s.db.QueryRowContext(r.Context(), `SELECT `+payerColumns+` FROM payers WHERE id=?`, chi.URLParam(r, "id")))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PAYER_NOT_FOUND", "That insurer was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYER_LOAD_FAILED", "Could not load the insurer.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type payerPayload struct {
	Name, ContactName, Phone, Email, Address                         string
	Website, Notes, AcceptedCoverage, BillingInfo, ClaimInstructions string
	DefaultCoveragePercent                                           float64
	Active                                                           bool
	Version                                                          int
}

func validatePayer(in *payerPayload) *APIError {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return &APIError{Code: "INVALID_PAYER", Message: "Insurer name is required."}
	}
	if in.DefaultCoveragePercent < 0 || in.DefaultCoveragePercent > 100 {
		return &APIError{Code: "INVALID_COVERAGE", Message: "Default coverage must be between 0 and 100 percent."}
	}
	return nil
}

func (s *Server) handlePayerCreate(w http.ResponseWriter, r *http.Request) {
	var in payerPayload
	if decodeJSON(r, &in) != nil {
		writeError(w, 400, "INVALID_PAYER", "Insurer name is required.")
		return
	}
	if validation := validatePayer(&in); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	u, _ := userFromContext(r.Context())
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO payers(id,name,contact_name,phone,email,address,website,notes,accepted_coverage,billing_info,claim_instructions,default_coverage_percent,active,version,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,1,?,?,?)`,
		id, in.Name, nilIfEmpty(in.ContactName), nilIfEmpty(in.Phone), nilIfEmpty(in.Email), nilIfEmpty(in.Address), nilIfEmpty(in.Website), nilIfEmpty(in.Notes), nilIfEmpty(in.AcceptedCoverage), nilIfEmpty(in.BillingInfo), nilIfEmpty(in.ClaimInstructions), in.DefaultCoveragePercent, now, now, u.ID)
	if err != nil {
		writeError(w, 409, "PAYER_EXISTS", "Could not create insurer; verify the information.")
		return
	}
	s.audit(r.Context(), &u, "create", "payer", id, "Created insurer "+in.Name, "", "", r)
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) handlePayerUpdate(w http.ResponseWriter, r *http.Request) {
	var in payerPayload
	if decodeJSON(r, &in) != nil || in.Version < 1 {
		writeError(w, 400, "INVALID_PAYER", "Name and version are required.")
		return
	}
	if validation := validatePayer(&in); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	u, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), `UPDATE payers SET name=?,contact_name=?,phone=?,email=?,address=?,website=?,notes=?,accepted_coverage=?,billing_info=?,claim_instructions=?,default_coverage_percent=?,active=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`,
		in.Name, nilIfEmpty(in.ContactName), nilIfEmpty(in.Phone), nilIfEmpty(in.Email), nilIfEmpty(in.Address), nilIfEmpty(in.Website), nilIfEmpty(in.Notes), nilIfEmpty(in.AcceptedCoverage), nilIfEmpty(in.BillingInfo), nilIfEmpty(in.ClaimInstructions), in.DefaultCoveragePercent, in.Active, now, u.ID, chi.URLParam(r, "id"), in.Version)
	if err != nil {
		writeError(w, 500, "PAYER_UPDATE_FAILED", "Could not update insurer.")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 409, "CONCURRENT_MODIFICATION", "The insurer changed since it was opened.")
		return
	}
	s.audit(r.Context(), &u, "update", "payer", chi.URLParam(r, "id"), "Updated insurer", "", "", r)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleClaimsSummary counts the claim states the insurance screen headlines.
// Those counts are over every claim, so they are computed in SQL rather than by
// filtering whatever page the table happens to be showing.
func (s *Server) handleClaimsSummary(w http.ResponseWriter, r *http.Request) {
	var open, outstanding, overdue int
	err := s.db.QueryRowContext(r.Context(), `SELECT
		COUNT(*) FILTER (WHERE c.status NOT IN ('paid','rejected','cancelled')),
		COUNT(*) FILTER (WHERE c.payer_portion_minor - COALESCE(p.paid,0) > 0),
		COUNT(*) FILTER (WHERE c.payer_portion_minor - COALESCE(p.paid,0) > 0 AND julianday('now') - julianday(c.created_at) > 90)
		FROM insurance_claims c
		LEFT JOIN (SELECT claim_id, SUM(amount_minor) paid FROM insurance_claim_payments GROUP BY claim_id) p ON p.claim_id=c.id`).
		Scan(&open, &outstanding, &overdue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIMS_SUMMARY_FAILED", "Could not summarize insurance claims.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"openClaims": open, "outstandingClaims": outstanding, "overNinetyDays": overdue})
}

func (s *Server) handleClaimsList(w http.ResponseWriter, r *http.Request) {
	q := `SELECT c.id,p.medical_record_number,p.first_name||' '||p.last_name,c.patient_id,py.name,c.payer_id,COALESCE(i.invoice_number,''),COALESCE(c.invoice_id,''),COALESCE(c.authorization,''),COALESCE(c.member_number,''),COALESCE(c.policy_number,''),c.currency,c.exchange_rate,c.claim_amount_minor,c.patient_portion_minor,c.payer_portion_minor,COALESCE((SELECT SUM(amount_minor) FROM insurance_claim_payments WHERE claim_id=c.id),0),c.status,COALESCE(c.submitted_at,''),c.version,c.created_at,c.updated_at FROM insurance_claims c JOIN patients p ON p.id=c.patient_id JOIN payers py ON py.id=c.payer_id LEFT JOIN invoices i ON i.id=c.invoice_id WHERE (?='' OR c.status=?) ORDER BY c.updated_at DESC LIMIT ? OFFSET ?`
	status := r.URL.Query().Get("status")
	paging := paginationFrom(r, 50, 200)
	claimCount, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM insurance_claims c WHERE (?='' OR c.status=?)", status, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIMS_FAILED", "Could not load insurance claims.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), q, paging.Args(status, status)...)
	if err != nil {
		writeError(w, 500, "CLAIMS_FAILED", "Could not load insurance claims.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	now := time.Now()
	for rows.Next() {
		var id, mrn, name, patientID, payer, payerID, inv, invID, auth, member, policy, currency, exchangeRate, status, submitted, created, updated string
		var claim, patient, payerPart, paid int64
		var version int
		if err := rows.Scan(&id, &mrn, &name, &patientID, &payer, &payerID, &inv, &invID, &auth, &member, &policy, &currency, &exchangeRate, &claim, &patient, &payerPart, &paid, &status, &submitted, &version, &created, &updated); err != nil {
			writeError(w, http.StatusInternalServerError, "CLAIMS_FAILED", "Could not load insurance claims.")
			return
		}
		age := 0
		if t, e := time.Parse(time.RFC3339Nano, created); e == nil {
			age = int(now.Sub(t).Hours() / 24)
		}
		bucket := "0-30"
		if age > 90 {
			bucket = "90+"
		} else if age > 60 {
			bucket = "61-90"
		} else if age > 30 {
			bucket = "31-60"
		}
		items = append(items, map[string]any{"id": id, "medicalRecordNumber": mrn, "patientName": name, "patientId": patientID, "payerName": payer, "payerId": payerID, "invoiceNumber": inv, "invoiceId": invID, "authorization": auth, "memberNumber": member, "policyNumber": policy, "currency": currency, "exchangeRate": exchangeRate, "claimAmountMinor": claim, "patientPortionMinor": patient, "payerPortionMinor": payerPart, "paidMinor": paid, "outstandingMinor": payerPart - paid, "status": status, "submittedAt": submitted, "version": version, "createdAt": created, "updatedAt": updated, "agingBucket": bucket})
	}
	writeJSON(w, 200, withItems(items, paging.Meta(claimCount)))
}

type claimPayload struct {
	PatientID, PayerID, InvoiceID, Authorization, MemberNumber, PolicyNumber string
	// A claim may cover one line of an invoice rather than the whole bill.
	InvoiceItemID                                            string
	EncounterID, AppointmentID, PrescriptionID               string
	Currency, ExchangeRate                                   string
	ClaimAmountMinor, PatientPortionMinor, PayerPortionMinor int64
	Notes                                                    string
}

func (s *Server) handleClaimCreate(w http.ResponseWriter, r *http.Request) {
	var in claimPayload
	if decodeJSON(r, &in) != nil || in.PatientID == "" || in.PayerID == "" || in.ClaimAmountMinor <= 0 || in.PatientPortionMinor < 0 || in.PayerPortionMinor < 0 || in.PatientPortionMinor+in.PayerPortionMinor != in.ClaimAmountMinor {
		writeError(w, 422, "INVALID_CLAIM", "Patient and insurer are required, and patient plus insurer portions must equal the claim total.")
		return
	}
	if err := validatePatientLinks(r.Context(), s.db, in.PatientID,
		patientLink{"SELECT COALESCE(patient_id,'') FROM invoices WHERE id=? AND archived_at IS NULL", in.InvoiceID},
		patientLink{"SELECT patient_id FROM encounters WHERE id=? AND archived_at IS NULL", in.EncounterID},
		patientLink{"SELECT patient_id FROM appointments WHERE id=? AND archived_at IS NULL", in.AppointmentID},
		patientLink{"SELECT patient_id FROM prescriptions WHERE id=? AND archived_at IS NULL", in.PrescriptionID}); err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, 422, apiErr)
		} else {
			writeError(w, 500, "CLAIM_PATIENT_CHECK_FAILED", "Could not validate the claim patient.")
		}
		return
	}
	u, _ := userFromContext(r.Context())
	if in.Currency == "" {
		in.Currency = "HTG"
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.ExchangeRate == "" {
		in.ExchangeRate = "1"
	}
	if len(in.Currency) != 3 || !validExchangeRate(in.ExchangeRate) {
		writeError(w, 422, "INVALID_CURRENCY", "Currency and a positive exchange rate are required.")
		return
	}
	if in.InvoiceItemID != "" {
		var onInvoice int
		if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM invoice_items WHERE id=? AND invoice_id=?", in.InvoiceItemID, in.InvoiceID).Scan(&onInvoice); err != nil || onInvoice != 1 {
			writeError(w, 422, "INVOICE_ITEM_NOT_FOUND", "That line is not on the invoice being claimed for.")
			return
		}
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO insurance_claims(id,patient_id,payer_id,invoice_id,invoice_item_id,encounter_id,appointment_id,prescription_id,authorization,member_number,policy_number,currency,exchange_rate,claim_amount_minor,patient_portion_minor,payer_portion_minor,notes,status,version,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'draft',1,?,?,?,?)`,
		id, in.PatientID, in.PayerID, nilIfEmpty(in.InvoiceID), nilIfEmpty(in.InvoiceItemID), nilIfEmpty(in.EncounterID), nilIfEmpty(in.AppointmentID), nilIfEmpty(in.PrescriptionID), nilIfEmpty(in.Authorization), nilIfEmpty(in.MemberNumber), nilIfEmpty(in.PolicyNumber), in.Currency, in.ExchangeRate, in.ClaimAmountMinor, in.PatientPortionMinor, in.PayerPortionMinor, nilIfEmpty(in.Notes), now, now, u.ID, u.ID)
	if err != nil {
		writeError(w, 422, "CLAIM_CREATE_FAILED", "Could not create claim. Verify patient, insurer and invoice.")
		return
	}
	s.audit(r.Context(), &u, "create", "insurance_claim", id, "Created manual insurance claim", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_claim", EntityID: id})
	writeJSON(w, 201, map[string]any{"id": id})
}

const claimDetailColumns = `c.id,p.medical_record_number,p.first_name||' '||p.last_name,c.patient_id,py.name,c.payer_id,COALESCE(i.invoice_number,''),COALESCE(c.invoice_id,''),COALESCE(c.invoice_item_id,''),COALESCE(c.encounter_id,''),COALESCE(c.appointment_id,''),COALESCE(c.prescription_id,''),COALESCE(c.authorization,''),COALESCE(c.member_number,''),COALESCE(c.policy_number,''),c.currency,c.exchange_rate,c.claim_amount_minor,c.patient_portion_minor,c.payer_portion_minor,COALESCE(c.approved_amount_minor,-1),COALESCE((SELECT SUM(amount_minor) FROM insurance_claim_payments WHERE claim_id=c.id),0),c.status,COALESCE(c.submitted_at,''),COALESCE(c.response_date,''),COALESCE(c.notes,''),c.version,c.created_at,c.updated_at`

func scanClaimDetail(row interface{ Scan(...any) error }) (map[string]any, error) {
	var id, mrn, name, patientID, payer, payerID, inv, invID, invItemID, encounterID, appointmentID, prescriptionID, auth, member, policy, currency, exchangeRate, status, submitted, response, notes, created, updated string
	var claim, patient, payerPart, approved, paid int64
	var version int
	if err := row.Scan(&id, &mrn, &name, &patientID, &payer, &payerID, &inv, &invID, &invItemID, &encounterID, &appointmentID, &prescriptionID, &auth, &member, &policy, &currency, &exchangeRate, &claim, &patient, &payerPart, &approved, &paid, &status, &submitted, &response, &notes, &version, &created, &updated); err != nil {
		return nil, err
	}
	result := map[string]any{"id": id, "medicalRecordNumber": mrn, "patientName": name, "patientId": patientID, "payerName": payer, "payerId": payerID,
		"invoiceNumber": inv, "invoiceId": invID, "invoiceItemId": invItemID, "encounterId": encounterID, "appointmentId": appointmentID, "prescriptionId": prescriptionID,
		"authorization": auth, "memberNumber": member, "policyNumber": policy, "currency": currency, "exchangeRate": exchangeRate,
		"claimAmountMinor": claim, "patientPortionMinor": patient, "payerPortionMinor": payerPart, "paidMinor": paid, "outstandingMinor": payerPart - paid,
		"status": status, "submittedAt": submitted, "responseDate": response, "notes": notes, "version": version, "createdAt": created, "updatedAt": updated}
	if approved >= 0 {
		result["approvedAmountMinor"] = approved
		result["partiallyApproved"] = approved < claim
	} else {
		result["approvedAmountMinor"] = nil
		result["partiallyApproved"] = false
	}
	return result, nil
}

func (s *Server) handleClaimGet(w http.ResponseWriter, r *http.Request) {
	item, err := scanClaimDetail(s.db.QueryRowContext(r.Context(), `SELECT `+claimDetailColumns+` FROM insurance_claims c JOIN patients p ON p.id=c.patient_id JOIN payers py ON py.id=c.payer_id LEFT JOIN invoices i ON i.id=c.invoice_id WHERE c.id=?`, chi.URLParam(r, "id")))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "CLAIM_NOT_FOUND", "Insurance claim was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIM_LOAD_FAILED", "Could not load the claim.")
		return
	}
	documents, err := s.listInsuranceDocuments(r.Context(), "claim_id", chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIM_LOAD_FAILED", "Could not load claim documents.")
		return
	}
	item["documents"] = documents
	writeJSON(w, http.StatusOK, item)
}

// handleClaimUpdate lets staff correct a claim's own figures and links while
// it is still a draft. Once submitted, the paper trail (what was actually
// filed with the insurer) should not silently change underneath it.
func (s *Server) handleClaimUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in struct {
		claimPayload
		Version int
	}
	if decodeJSON(r, &in) != nil || in.Version < 1 || in.ClaimAmountMinor <= 0 || in.PatientPortionMinor < 0 || in.PayerPortionMinor < 0 || in.PatientPortionMinor+in.PayerPortionMinor != in.ClaimAmountMinor {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CLAIM", "Patient plus insurer portions must equal the claim total, and a current version is required.")
		return
	}
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM insurance_claims WHERE id=?", id).Scan(&status); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "CLAIM_NOT_FOUND", "Insurance claim was not found.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIM_UPDATE_FAILED", "Could not update claim.")
		return
	}
	if status != "draft" {
		writeError(w, http.StatusUnprocessableEntity, "CLAIM_NOT_DRAFT", "Only a draft claim's own figures can be edited; cancel and recreate it otherwise.")
		return
	}
	u, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), `UPDATE insurance_claims SET payer_id=?,invoice_id=?,invoice_item_id=?,encounter_id=?,appointment_id=?,prescription_id=?,authorization=?,member_number=?,policy_number=?,claim_amount_minor=?,patient_portion_minor=?,payer_portion_minor=?,notes=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status='draft'`,
		in.PayerID, nilIfEmpty(in.InvoiceID), nilIfEmpty(in.InvoiceItemID), nilIfEmpty(in.EncounterID), nilIfEmpty(in.AppointmentID), nilIfEmpty(in.PrescriptionID), nilIfEmpty(in.Authorization), nilIfEmpty(in.MemberNumber), nilIfEmpty(in.PolicyNumber), in.ClaimAmountMinor, in.PatientPortionMinor, in.PayerPortionMinor, nilIfEmpty(in.Notes), now, u.ID, id, in.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIM_UPDATE_FAILED", "Could not update claim.")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The claim changed since it was opened.")
		return
	}
	s.audit(r.Context(), &u, "update", "insurance_claim", id, "Updated draft insurance claim", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_claim", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": in.Version + 1})
}

func (s *Server) handleClaimStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status              string
		ApprovedAmountMinor *int64
		Version             int
	}
	if decodeJSON(r, &in) != nil || !claimStatuses[in.Status] || in.Version < 1 {
		writeError(w, 400, "INVALID_STATUS", "A valid status and version are required.")
		return
	}
	id := chi.URLParam(r, "id")
	u, _ := userFromContext(r.Context())
	var current string
	var claimAmount int64
	if err := s.db.QueryRowContext(r.Context(), "SELECT status,claim_amount_minor FROM insurance_claims WHERE id=?", id).Scan(&current, &claimAmount); err != nil {
		writeError(w, 404, "CLAIM_NOT_FOUND", "Insurance claim was not found.")
		return
	}
	if (current == "paid" || current == "rejected" || current == "cancelled") && in.Status != current {
		writeError(w, 422, "CLAIM_CLOSED", "A closed claim cannot be reopened; create a new claim if needed.")
		return
	}
	if in.ApprovedAmountMinor != nil && (*in.ApprovedAmountMinor < 0 || *in.ApprovedAmountMinor > claimAmount) {
		writeError(w, 422, "INVALID_APPROVED_AMOUNT", "The approved amount cannot be negative or exceed the claim amount.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var submitted, response any
	if in.Status == "submitted" {
		submitted = now
	}
	if in.Status == "approved" || in.Status == "rejected" {
		response = now
	}
	var approved any
	if in.ApprovedAmountMinor != nil {
		approved = *in.ApprovedAmountMinor
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE insurance_claims SET status=?,submitted_at=COALESCE(submitted_at,?),response_date=COALESCE(response_date,?),approved_amount_minor=COALESCE(?,approved_amount_minor),version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, in.Status, submitted, response, approved, now, u.ID, id, in.Version)
	if err != nil {
		writeError(w, 500, "CLAIM_UPDATE_FAILED", "Could not update claim.")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 409, "CONCURRENT_MODIFICATION", "The claim changed since it was opened.")
		return
	}
	s.audit(r.Context(), &u, "status", "insurance_claim", id, "Changed claim status to "+in.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_claim", EntityID: id})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleClaimPayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AmountMinor                   int64
		PaymentDate, Reference, Notes string
	}
	if decodeJSON(r, &in) != nil || in.AmountMinor <= 0 || strings.TrimSpace(in.PaymentDate) == "" {
		writeError(w, 400, "INVALID_PAYMENT", "Positive amount and payment date are required.")
		return
	}
	u, _ := userFromContext(r.Context())
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	claimID := chi.URLParam(r, "id")
	var invoiceID string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var portion, paid int64
		var approvedAmount sql.NullInt64
		var claimStatus string
		if err := tx.QueryRowContext(r.Context(), `SELECT payer_portion_minor,approved_amount_minor,COALESCE((SELECT SUM(amount_minor) FROM insurance_claim_payments WHERE claim_id=?),0),status,COALESCE(invoice_id,'') FROM insurance_claims WHERE id=?`, claimID, claimID).Scan(&portion, &approvedAmount, &paid, &claimStatus, &invoiceID); err != nil {
			return err
		}
		if claimStatus != "approved" && claimStatus != "partially_paid" {
			return &APIError{Code: "CLAIM_NOT_APPROVED", Message: "Approve the claim before recording an insurer payment."}
		}
		// A payment can never collect more than what the insurer actually
		// approved. Absent an explicit approved amount, the claim's own
		// payer portion stands in as an implicit full approval.
		ceiling := portion
		if approvedAmount.Valid {
			ceiling = approvedAmount.Int64
		}
		if paid > ceiling || in.AmountMinor > ceiling-paid {
			return &APIError{Code: "PAYMENT_EXCEEDS_CLAIM", Message: "Payment exceeds the insurer's approved and outstanding balance."}
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO insurance_claim_payments(id,claim_id,amount_minor,payment_date,reference,notes,created_at,created_by) VALUES(?,?,?,?,?,?,?,?)`, id, claimID, in.AmountMinor, in.PaymentDate, nilIfEmpty(in.Reference), nilIfEmpty(in.Notes), now, u.ID); err != nil {
			return err
		}
		status := "partially_paid"
		if paid+in.AmountMinor >= ceiling {
			status = "paid"
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE insurance_claims SET amount_paid_minor=?,status=?,version=version+1,updated_at=?,updated_by=? WHERE id=?`, paid+in.AmountMinor, status, now, u.ID, claimID); err != nil {
			return err
		}
		// The invoice's own balance must reflect this remittance too, or it
		// stays "partially_paid" forever even once the patient and the
		// insurer have between them settled the whole bill.
		return s.syncInvoiceStatusAfterInsurancePayment(r.Context(), tx, invoiceID, u.ID, now)
	})
	if err != nil {
		if e, ok := err.(*APIError); ok {
			writeJSON(w, 422, e)
		} else {
			writeError(w, 500, "CLAIM_PAYMENT_FAILED", "Could not record insurer payment.")
		}
		return
	}
	s.audit(r.Context(), &u, "payment", "insurance_claim", claimID, "Recorded manual insurer payment", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_claim", EntityID: claimID})
	if invoiceID != "" {
		s.broker.Publish(realtime.Event{Type: "invoice.paid", EntityType: "invoice", EntityID: invoiceID})
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

type patientPolicyPayload struct {
	PayerID                  string  `json:"payerId"`
	PayerName                string  `json:"payerName"`
	MemberNumber             string  `json:"memberNumber"`
	PolicyNumber             string  `json:"policyNumber"`
	GroupNumber              string  `json:"groupNumber"`
	SubscriberName           string  `json:"subscriberName"`
	RelationshipToSubscriber string  `json:"relationshipToSubscriber"`
	Authorization            string  `json:"authorization"`
	CoveragePercent          float64 `json:"coveragePercent"`
	EffectiveDate            string  `json:"effectiveDate"`
	ExpirationDate           string  `json:"expirationDate"`
	CoverageNotes            string  `json:"coverageNotes"`
	Notes                    string  `json:"notes"`
	IsPrimary                bool    `json:"isPrimary"`
	Version                  int     `json:"version"`
}

var relationshipOptions = map[string]bool{"": true, "self": true, "spouse": true, "child": true, "other": true}
var verificationStatuses = map[string]bool{"not_verified": true, "pending_verification": true, "verified": true, "expired": true, "rejected": true}

func validatePatientPolicy(input *patientPolicyPayload) *APIError {
	input.PayerName = strings.TrimSpace(input.PayerName)
	input.SubscriberName = strings.TrimSpace(input.SubscriberName)
	if input.PayerName == "" || input.CoveragePercent < 0 || input.CoveragePercent > 100 {
		return &APIError{Code: "INVALID_POLICY", Message: "An insurer and a coverage between 0 and 100 percent are required."}
	}
	if !relationshipOptions[input.RelationshipToSubscriber] {
		return &APIError{Code: "INVALID_RELATIONSHIP", Message: "Relationship to subscriber is invalid."}
	}
	return nil
}

const patientPolicyColumns = `id,patient_id,COALESCE(payer_id,''),payer_name,COALESCE(member_number,''),COALESCE(policy_number,''),COALESCE(group_number,''),COALESCE(subscriber_name,''),COALESCE(relationship_to_subscriber,''),COALESCE(authorization,''),coverage_percent,COALESCE(effective_date,''),COALESCE(expiration_date,''),COALESCE(coverage_notes,''),COALESCE(notes,''),verification_status,COALESCE(verified_by,''),COALESCE(verified_at,''),COALESCE(verification_reference,''),COALESCE(verification_contact,''),COALESCE(verification_notes,''),is_primary,is_active,version,created_at,updated_at`

func scanPatientPolicy(row interface{ Scan(...any) error }) (map[string]any, error) {
	var id, patientID, payerID, payerName, member, policy, group, subscriber, relationship, authorization, effective, expiration, coverageNotes, notes, verificationStatus, verifiedBy, verifiedAt, verificationReference, verificationContact, verificationNotes, created, updated string
	var coverage float64
	var primary, active bool
	var version int
	if err := row.Scan(&id, &patientID, &payerID, &payerName, &member, &policy, &group, &subscriber, &relationship, &authorization, &coverage, &effective, &expiration, &coverageNotes, &notes, &verificationStatus, &verifiedBy, &verifiedAt, &verificationReference, &verificationContact, &verificationNotes, &primary, &active, &version, &created, &updated); err != nil {
		return nil, err
	}
	expired := expiration != "" && expiration < time.Now().UTC().Format("2006-01-02")
	displayStatus := verificationStatus
	if expired && displayStatus == "verified" {
		displayStatus = "expired"
	}
	return map[string]any{
		"id": id, "patientId": patientID, "payerId": payerID, "payerName": payerName, "memberNumber": member, "policyNumber": policy,
		"groupNumber": group, "subscriberName": subscriber, "relationshipToSubscriber": relationship,
		"authorization": authorization, "coveragePercent": coverage, "effectiveDate": effective, "expirationDate": expiration,
		"coverageNotes": coverageNotes, "notes": notes,
		"verificationStatus": displayStatus, "verifiedBy": verifiedBy, "verifiedAt": verifiedAt, "verificationReference": verificationReference,
		"verificationContact": verificationContact, "verificationNotes": verificationNotes,
		"isPrimary": primary, "isActive": active, "expired": expired, "version": version, "createdAt": created, "updatedAt": updated,
	}, nil
}

// handlePatientPoliciesList reads what a patient is covered by, including how
// much of a bill each policy takes.
func (s *Server) handlePatientPoliciesList(w http.ResponseWriter, r *http.Request) {
	patientID := strings.TrimSpace(r.URL.Query().Get("patientId"))
	if patientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A patient is required.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+patientPolicyColumns+` FROM patient_insurance WHERE patient_id=? ORDER BY is_active DESC, is_primary DESC, payer_name`, patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICIES_FAILED", "Could not load the patient's insurance.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item, err := scanPatientPolicy(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "POLICIES_FAILED", "Could not load the patient's insurance.")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePatientPolicySave(w http.ResponseWriter, r *http.Request) {
	patientID := strings.TrimSpace(chi.URLParam(r, "patientId"))
	var input patientPolicyPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if input.PayerID != "" && strings.TrimSpace(input.PayerName) == "" {
		// A named insurer keeps its own name on the policy, so a policy still
		// reads correctly if the insurer record is later renamed or retired.
		_ = s.db.QueryRowContext(r.Context(), "SELECT name FROM payers WHERE id=?", input.PayerID).Scan(&input.PayerName)
	}
	if validation := validatePatientPolicy(&input); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM patients WHERE id=? AND archived_at IS NULL", patientID).Scan(&exists); err != nil || exists != 1 {
		writeError(w, http.StatusUnprocessableEntity, "PATIENT_NOT_FOUND", "That patient was not found.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if input.IsPrimary {
			if _, err := tx.ExecContext(r.Context(), `UPDATE patient_insurance SET is_primary=0 WHERE patient_id=? AND is_primary=1 AND is_active=1`, patientID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(r.Context(), `INSERT INTO patient_insurance(id,patient_id,payer_id,payer_name,member_number,policy_number,group_number,subscriber_name,relationship_to_subscriber,authorization,coverage_percent,effective_date,expiration_date,coverage_notes,notes,is_primary,is_active,created_at,updated_at,updated_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?)`, id, patientID, nilIfEmpty(input.PayerID), input.PayerName, nilIfEmpty(input.MemberNumber), nilIfEmpty(input.PolicyNumber),
			nilIfEmpty(input.GroupNumber), nilIfEmpty(input.SubscriberName), nilIfEmpty(input.RelationshipToSubscriber), nilIfEmpty(input.Authorization), input.CoveragePercent,
			nilIfEmpty(input.EffectiveDate), nilIfEmpty(input.ExpirationDate), nilIfEmpty(input.CoverageNotes), nilIfEmpty(input.Notes), boolInt(input.IsPrimary), now, now, user.ID)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICY_SAVE_FAILED", "Could not record the policy.")
		return
	}
	s.audit(r.Context(), &user, "create", "patient_insurance", id, "Recorded insurance policy with "+input.PayerName, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "payerName": input.PayerName, "coveragePercent": input.CoveragePercent, "version": 1})
}

func (s *Server) handlePatientPolicyUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input patientPolicyPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Current policy version is required.")
		return
	}
	if input.PayerID != "" && strings.TrimSpace(input.PayerName) == "" {
		_ = s.db.QueryRowContext(r.Context(), "SELECT name FROM payers WHERE id=?", input.PayerID).Scan(&input.PayerName)
	}
	if validation := validatePatientPolicy(&input); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), `UPDATE patient_insurance SET payer_id=?,payer_name=?,member_number=?,policy_number=?,group_number=?,subscriber_name=?,relationship_to_subscriber=?,authorization=?,coverage_percent=?,effective_date=?,expiration_date=?,coverage_notes=?,notes=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`,
		nilIfEmpty(input.PayerID), input.PayerName, nilIfEmpty(input.MemberNumber), nilIfEmpty(input.PolicyNumber), nilIfEmpty(input.GroupNumber), nilIfEmpty(input.SubscriberName), nilIfEmpty(input.RelationshipToSubscriber),
		nilIfEmpty(input.Authorization), input.CoveragePercent, nilIfEmpty(input.EffectiveDate), nilIfEmpty(input.ExpirationDate), nilIfEmpty(input.CoverageNotes), nilIfEmpty(input.Notes), now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICY_UPDATE_FAILED", "Could not update the policy.")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This policy changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "update", "patient_insurance", id, "Updated insurance policy", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

// handlePatientPolicySetPrimary promotes one active policy to primary and
// demotes whichever policy held that place before, inside one transaction —
// the unique index on (patient_id) WHERE is_primary=1 AND is_active=1 backs
// this up at the database level regardless of what application code does.
func (s *Server) handlePatientPolicySetPrimary(w http.ResponseWriter, r *http.Request) {
	var input struct{ Version int }
	if decodeJSON(r, &input) != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Current policy version is required.")
		return
	}
	id := chi.URLParam(r, "id")
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var patientID string
		var active bool
		if err := tx.QueryRowContext(r.Context(), "SELECT patient_id,is_active FROM patient_insurance WHERE id=?", id).Scan(&patientID, &active); err != nil {
			return err
		}
		if !active {
			return &APIError{Code: "POLICY_INACTIVE", Message: "An inactive policy cannot be marked primary; reactivate it first."}
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE patient_insurance SET is_primary=0 WHERE patient_id=? AND is_primary=1 AND is_active=1 AND id<>?`, patientID, id); err != nil {
			return err
		}
		result, err := tx.ExecContext(r.Context(), `UPDATE patient_insurance SET is_primary=1,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, now, user.ID, id, input.Version)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errConcurrentModification
		}
		return nil
	})
	if err != nil {
		if err == errConcurrentModification {
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This policy changed since it was opened.")
			return
		}
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "POLICY_NOT_FOUND", "That policy was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "POLICY_UPDATE_FAILED", "Could not set the primary policy.")
		return
	}
	s.audit(r.Context(), &user, "set_primary", "patient_insurance", id, "Marked insurance policy as primary", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

// handlePatientPolicyActive sets a policy active or inactive. Deactivating
// also clears primary status — a deactivated policy is not the patient's
// active coverage, so it cannot remain flagged as the primary one.
func (s *Server) handlePatientPolicyActive(w http.ResponseWriter, r *http.Request, active bool) {
	var input struct{ Version int }
	if decodeJSON(r, &input) != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Current policy version is required.")
		return
	}
	id := chi.URLParam(r, "id")
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	primaryClause := ""
	if !active {
		primaryClause = ",is_primary=0"
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE patient_insurance SET is_active=?`+primaryClause+`,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, boolInt(active), now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICY_UPDATE_FAILED", "Could not update the policy.")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This policy changed since it was opened.")
		return
	}
	action := "deactivate"
	description := "Deactivated insurance policy"
	if active {
		action, description = "reactivate", "Reactivated insurance policy"
	}
	s.audit(r.Context(), &user, action, "patient_insurance", id, description, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

func (s *Server) handlePatientPolicyDeactivate(w http.ResponseWriter, r *http.Request) {
	s.handlePatientPolicyActive(w, r, false)
}

func (s *Server) handlePatientPolicyReactivate(w http.ResponseWriter, r *http.Request) {
	s.handlePatientPolicyActive(w, r, true)
}

// handlePatientPolicyDelete removes a policy recorded in error. A policy with
// an insurance card or a document already attached keeps its history through
// deactivation instead — a hard delete would orphan those records.
func (s *Server) handlePatientPolicyDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := r.URL.Query().Get("version")
	if version == "" {
		writeError(w, http.StatusBadRequest, "VERSION_REQUIRED", "Current policy version is required.")
		return
	}
	var cards, documents int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM patient_insurance_cards WHERE patient_insurance_id=?", id).Scan(&cards)
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM insurance_documents WHERE patient_insurance_id=? AND archived_at IS NULL", id).Scan(&documents)
	if cards > 0 || documents > 0 {
		writeError(w, http.StatusUnprocessableEntity, "POLICY_IN_USE", "Remove the insurance card and any documents first, or deactivate this policy instead of deleting it.")
		return
	}
	user, _ := userFromContext(r.Context())
	res, err := s.db.ExecContext(r.Context(), "DELETE FROM patient_insurance WHERE id=? AND version=?", id, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICY_DELETE_FAILED", "Could not remove the policy.")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This policy changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "delete", "patient_insurance", id, "Removed insurance policy", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}

// handlePatientPolicyVerify records a manual verification decision. This is
// deliberately staff-entered: nothing here should read as an electronic
// eligibility check unless a real insurer/API integration exists.
func (s *Server) handlePatientPolicyVerify(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Contact   string `json:"contact"`
		Notes     string `json:"notes"`
		Version   int    `json:"version"`
	}
	if decodeJSON(r, &input) != nil || !verificationStatuses[input.Status] || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid verification status and current version are required.")
		return
	}
	id := chi.URLParam(r, "id")
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), `UPDATE patient_insurance SET verification_status=?,verified_by=?,verified_at=?,verification_reference=?,verification_contact=?,verification_notes=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`,
		input.Status, user.ID, now, nilIfEmpty(input.Reference), nilIfEmpty(input.Contact), nilIfEmpty(input.Notes), now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICY_VERIFY_FAILED", "Could not record verification.")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This policy changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "verify", "patient_insurance", id, "Recorded insurance verification: "+input.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status, "version": input.Version + 1})
}

// handleClaimProposal works out what a claim for this bill should say: the
// patient's policy, what the insurer's percentage comes to, and what would be
// left for the patient after everything already paid or claimed. It proposes;
// it never files anything, and the figures stay editable.
func (s *Server) handleClaimProposal(w http.ResponseWriter, r *http.Request) {
	patientID := strings.TrimSpace(r.URL.Query().Get("patientId"))
	invoiceID := strings.TrimSpace(r.URL.Query().Get("invoiceId"))
	invoiceItemID := strings.TrimSpace(r.URL.Query().Get("invoiceItemId"))
	if patientID == "" || invoiceID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A patient and an invoice are required.")
		return
	}
	var owner, currency, exchangeRate string
	var invoiceTotal int64
	switch err := s.db.QueryRowContext(r.Context(), "SELECT COALESCE(patient_id,''),currency,exchange_rate,total_minor FROM invoices WHERE id=? AND archived_at IS NULL", invoiceID).
		Scan(&owner, &currency, &exchangeRate, &invoiceTotal); {
	case err == sql.ErrNoRows:
		writeError(w, http.StatusUnprocessableEntity, "INVOICE_NOT_FOUND", "That invoice was not found.")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "CLAIM_PROPOSAL_FAILED", "Could not read the invoice.")
		return
	case owner != "" && owner != patientID:
		writeError(w, http.StatusUnprocessableEntity, "INVOICE_PATIENT_MISMATCH", "That invoice belongs to a different patient.")
		return
	}
	// A claim can be for one line rather than the whole bill.
	claimable, lineDescription := invoiceTotal, ""
	if invoiceItemID != "" {
		if err := s.db.QueryRowContext(r.Context(), "SELECT line_total_minor,description FROM invoice_items WHERE id=? AND invoice_id=?", invoiceItemID, invoiceID).Scan(&claimable, &lineDescription); err == sql.ErrNoRows {
			writeError(w, http.StatusUnprocessableEntity, "INVOICE_ITEM_NOT_FOUND", "That line is not on this invoice.")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "CLAIM_PROPOSAL_FAILED", "Could not read the invoice line.")
			return
		}
	}
	var payerID, payerName, member, policy, authorization string
	var coverage float64
	policyFound := true
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(pi.payer_id,''),pi.payer_name,COALESCE(pi.member_number,''),COALESCE(pi.policy_number,''),COALESCE(pi.authorization,''),
		CASE WHEN pi.coverage_percent>0 THEN pi.coverage_percent ELSE COALESCE(p.default_coverage_percent,0) END
		FROM patient_insurance pi LEFT JOIN payers p ON p.id=pi.payer_id WHERE pi.patient_id=? ORDER BY pi.is_primary DESC LIMIT 1`, patientID).
		Scan(&payerID, &payerName, &member, &policy, &authorization, &coverage); err == sql.ErrNoRows {
		policyFound = false
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "CLAIM_PROPOSAL_FAILED", "Could not read the patient's insurance.")
		return
	}
	// Anything already claimed against this invoice is not claimable twice.
	var alreadyClaimed int64
	_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(claim_amount_minor),0) FROM insurance_claims WHERE invoice_id=? AND status<>'cancelled'", invoiceID).Scan(&alreadyClaimed)
	remaining := claimable - alreadyClaimed
	if remaining < 0 {
		remaining = 0
	}
	payerPortion, _ := splitMinor(remaining, coverage)
	writeJSON(w, http.StatusOK, map[string]any{
		"invoiceId": invoiceID, "invoiceItemId": invoiceItemID, "lineDescription": lineDescription,
		"currency": currency, "exchangeRate": exchangeRate,
		"invoiceTotalMinor": invoiceTotal, "claimableMinor": claimable, "alreadyClaimedMinor": alreadyClaimed,
		"claimAmountMinor": remaining, "payerPortionMinor": payerPortion, "patientPortionMinor": remaining - payerPortion,
		"policyFound": policyFound,
		"policy":      map[string]any{"payerId": payerID, "payerName": payerName, "memberNumber": member, "policyNumber": policy, "authorization": authorization, "coveragePercent": coverage},
	})
}
