package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

func (s *Server) registerPOSRoutes(r chi.Router) {
	r.Get("/pos/service-types", s.handleServiceTypesList)
	r.Get("/pos/prescription-cart", s.handlePrescriptionCart)
	r.Get("/pos/parked", s.handleParkedSalesList)
	r.Post("/pos/parked", s.handleParkSale)
	r.Post("/pos/parked/{id}/resume", s.handleResumeParkedSale)
	r.Delete("/pos/parked/{id}", s.handleDiscardParkedSale)
	r.Get("/pos/sales", s.handleSalesHistory)
	r.Get("/cash-register/{id}/report", s.handleCashRegisterReport)
}

// handleServiceTypesList returns the clinic's sellable services — appointment
// and consultation types included. They are inventory items, so a price set once
// is the price the till charges and the price the schedule quotes.
func (s *Server) handleServiceTypesList(w http.ResponseWriter, r *http.Request) {
	where := "archived_at IS NULL AND category='service'"
	if r.URL.Query().Get("bookable") == "true" {
		where += " AND bookable=1"
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,sku,name,sale_price_minor,currency,duration_minutes,bookable,COALESCE(procedure_code,'') FROM inventory_items WHERE "+where+" ORDER BY name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVICE_TYPES_FAILED", "Could not load the clinic's services.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sku, name, currency, procedureCode string
		var price int64
		var duration int
		var bookable bool
		if err := rows.Scan(&id, &sku, &name, &price, &currency, &duration, &bookable, &procedureCode); err != nil {
			writeError(w, http.StatusInternalServerError, "SERVICE_TYPES_FAILED", "Could not load the clinic's services.")
			return
		}
		// A zero price is a service nobody has priced yet, not a free one. Saying
		// so here stops the till quietly charging nothing for an exam.
		items = append(items, map[string]any{"id": id, "sku": sku, "name": name, "salePriceMinor": price, "currency": currency, "durationMinutes": duration, "bookable": bookable, "procedureCode": procedureCode, "unpriced": price == 0})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handlePrescriptionCart turns a prescription into a cart the cashier can accept
// or edit. Each line the prescription implies is matched against the catalogue,
// so what is in stock, what it costs and what has to be ordered from the lab is
// answered before the conversation with the patient starts.
func (s *Server) handlePrescriptionCart(w http.ResponseWriter, r *http.Request) {
	prescriptionID := strings.TrimSpace(r.URL.Query().Get("prescriptionId"))
	patientID := strings.TrimSpace(r.URL.Query().Get("patientId"))
	if prescriptionID == "" && patientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A prescription or a patient is required.")
		return
	}
	var id, number, kind, details, notes, issuedAt, expiresAt, owner, patientName string
	query := `SELECT p.id,p.prescription_number,p.type,p.details_json,COALESCE(p.notes,''),p.issued_at,COALESCE(p.expires_at,''),p.patient_id,pt.first_name||' '||pt.last_name
		FROM prescriptions p JOIN patients pt ON pt.id=p.patient_id WHERE p.archived_at IS NULL AND p.status='final' AND `
	var err error
	if prescriptionID != "" {
		err = s.db.QueryRowContext(r.Context(), query+"p.id=?", prescriptionID).Scan(&id, &number, &kind, &details, &notes, &issuedAt, &expiresAt, &owner, &patientName)
	} else {
		err = s.db.QueryRowContext(r.Context(), query+"p.patient_id=? ORDER BY p.issued_at DESC LIMIT 1", patientID).Scan(&id, &number, &kind, &details, &notes, &issuedAt, &expiresAt, &owner, &patientName)
	}
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PRESCRIPTION_NOT_FOUND", "No prescription was found for that patient.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PRESCRIPTION_CART_FAILED", "Could not read the prescription.")
		return
	}
	suggestions, err := s.matchPrescriptionToCatalogue(r, kind, details)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PRESCRIPTION_CART_FAILED", "Could not price the prescription against the catalogue.")
		return
	}
	expired := expiresAt != "" && expiresAt < time.Now().UTC().Format("2006-01-02")
	writeJSON(w, http.StatusOK, map[string]any{
		"prescription": map[string]any{"id": id, "prescriptionNumber": number, "type": kind, "notes": notes, "issuedAt": issuedAt, "expiresAt": expiresAt, "expired": expired, "patientId": owner, "patientName": patientName},
		"lines":        suggestions,
	})
}

// matchPrescriptionToCatalogue proposes the catalogue items a prescription of
// this kind calls for. Availability and price come from inventory, never from
// the prescription, so the cart cannot promise stock the clinic does not have.
func (s *Server) matchPrescriptionToCatalogue(r *http.Request, kind, details string) ([]map[string]any, error) {
	var wanted []struct{ role, term string }
	switch kind {
	case "spectacle":
		wanted = []struct{ role, term string }{{"frame", ""}, {"lens", ""}}
	case "contact_lens":
		var parsed map[string]any
		_ = json.Unmarshal([]byte(details), &parsed)
		term, _ := parsed["brand"].(string)
		wanted = []struct{ role, term string }{{"contact_lens", term}}
	case "medication":
		var parsed map[string]any
		_ = json.Unmarshal([]byte(details), &parsed)
		term, _ := parsed["medication"].(string)
		wanted = []struct{ role, term string }{{"medication", term}}
	}
	categories := map[string]string{"frame": "frame", "lens": "ophthalmic_lens", "contact_lens": "contact_lens", "medication": "accessory"}
	lines := []map[string]any{}
	for _, want := range wanted {
		category := categories[want.role]
		where, args := "archived_at IS NULL AND category=?", []any{category}
		if strings.TrimSpace(want.term) != "" {
			where += " AND (name LIKE ? OR brand LIKE ?)"
			like := "%" + strings.TrimSpace(want.term) + "%"
			args = append(args, like, like)
		}
		// In-stock items first, then by price, so the cheapest thing the clinic
		// can actually hand over today is the default.
		rows, err := s.db.QueryContext(r.Context(), "SELECT id,sku,name,COALESCE(brand,''),sale_price_minor,currency,quantity,track_stock FROM inventory_items WHERE "+where+" ORDER BY CASE WHEN track_stock=0 OR quantity>0 THEN 0 ELSE 1 END, sale_price_minor LIMIT 8", args...)
		if err != nil {
			return nil, err
		}
		options := []map[string]any{}
		for rows.Next() {
			var itemID, sku, name, brand, currency string
			var price int64
			var quantity int
			var tracked bool
			if err := rows.Scan(&itemID, &sku, &name, &brand, &price, &currency, &quantity, &tracked); err != nil {
				_ = rows.Close()
				return nil, err
			}
			options = append(options, map[string]any{"id": itemID, "sku": sku, "name": name, "brand": brand, "salePriceMinor": price, "currency": currency, "quantity": quantity, "trackStock": tracked, "inStock": !tracked || quantity > 0})
		}
		_ = rows.Close()
		lines = append(lines, map[string]any{"role": want.role, "searchTerm": want.term, "options": options, "needsLabOrder": want.role == "lens" && len(options) == 0})
	}
	return lines, nil
}

type parkedSalePayload struct {
	Label      string           `json:"label"`
	PatientID  string           `json:"patientId"`
	Currency   string           `json:"currency"`
	Note       string           `json:"note"`
	TotalMinor int64            `json:"totalMinor"`
	Cart       []map[string]any `json:"cart"`
}

func (s *Server) handleParkSale(w http.ResponseWriter, r *http.Request) {
	var input parkedSalePayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" || len(input.Cart) == 0 || len(input.Cart) > 1000 || input.TotalMinor < 0 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PARKED_SALE", "A label and at least one cart line are required to park a sale.")
		return
	}
	if len([]rune(input.Label)) > 120 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PARKED_SALE", "The label is too long.")
		return
	}
	cart, err := json.Marshal(input.Cart)
	if err != nil || len(cart) > 256*1024 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PARKED_SALE", "The parked cart is invalid or too large.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(r.Context(), "INSERT INTO parked_sales(id,label,patient_id,currency,cart_json,note,total_minor,created_at,updated_at,created_by) VALUES(?,?,?,?,?,?,?,?,?,?)",
		id, input.Label, nilIfEmpty(input.PatientID), strings.ToUpper(input.Currency), string(cart), nilIfEmpty(input.Note), input.TotalMinor, now, now, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "PARK_SALE_FAILED", "Could not park the sale.")
		return
	}
	s.audit(r.Context(), &user, "park", "sale", id, "Parked sale "+input.Label, "", "", r)
	s.broker.Publish(realtime.Event{Type: "pos.parked", EntityType: "parked_sale", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "label": input.Label, "createdAt": now})
}

func (s *Server) handleParkedSalesList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.label,COALESCE(p.patient_id,''),COALESCE(pt.first_name||' '||pt.last_name,''),p.currency,p.total_minor,COALESCE(p.note,''),p.created_at,u.display_name
		FROM parked_sales p LEFT JOIN patients pt ON pt.id=p.patient_id JOIN users u ON u.id=p.created_by
		WHERE p.resumed_at IS NULL ORDER BY p.created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PARKED_SALES_FAILED", "Could not load parked sales.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, label, patientID, patientName, currency, note, createdAt, cashier string
		var total int64
		if err := rows.Scan(&id, &label, &patientID, &patientName, &currency, &total, &note, &createdAt, &cashier); err != nil {
			writeError(w, http.StatusInternalServerError, "PARKED_SALES_FAILED", "Could not load parked sales.")
			return
		}
		items = append(items, map[string]any{"id": id, "label": label, "patientId": patientID, "patientName": patientName, "currency": currency, "totalMinor": total, "note": note, "createdAt": createdAt, "parkedBy": cashier})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleResumeParkedSale hands the cart back and marks the hold used. Marking it
// inside the same statement that reads it means two cashiers cannot both resume
// the same parked sale and ring it up twice.
func (s *Server) handleResumeParkedSale(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	var label, patientID, currency, cart, note string
	var total int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(r.Context(), "SELECT label,COALESCE(patient_id,''),currency,cart_json,COALESCE(note,''),total_minor FROM parked_sales WHERE id=? AND resumed_at IS NULL", id).
			Scan(&label, &patientID, &currency, &cart, &note, &total); err != nil {
			return err
		}
		result, err := tx.ExecContext(r.Context(), "UPDATE parked_sales SET resumed_at=?,resumed_by=?,updated_at=? WHERE id=? AND resumed_at IS NULL", now, user.ID, now, id)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PARKED_SALE_NOT_FOUND", "That parked sale was already resumed or discarded.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RESUME_SALE_FAILED", "Could not resume the parked sale.")
		return
	}
	// The held cart stores what was chosen, not what it cost. Each line is priced
	// and stocked again from the catalogue now, so a sale set aside last week is
	// not resumed at last week's price or against stock that has since sold.
	lines, missing, err := s.repriceParkedCart(r, cart)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RESUME_SALE_FAILED", "Could not price the parked sale against today's catalogue.")
		return
	}
	s.audit(r.Context(), &user, "resume", "sale", id, "Resumed parked sale "+label, "", "", r)
	s.broker.Publish(realtime.Event{Type: "pos.parked", EntityType: "parked_sale", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "label": label, "patientId": patientID, "currency": currency, "note": note, "totalMinor": total, "cart": lines, "unavailable": missing})
}

type parkedCartLine struct {
	InventoryItemID string `json:"inventoryItemId"`
	Description     string `json:"description"`
	Quantity        int    `json:"quantity"`
	DiscountMinor   int64  `json:"discountMinor"`
}

// repriceParkedCart resolves each held line against the catalogue as it stands
// now, and reports the lines that no longer exist rather than dropping them
// silently.
func (s *Server) repriceParkedCart(r *http.Request, cart string) ([]map[string]any, []map[string]any, error) {
	var held []parkedCartLine
	if err := json.Unmarshal([]byte(cart), &held); err != nil {
		return nil, nil, err
	}
	lines, missing := []map[string]any{}, []map[string]any{}
	for _, line := range held {
		var sku, barcode, category, name, brand, currency, procedureCode string
		var price int64
		var quantity int
		var tracked bool
		err := s.db.QueryRowContext(r.Context(), "SELECT sku,COALESCE(barcode,''),category,name,COALESCE(brand,''),sale_price_minor,currency,quantity,track_stock,COALESCE(procedure_code,'') FROM inventory_items WHERE id=? AND archived_at IS NULL", line.InventoryItemID).
			Scan(&sku, &barcode, &category, &name, &brand, &price, &currency, &quantity, &tracked, &procedureCode)
		if err == sql.ErrNoRows {
			missing = append(missing, map[string]any{"inventoryItemId": line.InventoryItemID, "description": line.Description})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		lines = append(lines, map[string]any{
			"inventoryItemId": line.InventoryItemID, "sku": sku, "barcode": barcode, "category": category, "name": name, "brand": brand,
			"salePriceMinor": price, "currency": currency, "quantity": quantity, "trackStock": tracked, "procedureCode": procedureCode,
			"cartQuantity": line.Quantity, "discountMinor": line.DiscountMinor,
			"inStock": !tracked || quantity >= line.Quantity,
		})
	}
	return lines, missing, nil
}

func (s *Server) handleDiscardParkedSale(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), "UPDATE parked_sales SET resumed_at=?,resumed_by=?,updated_at=? WHERE id=? AND resumed_at IS NULL", now, user.ID, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DISCARD_SALE_FAILED", "Could not discard the parked sale.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "PARKED_SALE_NOT_FOUND", "That parked sale was already resumed or discarded.")
		return
	}
	s.audit(r.Context(), &user, "discard", "sale", id, "Discarded a parked sale", "", "", r)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "discarded": true})
}

// handleSalesHistory is the till's own view of what was sold: every invoice with
// what has been paid against it, who took the money and how, filterable the way
// a cashier or a manager actually asks the question.
func (s *Server) handleSalesHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	where, args := "i.archived_at IS NULL", []any{}
	if from := strings.TrimSpace(query.Get("from")); from != "" {
		where += " AND i.created_at>=?"
		args = append(args, from)
	}
	if to := strings.TrimSpace(query.Get("to")); to != "" {
		where += " AND i.created_at<=?"
		args = append(args, to+"T23:59:59.999Z")
	}
	if cashier := strings.TrimSpace(query.Get("cashierId")); cashier != "" {
		where += " AND i.created_by=?"
		args = append(args, cashier)
	}
	if patient := strings.TrimSpace(query.Get("patientId")); patient != "" {
		where += " AND i.patient_id=?"
		args = append(args, patient)
	}
	if method := strings.TrimSpace(query.Get("paymentMethodId")); method != "" {
		where += " AND EXISTS(SELECT 1 FROM payments pm WHERE pm.invoice_id=i.id AND pm.payment_method_id=?)"
		args = append(args, method)
	}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		where += " AND i.status=?"
		args = append(args, status)
	}
	paging := paginationFrom(r, 50, 200)
	total, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM invoices i WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SALES_HISTORY_FAILED", "Could not load the sales history.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT i.id,i.invoice_number,COALESCE(i.patient_id,''),COALESCE(pt.first_name||' '||pt.last_name,'Retail customer'),i.status,i.currency,i.total_minor,
		COALESCE((SELECT SUM(amount_minor) FROM payments WHERE invoice_id=i.id),0),
		COALESCE((SELECT SUM(r.amount_minor) FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE p.invoice_id=i.id),0),
		u.display_name,COALESCE((SELECT group_concat(DISTINCT m.name) FROM payments p JOIN payment_methods m ON m.id=p.payment_method_id WHERE p.invoice_id=i.id),''),
		COALESCE(i.appointment_id,''),COALESCE(i.encounter_id,''),i.created_at
		FROM invoices i LEFT JOIN patients pt ON pt.id=i.patient_id JOIN users u ON u.id=i.created_by
		WHERE `+where+` ORDER BY i.created_at DESC LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SALES_HISTORY_FAILED", "Could not load the sales history.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, patientID, patientName, status, currency, cashier, methods, appointmentID, encounterID, createdAt string
		var amount, paid, refunded int64
		if err := rows.Scan(&id, &number, &patientID, &patientName, &status, &currency, &amount, &paid, &refunded, &cashier, &methods, &appointmentID, &encounterID, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "SALES_HISTORY_FAILED", "Could not load the sales history.")
			return
		}
		items = append(items, map[string]any{"id": id, "invoiceNumber": number, "patientId": patientID, "patientName": patientName, "status": status, "currency": currency,
			"totalMinor": amount, "paidMinor": paid - refunded, "refundedMinor": refunded, "balanceMinor": amount - (paid - refunded),
			"cashier": cashier, "paymentMethods": splitMethods(methods), "appointmentId": appointmentID, "encounterId": encounterID, "createdAt": createdAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(total)))
}

func splitMethods(joined string) []string {
	names := []string{}
	for _, name := range strings.Split(joined, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

// handleCashRegisterReport is the end-of-day Z-report: what the till took,
// broken down by payment method, against the cash it should be holding. It reads
// an open session as well as a closed one, so a cashier can reconcile before
// committing to a close.
func (s *Server) handleCashRegisterReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var currency, openedAt, openedBy, closedAt, closedBy string
	var opening int64
	var counted, expected, difference sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `SELECT c.currency,c.opening_float_minor,c.opened_at,u.display_name,COALESCE(c.closed_at,''),COALESCE(closer.display_name,''),c.counted_cash_minor,c.expected_cash_minor,c.difference_minor
		FROM cash_register_sessions c JOIN users u ON u.id=c.opened_by LEFT JOIN users closer ON closer.id=c.closed_by WHERE c.id=?`, id).
		Scan(&currency, &opening, &openedAt, &openedBy, &closedAt, &closedBy, &counted, &expected, &difference)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "REGISTER_NOT_FOUND", "That cash register session was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTER_REPORT_FAILED", "Could not build the register report.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT m.name,COUNT(p.id),COALESCE(SUM(p.amount_minor),0),
		COALESCE((SELECT SUM(r.amount_minor) FROM refunds r JOIN payments rp ON rp.id=r.payment_id WHERE rp.register_session_id=p.register_session_id AND rp.payment_method_id=m.id),0)
		FROM payments p JOIN payment_methods m ON m.id=p.payment_method_id WHERE p.register_session_id=? GROUP BY m.id ORDER BY m.name`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTER_REPORT_FAILED", "Could not build the register report.")
		return
	}
	defer rows.Close()
	byMethod := []map[string]any{}
	var takings, refunds int64
	for rows.Next() {
		var name string
		var count int
		var amount, refunded int64
		if err := rows.Scan(&name, &count, &amount, &refunded); err != nil {
			writeError(w, http.StatusInternalServerError, "REGISTER_REPORT_FAILED", "Could not build the register report.")
			return
		}
		byMethod = append(byMethod, map[string]any{"method": name, "payments": count, "amountMinor": amount, "refundedMinor": refunded, "netMinor": amount - refunded})
		takings += amount
		refunds += refunded
	}
	var sales int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(DISTINCT invoice_id) FROM payments WHERE register_session_id=?", id).Scan(&sales)
	report := map[string]any{
		"id": id, "currency": currency, "openingFloatMinor": opening, "openedAt": openedAt, "openedBy": openedBy,
		"closedAt": closedAt, "closedBy": closedBy, "open": closedAt == "",
		"salesCount": sales, "takingsMinor": takings, "refundsMinor": refunds, "netMinor": takings - refunds,
		"byMethod": byMethod, "generatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if counted.Valid {
		report["countedCashMinor"] = counted.Int64
		report["expectedCashMinor"] = expected.Int64
		report["differenceMinor"] = difference.Int64
	} else {
		// Still open: show what the drawer should hold if it were counted now.
		var cashPayments, cashRefunds int64
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(p.amount_minor),0) FROM payments p JOIN payment_methods pm ON pm.id=p.payment_method_id WHERE p.register_session_id=? AND lower(pm.name)='cash'`, id).Scan(&cashPayments)
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(r.amount_minor),0) FROM refunds r JOIN payments p ON p.id=r.payment_id JOIN payment_methods pm ON pm.id=p.payment_method_id WHERE p.register_session_id=? AND lower(pm.name)='cash'`, id).Scan(&cashRefunds)
		report["expectedCashMinor"] = opening + cashPayments - cashRefunds
	}
	writeJSON(w, http.StatusOK, report)
}
