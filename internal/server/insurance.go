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

func (s *Server) handlePayersList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,name,COALESCE(contact_name,''),COALESCE(phone,''),COALESCE(email,''),COALESCE(address,''),active,version,created_at,updated_at FROM payers ORDER BY active DESC,name`)
	if err != nil {
		writeError(w, 500, "PAYERS_FAILED", "Could not load insurers.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, contact, phone, email, address, created, updated string
		var active bool
		var version int
		if err := rows.Scan(&id, &name, &contact, &phone, &email, &address, &active, &version, &created, &updated); err != nil {
			writeError(w, http.StatusInternalServerError, "PAYERS_FAILED", "Could not load insurers.")
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "contactName": contact, "phone": phone, "email": email, "address": address, "active": active, "version": version, "createdAt": created, "updatedAt": updated})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handlePayerCreate(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, ContactName, Phone, Email, Address string }
	if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" {
		writeError(w, 400, "INVALID_PAYER", "Insurer name is required.")
		return
	}
	u, _ := userFromContext(r.Context())
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO payers(id,name,contact_name,phone,email,address,active,version,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,1,1,?,?,?)`, id, strings.TrimSpace(in.Name), nilIfEmpty(in.ContactName), nilIfEmpty(in.Phone), nilIfEmpty(in.Email), nilIfEmpty(in.Address), now, now, u.ID)
	if err != nil {
		writeError(w, 409, "PAYER_EXISTS", "Could not create insurer; verify the information.")
		return
	}
	s.audit(r.Context(), &u, "create", "payer", id, "Created insurer "+in.Name, "", "", r)
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) handlePayerUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name, ContactName, Phone, Email, Address string
		Active                                   bool
		Version                                  int
	}
	if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" || in.Version < 1 {
		writeError(w, 400, "INVALID_PAYER", "Name and version are required.")
		return
	}
	u, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), `UPDATE payers SET name=?,contact_name=?,phone=?,email=?,address=?,active=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, strings.TrimSpace(in.Name), nilIfEmpty(in.ContactName), nilIfEmpty(in.Phone), nilIfEmpty(in.Email), nilIfEmpty(in.Address), in.Active, now, u.ID, chi.URLParam(r, "id"), in.Version)
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

func (s *Server) handleClaimCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PatientID, PayerID, InvoiceID, Authorization, MemberNumber, PolicyNumber string
		// A claim may cover one line of an invoice rather than the whole bill.
		InvoiceItemID                                            string
		Currency, ExchangeRate                                   string
		ClaimAmountMinor, PatientPortionMinor, PayerPortionMinor int64
	}
	if decodeJSON(r, &in) != nil || in.PatientID == "" || in.PayerID == "" || in.ClaimAmountMinor <= 0 || in.PatientPortionMinor < 0 || in.PayerPortionMinor < 0 || in.PatientPortionMinor+in.PayerPortionMinor != in.ClaimAmountMinor {
		writeError(w, 422, "INVALID_CLAIM", "Patient and insurer are required, and patient plus insurer portions must equal the claim total.")
		return
	}
	if err := validatePatientLinks(r.Context(), s.db, in.PatientID, patientLink{"SELECT COALESCE(patient_id,'') FROM invoices WHERE id=? AND archived_at IS NULL", in.InvoiceID}); err != nil {
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
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO insurance_claims(id,patient_id,payer_id,invoice_id,invoice_item_id,authorization,member_number,policy_number,currency,exchange_rate,claim_amount_minor,patient_portion_minor,payer_portion_minor,status,version,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?, 'draft',1,?,?,?,?)`, id, in.PatientID, in.PayerID, nilIfEmpty(in.InvoiceID), nilIfEmpty(in.InvoiceItemID), nilIfEmpty(in.Authorization), nilIfEmpty(in.MemberNumber), nilIfEmpty(in.PolicyNumber), in.Currency, in.ExchangeRate, in.ClaimAmountMinor, in.PatientPortionMinor, in.PayerPortionMinor, now, now, u.ID, u.ID)
	if err != nil {
		writeError(w, 422, "CLAIM_CREATE_FAILED", "Could not create claim. Verify patient, insurer and invoice.")
		return
	}
	s.audit(r.Context(), &u, "create", "insurance_claim", id, "Created manual insurance claim", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_claim", EntityID: id})
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) handleClaimStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status  string
		Version int
	}
	if decodeJSON(r, &in) != nil || !claimStatuses[in.Status] || in.Version < 1 {
		writeError(w, 400, "INVALID_STATUS", "A valid status and version are required.")
		return
	}
	u, _ := userFromContext(r.Context())
	var current string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM insurance_claims WHERE id=?", chi.URLParam(r, "id")).Scan(&current); err != nil {
		writeError(w, 404, "CLAIM_NOT_FOUND", "Insurance claim was not found.")
		return
	}
	if (current == "paid" || current == "rejected" || current == "cancelled") && in.Status != current {
		writeError(w, 422, "CLAIM_CLOSED", "A closed claim cannot be reopened; create a new claim if needed.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var submitted any
	if in.Status == "submitted" {
		submitted = now
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE insurance_claims SET status=?,submitted_at=COALESCE(submitted_at,?),version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, in.Status, submitted, now, u.ID, chi.URLParam(r, "id"), in.Version)
	if err != nil {
		writeError(w, 500, "CLAIM_UPDATE_FAILED", "Could not update claim.")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 409, "CONCURRENT_MODIFICATION", "The claim changed since it was opened.")
		return
	}
	s.audit(r.Context(), &u, "status", "insurance_claim", chi.URLParam(r, "id"), "Changed claim status to "+in.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_claim", EntityID: chi.URLParam(r, "id")})
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
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var portion, paid int64
		var claimStatus string
		if err := tx.QueryRowContext(r.Context(), `SELECT payer_portion_minor,COALESCE((SELECT SUM(amount_minor) FROM insurance_claim_payments WHERE claim_id=?),0),status FROM insurance_claims WHERE id=?`, claimID, claimID).Scan(&portion, &paid, &claimStatus); err != nil {
			return err
		}
		if claimStatus != "approved" && claimStatus != "partially_paid" {
			return &APIError{Code: "CLAIM_NOT_APPROVED", Message: "Approve the claim before recording an insurer payment."}
		}
		if paid > portion || in.AmountMinor > portion-paid {
			return &APIError{Code: "PAYMENT_EXCEEDS_CLAIM", Message: "Payment exceeds the insurer outstanding balance."}
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO insurance_claim_payments(id,claim_id,amount_minor,payment_date,reference,notes,created_at,created_by) VALUES(?,?,?,?,?,?,?,?)`, id, claimID, in.AmountMinor, in.PaymentDate, nilIfEmpty(in.Reference), nilIfEmpty(in.Notes), now, u.ID); err != nil {
			return err
		}
		status := "partially_paid"
		if paid+in.AmountMinor == portion {
			status = "paid"
		}
		_, err := tx.ExecContext(r.Context(), `UPDATE insurance_claims SET amount_paid_minor=?,status=?,version=version+1,updated_at=?,updated_by=? WHERE id=?`, paid+in.AmountMinor, status, now, u.ID, claimID)
		return err
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
	writeJSON(w, 201, map[string]any{"id": id})
}

type patientPolicyPayload struct {
	PayerID         string  `json:"payerId"`
	PayerName       string  `json:"payerName"`
	MemberNumber    string  `json:"memberNumber"`
	PolicyNumber    string  `json:"policyNumber"`
	Authorization   string  `json:"authorization"`
	CoveragePercent float64 `json:"coveragePercent"`
	IsPrimary       bool    `json:"isPrimary"`
	Version         int     `json:"version"`
}

// handlePatientPoliciesList reads what a patient is covered by, including how
// much of a bill each policy takes.
func (s *Server) handlePatientPoliciesList(w http.ResponseWriter, r *http.Request) {
	patientID := strings.TrimSpace(r.URL.Query().Get("patientId"))
	if patientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A patient is required.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,COALESCE(payer_id,''),payer_name,COALESCE(member_number,''),COALESCE(policy_number,''),COALESCE(authorization,''),coverage_percent,is_primary,version
		FROM patient_insurance WHERE patient_id=? ORDER BY is_primary DESC, payer_name`, patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICIES_FAILED", "Could not load the patient's insurance.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, payerID, payerName, member, policy, authorization string
		var coverage float64
		var primary bool
		var version int
		if err := rows.Scan(&id, &payerID, &payerName, &member, &policy, &authorization, &coverage, &primary, &version); err != nil {
			writeError(w, http.StatusInternalServerError, "POLICIES_FAILED", "Could not load the patient's insurance.")
			return
		}
		items = append(items, map[string]any{"id": id, "payerId": payerID, "payerName": payerName, "memberNumber": member, "policyNumber": policy,
			"authorization": authorization, "coveragePercent": coverage, "isPrimary": primary, "version": version})
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
	input.PayerName = strings.TrimSpace(input.PayerName)
	if input.PayerID != "" && input.PayerName == "" {
		// A named insurer keeps its own name on the policy, so a policy still
		// reads correctly if the insurer record is later renamed or retired.
		_ = s.db.QueryRowContext(r.Context(), "SELECT name FROM payers WHERE id=?", input.PayerID).Scan(&input.PayerName)
	}
	if input.PayerName == "" || input.CoveragePercent < 0 || input.CoveragePercent > 100 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_POLICY", "An insurer and a coverage between 0 and 100 percent are required.")
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM patients WHERE id=? AND archived_at IS NULL", patientID).Scan(&exists); err != nil || exists != 1 {
		writeError(w, http.StatusUnprocessableEntity, "PATIENT_NOT_FOUND", "That patient was not found.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO patient_insurance(id,patient_id,payer_id,payer_name,member_number,policy_number,authorization,coverage_percent,is_primary,created_at,updated_at,updated_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, patientID, nilIfEmpty(input.PayerID), input.PayerName, nilIfEmpty(input.MemberNumber), nilIfEmpty(input.PolicyNumber),
		nilIfEmpty(input.Authorization), input.CoveragePercent, boolInt(input.IsPrimary), now, now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POLICY_SAVE_FAILED", "Could not record the policy.")
		return
	}
	s.audit(r.Context(), &user, "create", "patient_insurance", id, "Recorded insurance policy with "+input.PayerName, "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "payerName": input.PayerName, "coveragePercent": input.CoveragePercent, "version": 1})
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
