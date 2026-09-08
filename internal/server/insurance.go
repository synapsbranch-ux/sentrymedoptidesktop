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

func (s *Server) handleClaimsList(w http.ResponseWriter, r *http.Request) {
	q := `SELECT c.id,p.medical_record_number,p.first_name||' '||p.last_name,c.patient_id,py.name,c.payer_id,COALESCE(i.invoice_number,''),COALESCE(c.invoice_id,''),COALESCE(c.authorization,''),COALESCE(c.member_number,''),COALESCE(c.policy_number,''),c.currency,c.exchange_rate,c.claim_amount_minor,c.patient_portion_minor,c.payer_portion_minor,COALESCE((SELECT SUM(amount_minor) FROM insurance_claim_payments WHERE claim_id=c.id),0),c.status,COALESCE(c.submitted_at,''),c.version,c.created_at,c.updated_at FROM insurance_claims c JOIN patients p ON p.id=c.patient_id JOIN payers py ON py.id=c.payer_id LEFT JOIN invoices i ON i.id=c.invoice_id WHERE (?='' OR c.status=?) ORDER BY c.updated_at DESC`
	status := r.URL.Query().Get("status")
	rows, err := s.db.QueryContext(r.Context(), q, status, status)
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
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handleClaimCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PatientID, PayerID, InvoiceID, Authorization, MemberNumber, PolicyNumber string
		Currency, ExchangeRate string
		ClaimAmountMinor, PatientPortionMinor, PayerPortionMinor int64
	}
	if decodeJSON(r, &in) != nil || in.PatientID == "" || in.PayerID == "" || in.ClaimAmountMinor <= 0 || in.PatientPortionMinor < 0 || in.PayerPortionMinor < 0 || in.PatientPortionMinor+in.PayerPortionMinor != in.ClaimAmountMinor {
		writeError(w, 422, "INVALID_CLAIM", "Patient and insurer are required, and patient plus insurer portions must equal the claim total.")
		return
	}
	u, _ := userFromContext(r.Context())
	if in.Currency == "" { in.Currency = "HTG" }
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.ExchangeRate == "" { in.ExchangeRate = "1" }
	if len(in.Currency) != 3 || !validExchangeRate(in.ExchangeRate) { writeError(w, 422, "INVALID_CURRENCY", "Currency and a positive exchange rate are required."); return }
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO insurance_claims(id,patient_id,payer_id,invoice_id,authorization,member_number,policy_number,currency,exchange_rate,claim_amount_minor,patient_portion_minor,payer_portion_minor,status,version,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?, 'draft',1,?,?,?,?)`, id, in.PatientID, in.PayerID, nilIfEmpty(in.InvoiceID), nilIfEmpty(in.Authorization), nilIfEmpty(in.MemberNumber), nilIfEmpty(in.PolicyNumber), in.Currency, in.ExchangeRate, in.ClaimAmountMinor, in.PatientPortionMinor, in.PayerPortionMinor, now, now, u.ID, u.ID)
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
	if err:=s.db.QueryRowContext(r.Context(),"SELECT status FROM insurance_claims WHERE id=?",chi.URLParam(r,"id")).Scan(&current);err!=nil{writeError(w,404,"CLAIM_NOT_FOUND","Insurance claim was not found.");return}
	if (current=="paid"||current=="rejected"||current=="cancelled") && in.Status!=current { writeError(w,422,"CLAIM_CLOSED","A closed claim cannot be reopened; create a new claim if needed.");return }
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
		var portion, paid int64; var claimStatus string
		if err := tx.QueryRowContext(r.Context(), `SELECT payer_portion_minor,COALESCE((SELECT SUM(amount_minor) FROM insurance_claim_payments WHERE claim_id=?),0),status FROM insurance_claims WHERE id=?`, claimID, claimID).Scan(&portion, &paid, &claimStatus); err != nil {
			return err
		}
		if claimStatus!="approved" && claimStatus!="partially_paid" { return &APIError{Code:"CLAIM_NOT_APPROVED",Message:"Approve the claim before recording an insurer payment."} }
		if paid+in.AmountMinor > portion {
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
