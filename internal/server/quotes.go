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

func (s *Server) registerQuoteRoutes(r chi.Router) {
	r.Get("/quotes", s.handleQuotesList)
	r.Get("/quotes/{id}", s.handleQuoteGet)
	r.Post("/quotes", s.handleQuoteCreate)
	r.Patch("/quotes/{id}/status", s.handleQuoteStatus)
	r.Post("/quotes/{id}/convert", s.handleQuoteConvert)
}

type quotePayload struct {
	PatientID     string               `json:"patientId"`
	CustomerName  string               `json:"customerName"`
	Currency      string               `json:"currency"`
	ExchangeRate  string               `json:"exchangeRate"`
	DiscountMinor int64                `json:"discountMinor"`
	TaxMinor      int64                `json:"taxMinor"`
	ValidUntil    string               `json:"validUntil"`
	Notes         string               `json:"notes"`
	Status        string               `json:"status"`
	Items         []invoiceLinePayload `json:"items"`
}

// A quote is an offer, so it moves between these states without ever creating a
// debt. Only accepting it and converting it does that.
var quoteStatuses = map[string]bool{"draft": true, "sent": true, "accepted": true, "declined": true, "expired": true}

func (s *Server) handleQuoteCreate(w http.ResponseWriter, r *http.Request) {
	var input quotePayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	// A quote is priced exactly like an invoice, so it is validated by the same
	// rules: a quote that could not become an invoice is not worth sending.
	if validation := validateInvoice(invoicePayload{Items: input.Items, DiscountMinor: input.DiscountMinor, TaxMinor: input.TaxMinor}); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	if input.Status == "" {
		input.Status = "draft"
	}
	if !quoteStatuses[input.Status] || input.Status == "expired" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_QUOTE_STATUS", "A new quote may be a draft or sent.")
		return
	}
	if input.PatientID == "" && strings.TrimSpace(input.CustomerName) == "" {
		writeError(w, http.StatusUnprocessableEntity, "QUOTE_CUSTOMER_REQUIRED", "A quote needs a patient or a customer name.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	if input.ExchangeRate == "" {
		input.ExchangeRate = "1"
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(input.Currency) != 3 || !validExchangeRate(input.ExchangeRate) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CURRENCY", "Currency must be a three-letter code and exchange rate must be positive.")
		return
	}
	if input.ValidUntil != "" {
		if _, err := time.Parse("2006-01-02", input.ValidUntil); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_QUOTE_EXPIRY", "The validity date must be a valid date.")
			return
		}
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	var subtotal, total int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "quote", "QUO", true)
		if err != nil {
			return err
		}
		for _, item := range input.Items {
			subtotal += int64(item.Quantity) * item.UnitPriceMinor
			total += int64(item.Quantity)*item.UnitPriceMinor - item.DiscountMinor + item.TaxMinor
		}
		total = total - input.DiscountMinor + input.TaxMinor
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO quotes(id,quote_number,patient_id,customer_name,status,currency,exchange_rate,subtotal_minor,discount_minor,tax_minor,total_minor,valid_until,notes,created_at,updated_at,created_by,updated_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, number, nilIfEmpty(input.PatientID), nilIfEmpty(strings.TrimSpace(input.CustomerName)), input.Status, input.Currency, input.ExchangeRate,
			subtotal, input.DiscountMinor, input.TaxMinor, total, nilIfEmpty(input.ValidUntil), nilIfEmpty(input.Notes), now, now, user.ID, user.ID); err != nil {
			return err
		}
		for _, item := range input.Items {
			lineTotal := int64(item.Quantity)*item.UnitPriceMinor - item.DiscountMinor + item.TaxMinor
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO quote_items(id,quote_id,inventory_item_id,description,quantity,unit_price_minor,discount_minor,tax_minor,line_total_minor,procedure_code)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), id, nilIfEmpty(item.InventoryItemID), strings.TrimSpace(item.Description), item.Quantity, item.UnitPriceMinor, item.DiscountMinor, item.TaxMinor, lineTotal, nilIfEmpty(item.ProcedureCode)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUOTE_CREATE_FAILED", "Could not create the quote.")
		return
	}
	s.audit(r.Context(), &user, "create", "quote", id, "Created quote "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "quote.created", EntityType: "quote", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "quoteNumber": number, "status": input.Status, "totalMinor": total, "version": 1})
}

func (s *Server) handleQuotesList(w http.ResponseWriter, r *http.Request) {
	where, args := "q.archived_at IS NULL", []any{}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where += " AND q.status=?"
		args = append(args, status)
	}
	if patientID := strings.TrimSpace(r.URL.Query().Get("patientId")); patientID != "" {
		where += " AND q.patient_id=?"
		args = append(args, patientID)
	}
	paging := paginationFrom(r, 50, 200)
	total, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM quotes q WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUOTE_LIST_FAILED", "Could not load quotes.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT q.id,q.quote_number,COALESCE(q.patient_id,''),COALESCE(pt.first_name||' '||pt.last_name,COALESCE(q.customer_name,'')),q.status,q.currency,q.total_minor,COALESCE(q.valid_until,''),COALESCE(q.converted_invoice_id,''),COALESCE(inv.invoice_number,''),q.version,q.created_at,q.updated_at
		FROM quotes q LEFT JOIN patients pt ON pt.id=q.patient_id LEFT JOIN invoices inv ON inv.id=q.converted_invoice_id
		WHERE `+where+` ORDER BY q.created_at DESC LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUOTE_LIST_FAILED", "Could not load quotes.")
		return
	}
	defer rows.Close()
	today := time.Now().UTC().Format("2006-01-02")
	items := []map[string]any{}
	for rows.Next() {
		var id, number, patientID, customer, status, currency, validUntil, invoiceID, invoiceNumber, createdAt, updatedAt string
		var amount int64
		var version int
		if err := rows.Scan(&id, &number, &patientID, &customer, &status, &currency, &amount, &validUntil, &invoiceID, &invoiceNumber, &version, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "QUOTE_LIST_FAILED", "Could not load quotes.")
			return
		}
		// Expiry is derived rather than stored, so a quote does not need a
		// nightly job to stop looking current.
		expired := validUntil != "" && validUntil < today && status != "converted" && status != "accepted"
		items = append(items, map[string]any{"id": id, "quoteNumber": number, "patientId": patientID, "customerName": customer, "status": status,
			"currency": currency, "totalMinor": amount, "validUntil": validUntil, "expired": expired,
			"convertedInvoiceId": invoiceID, "convertedInvoiceNumber": invoiceNumber, "version": version, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(total)))
}

func (s *Server) handleQuoteGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var number, patientID, customer, status, currency, rate, validUntil, notes, invoiceID, createdAt, updatedAt string
	var subtotal, discount, tax, total int64
	var version int
	err := s.db.QueryRowContext(r.Context(), `SELECT q.quote_number,COALESCE(q.patient_id,''),COALESCE(pt.first_name||' '||pt.last_name,COALESCE(q.customer_name,'')),q.status,q.currency,q.exchange_rate,q.subtotal_minor,q.discount_minor,q.tax_minor,q.total_minor,COALESCE(q.valid_until,''),COALESCE(q.notes,''),COALESCE(q.converted_invoice_id,''),q.version,q.created_at,q.updated_at
		FROM quotes q LEFT JOIN patients pt ON pt.id=q.patient_id WHERE q.id=? AND q.archived_at IS NULL`, id).
		Scan(&number, &patientID, &customer, &status, &currency, &rate, &subtotal, &discount, &tax, &total, &validUntil, &notes, &invoiceID, &version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "QUOTE_NOT_FOUND", "Quote was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUOTE_LOAD_FAILED", "Could not load the quote.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT COALESCE(inventory_item_id,''),description,quantity,unit_price_minor,discount_minor,tax_minor,line_total_minor,COALESCE(procedure_code,'') FROM quote_items WHERE quote_id=?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUOTE_LOAD_FAILED", "Could not load the quote.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var itemID, description, procedureCode string
		var quantity int
		var unitPrice, lineDiscount, lineTax, lineTotal int64
		if err := rows.Scan(&itemID, &description, &quantity, &unitPrice, &lineDiscount, &lineTax, &lineTotal, &procedureCode); err != nil {
			writeError(w, http.StatusInternalServerError, "QUOTE_LOAD_FAILED", "Could not load the quote.")
			return
		}
		items = append(items, map[string]any{"inventoryItemId": itemID, "description": description, "quantity": quantity, "unitPriceMinor": unitPrice, "discountMinor": lineDiscount, "taxMinor": lineTax, "lineTotalMinor": lineTotal, "procedureCode": procedureCode})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "quoteNumber": number, "patientId": patientID, "customerName": customer, "status": status,
		"currency": currency, "exchangeRate": rate, "subtotalMinor": subtotal, "discountMinor": discount, "taxMinor": tax, "totalMinor": total,
		"validUntil": validUntil, "notes": notes, "convertedInvoiceId": invoiceID, "version": version, "createdAt": createdAt, "updatedAt": updatedAt, "items": items})
}

func (s *Server) handleQuoteStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A status and the current version are required.")
		return
	}
	if !quoteStatuses[input.Status] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_QUOTE_STATUS", "That is not a quote status.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	// A converted quote is finished: its invoice is the live record now.
	result, err := s.db.ExecContext(r.Context(), "UPDATE quotes SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status<>'converted' AND archived_at IS NULL",
		input.Status, now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUOTE_STATUS_FAILED", "Could not update the quote.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The quote changed since it was opened, or it has already been converted to an invoice.")
		return
	}
	s.audit(r.Context(), &user, "update", "quote", id, "Quote marked "+input.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "quote.updated", EntityType: "quote", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status, "version": input.Version + 1})
}

// handleQuoteConvert turns an accepted offer into a debt. Marking the quote and
// creating the invoice happen in one transaction, and the unique index on
// converted_invoice_id is what makes billing the same quote twice impossible
// even if two people press the button at once.
func (s *Server) handleQuoteConvert(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int    `json:"version"`
		DueAt   string `json:"dueAt"`
	}
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "The current quote version is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	var invoiceID, invoiceNumber string
	var total int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var patientID, customer, currency, rate, notes, status string
		var discount, tax int64
		if err := tx.QueryRowContext(r.Context(), "SELECT COALESCE(patient_id,''),COALESCE(customer_name,''),currency,exchange_rate,discount_minor,tax_minor,COALESCE(notes,''),status FROM quotes WHERE id=? AND version=? AND archived_at IS NULL",
			id, input.Version).Scan(&patientID, &customer, &currency, &rate, &discount, &tax, &notes, &status); err != nil {
			return err
		}
		if status == "converted" {
			return &APIError{Code: "QUOTE_ALREADY_CONVERTED", Message: "This quote has already become an invoice."}
		}
		if status == "declined" || status == "expired" {
			return &APIError{Code: "QUOTE_NOT_CONVERTIBLE", Message: "A declined or expired quote cannot be billed. Reissue it first."}
		}
		rows, err := tx.QueryContext(r.Context(), "SELECT COALESCE(inventory_item_id,''),description,quantity,unit_price_minor,discount_minor,tax_minor,COALESCE(procedure_code,'') FROM quote_items WHERE quote_id=?", id)
		if err != nil {
			return err
		}
		items := []invoiceLinePayload{}
		for rows.Next() {
			var line invoiceLinePayload
			if err := rows.Scan(&line.InventoryItemID, &line.Description, &line.Quantity, &line.UnitPriceMinor, &line.DiscountMinor, &line.TaxMinor, &line.ProcedureCode); err != nil {
				_ = rows.Close()
				return err
			}
			items = append(items, line)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) == 0 {
			return &APIError{Code: "QUOTE_HAS_NO_ITEMS", Message: "This quote has nothing to bill."}
		}
		// Converting does not move stock: the goods leave when they are handed
		// over and paid for at the till, not when the invoice is raised.
		invoiceID, invoiceNumber, total, err = s.createInvoiceTx(r, tx, invoicePayload{
			PatientID: patientID, Currency: currency, ExchangeRate: rate, DiscountMinor: discount, TaxMinor: tax,
			DueAt: input.DueAt, Notes: strings.TrimSpace("From quote. " + notes), Status: "issued", Items: items,
		}, user, false)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(r.Context(), "UPDATE quotes SET status='converted',converted_invoice_id=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND converted_invoice_id IS NULL",
			invoiceID, now, user.ID, id, input.Version)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return &APIError{Code: "QUOTE_ALREADY_CONVERTED", Message: "This quote has already become an invoice."}
		}
		return nil
	})
	if err == sql.ErrNoRows {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The quote changed since it was opened. Reload before converting it.")
		return
	}
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "QUOTE_CONVERT_FAILED", "Could not turn the quote into an invoice.")
		return
	}
	s.audit(r.Context(), &user, "convert", "quote", id, "Converted quote to invoice "+invoiceNumber, "", "", r)
	s.broker.Publish(realtime.Event{Type: "invoice.created", EntityType: "invoice", EntityID: invoiceID})
	writeJSON(w, http.StatusCreated, map[string]any{"quoteId": id, "invoiceId": invoiceID, "invoiceNumber": invoiceNumber, "totalMinor": total, "version": input.Version + 1})
}
