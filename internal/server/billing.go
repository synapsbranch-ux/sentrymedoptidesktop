package server

import (
	"database/sql"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

type invoiceLinePayload struct {
	InventoryItemID string `json:"inventoryItemId"`
	Description     string `json:"description"`
	Quantity        int    `json:"quantity"`
	UnitPriceMinor  int64  `json:"unitPriceMinor"`
	DiscountMinor   int64  `json:"discountMinor"`
	TaxMinor        int64  `json:"taxMinor"`
}

type invoicePayload struct {
	PatientID     string               `json:"patientId"`
	Currency      string               `json:"currency"`
	ExchangeRate  string               `json:"exchangeRate"`
	DiscountMinor int64                `json:"discountMinor"`
	TaxMinor      int64                `json:"taxMinor"`
	DueAt         string               `json:"dueAt"`
	Notes         string               `json:"notes"`
	Status        string               `json:"status"`
	Items         []invoiceLinePayload `json:"items"`
}

type paymentPayload struct {
	PaymentMethodID   string `json:"paymentMethodId"`
	RegisterSessionID string `json:"registerSessionId"`
	AmountMinor       int64  `json:"amountMinor"`
	Currency          string `json:"currency"`
	ExchangeRate      string `json:"exchangeRate"`
	Reference         string `json:"reference"`
	Notes             string `json:"notes"`
}

func (s *Server) handleInvoicesList(w http.ResponseWriter, r *http.Request) {
	status, patientID := r.URL.Query().Get("status"), r.URL.Query().Get("patientId")
	where, args := "i.archived_at IS NULL", []any{}
	if status != "" {
		where += " AND i.status=?"
		args = append(args, status)
	}
	if patientID != "" {
		where += " AND i.patient_id=?"
		args = append(args, patientID)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT i.id,i.invoice_number,COALESCE(i.patient_id,''),COALESCE(p.first_name||' '||p.last_name,'Retail customer'),i.status,i.currency,i.exchange_rate,i.subtotal_minor,i.discount_minor,i.tax_minor,i.total_minor,COALESCE(pay.paid,0)-COALESCE(ref.refunded,0),i.total_minor-(COALESCE(pay.paid,0)-COALESCE(ref.refunded,0)),COALESCE(i.due_at,''),i.version,i.created_at,i.updated_at
		FROM invoices i LEFT JOIN patients p ON p.id=i.patient_id
		LEFT JOIN (SELECT invoice_id,SUM(amount_minor) paid FROM payments GROUP BY invoice_id) pay ON pay.invoice_id=i.id
		LEFT JOIN (SELECT p.invoice_id,SUM(r.amount_minor) refunded FROM refunds r JOIN payments p ON p.id=r.payment_id GROUP BY p.invoice_id) ref ON ref.invoice_id=i.id
		WHERE `+where+` ORDER BY i.created_at DESC LIMIT 1000`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INVOICE_LIST_FAILED", "Could not load invoices.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, patientID, patientName, status, currency, exchangeRate, dueAt, createdAt, updatedAt string
		var subtotal, discount, tax, total, paid, balance int64
		var version int
		if rows.Scan(&id, &number, &patientID, &patientName, &status, &currency, &exchangeRate, &subtotal, &discount, &tax, &total, &paid, &balance, &dueAt, &version, &createdAt, &updatedAt) == nil {
			items = append(items, map[string]any{"id": id, "invoiceNumber": number, "patientId": patientID, "patientName": patientName, "status": status, "currency": currency, "exchangeRate": exchangeRate, "subtotalMinor": subtotal, "discountMinor": discount, "taxMinor": tax, "totalMinor": total, "paidMinor": paid, "balanceMinor": balance, "dueAt": dueAt, "version": version, "createdAt": createdAt, "updatedAt": updatedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleInvoiceGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var number, patientID, patientName, status, currency, exchangeRate, dueAt, notes, createdAt, updatedAt string
	var subtotal, discount, tax, total, paid, balance int64
	var version int
	err := s.db.QueryRowContext(r.Context(), `SELECT i.invoice_number,COALESCE(i.patient_id,''),COALESCE(p.first_name||' '||p.last_name,'Retail customer'),i.status,i.currency,i.exchange_rate,i.subtotal_minor,i.discount_minor,i.tax_minor,i.total_minor,COALESCE(pay.paid,0)-COALESCE(ref.refunded,0),i.total_minor-(COALESCE(pay.paid,0)-COALESCE(ref.refunded,0)),COALESCE(i.due_at,''),COALESCE(i.notes,''),i.version,i.created_at,i.updated_at FROM invoices i LEFT JOIN patients p ON p.id=i.patient_id LEFT JOIN (SELECT invoice_id,SUM(amount_minor) paid FROM payments GROUP BY invoice_id) pay ON pay.invoice_id=i.id LEFT JOIN (SELECT p.invoice_id,SUM(r.amount_minor) refunded FROM refunds r JOIN payments p ON p.id=r.payment_id GROUP BY p.invoice_id) ref ON ref.invoice_id=i.id WHERE i.id=? AND i.archived_at IS NULL`, id).
		Scan(&number, &patientID, &patientName, &status, &currency, &exchangeRate, &subtotal, &discount, &tax, &total, &paid, &balance, &dueAt, &notes, &version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "INVOICE_NOT_FOUND", "Invoice was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INVOICE_LOAD_FAILED", "Could not load invoice.")
		return
	}
	items := []map[string]any{}
	rows, _ := s.db.QueryContext(r.Context(), "SELECT id,COALESCE(inventory_item_id,''),description,quantity,unit_price_minor,discount_minor,tax_minor,line_total_minor,cost_minor FROM invoice_items WHERE invoice_id=?", id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var itemID, inventoryID, description string
			var quantity int
			var price, itemDiscount, itemTax, lineTotal, cost int64
			if rows.Scan(&itemID, &inventoryID, &description, &quantity, &price, &itemDiscount, &itemTax, &lineTotal, &cost) == nil {
				items = append(items, map[string]any{"id": itemID, "inventoryItemId": inventoryID, "description": description, "quantity": quantity, "unitPriceMinor": price, "discountMinor": itemDiscount, "taxMinor": itemTax, "lineTotalMinor": lineTotal, "costMinor": cost})
			}
		}
	}
	payments := []map[string]any{}
	payRows, _ := s.db.QueryContext(r.Context(), `SELECT p.id,p.receipt_number,p.amount_minor,p.currency,p.exchange_rate,pm.name,COALESCE(p.reference,''),p.received_at,u.display_name,COALESCE((SELECT SUM(amount_minor) FROM refunds WHERE payment_id=p.id),0) FROM payments p JOIN payment_methods pm ON pm.id=p.payment_method_id JOIN users u ON u.id=p.created_by WHERE p.invoice_id=? ORDER BY p.received_at`, id)
	if payRows != nil {
		defer payRows.Close()
		for payRows.Next() {
			var paymentID, receipt, payCurrency, rate, method, reference, receivedAt, user string
			var amount, refunded int64
			if payRows.Scan(&paymentID, &receipt, &amount, &payCurrency, &rate, &method, &reference, &receivedAt, &user, &refunded) == nil {
				payments = append(payments, map[string]any{"id": paymentID, "receiptNumber": receipt, "amountMinor": amount, "refundedMinor": refunded, "currency": payCurrency, "exchangeRate": rate, "paymentMethod": method, "reference": reference, "receivedAt": receivedAt, "receivedBy": user})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "invoiceNumber": number, "patientId": patientID, "patientName": patientName, "status": status, "currency": currency, "exchangeRate": exchangeRate, "subtotalMinor": subtotal, "discountMinor": discount, "taxMinor": tax, "totalMinor": total, "paidMinor": paid, "balanceMinor": balance, "dueAt": dueAt, "notes": notes, "version": version, "createdAt": createdAt, "updatedAt": updatedAt, "items": items, "payments": payments})
}

func validateInvoice(input invoicePayload) *APIError {
	if len(input.Items) == 0 {
		return &APIError{Code: "INVOICE_ITEMS_REQUIRED", Message: "Add at least one invoice item."}
	}
	if input.DiscountMinor < 0 || input.TaxMinor < 0 {
		return &APIError{Code: "INVALID_INVOICE_TOTAL", Message: "Discount and tax cannot be negative."}
	}
	for _, item := range input.Items {
		if strings.TrimSpace(item.Description) == "" || item.Quantity <= 0 || item.UnitPriceMinor < 0 || item.DiscountMinor < 0 || item.TaxMinor < 0 {
			return &APIError{Code: "INVALID_INVOICE_ITEM", Message: "Every invoice item needs a description, positive quantity, and valid amounts."}
		}
	}
	return nil
}

func (s *Server) createInvoiceTx(ctx *http.Request, tx *sql.Tx, input invoicePayload, user AuthUser, deductStock bool) (id, number string, total int64, err error) {
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	if input.ExchangeRate == "" {
		input.ExchangeRate = "1"
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(input.Currency) != 3 || !validExchangeRate(input.ExchangeRate) {
		return "", "", 0, &APIError{Code: "INVALID_CURRENCY", Message: "Currency must be a three-letter code and exchange rate must be positive."}
	}
	status := input.Status
	if status == "" {
		status = "issued"
	}
	if status != "draft" && status != "issued" {
		return "", "", 0, &APIError{Code: "INVALID_INVOICE_STATUS", Message: "New invoices may be draft or issued."}
	}
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	number, err = s.nextNumber(ctx.Context(), tx, "invoice", "INV", true)
	if err != nil {
		return "", "", 0, err
	}
	var subtotal int64
	for _, item := range input.Items {
		line := int64(item.Quantity)*item.UnitPriceMinor - item.DiscountMinor + item.TaxMinor
		if line < 0 {
			return "", "", 0, &APIError{Code: "INVALID_LINE_TOTAL", Message: "An invoice line total cannot be negative."}
		}
		subtotal += int64(item.Quantity) * item.UnitPriceMinor
		total += line
	}
	total = total - input.DiscountMinor + input.TaxMinor
	if total < 0 {
		return "", "", 0, &APIError{Code: "INVALID_INVOICE_TOTAL", Message: "Invoice total cannot be negative."}
	}
	if _, err = tx.ExecContext(ctx.Context(), `INSERT INTO invoices(id,invoice_number,patient_id,status,currency,exchange_rate,subtotal_minor,discount_minor,tax_minor,total_minor,due_at,notes,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, number, nilIfEmpty(input.PatientID), status, input.Currency, input.ExchangeRate, subtotal, input.DiscountMinor, input.TaxMinor, total, nilIfEmpty(input.DueAt), nilIfEmpty(input.Notes), now, now, user.ID, user.ID); err != nil {
		return "", "", 0, err
	}
	for _, item := range input.Items {
		var cost int64
		if item.InventoryItemID != "" {
			var trackStock bool
			var quantity int
			var itemCurrency string
			if err = tx.QueryRowContext(ctx.Context(), "SELECT cost_minor,track_stock,quantity,currency FROM inventory_items WHERE id=? AND archived_at IS NULL", item.InventoryItemID).Scan(&cost, &trackStock, &quantity, &itemCurrency); err != nil {
				return "", "", 0, err
			}
			if itemCurrency != input.Currency {
				return "", "", 0, &APIError{Code: "MIXED_INVOICE_CURRENCY", Message: "Inventory items on an invoice must use the invoice currency."}
			}
			if deductStock && trackStock {
				resulting := quantity - item.Quantity
				if resulting < 0 {
					return "", "", 0, &APIError{Code: "INSUFFICIENT_STOCK", Message: "Insufficient stock for " + item.Description + "."}
				}
				if _, err = tx.ExecContext(ctx.Context(), "UPDATE inventory_items SET quantity=?,version=version+1,updated_at=?,updated_by=? WHERE id=?", resulting, now, user.ID, item.InventoryItemID); err != nil {
					return "", "", 0, err
				}
				if _, err = tx.ExecContext(ctx.Context(), `INSERT INTO stock_movements(id,item_id,movement_type,previous_quantity,quantity_change,resulting_quantity,reason,reference_type,reference_id,created_at,created_by) VALUES(?,?,'sale',?,?,?,'POS sale','invoice',?,?,?)`, uuid.NewString(), item.InventoryItemID, quantity, -item.Quantity, resulting, id, now, user.ID); err != nil {
					return "", "", 0, err
				}
			}
		}
		lineTotal := int64(item.Quantity)*item.UnitPriceMinor - item.DiscountMinor + item.TaxMinor
		if _, err = tx.ExecContext(ctx.Context(), `INSERT INTO invoice_items(id,invoice_id,inventory_item_id,description,quantity,unit_price_minor,discount_minor,tax_minor,line_total_minor,cost_minor) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), id, nilIfEmpty(item.InventoryItemID), strings.TrimSpace(item.Description), item.Quantity, item.UnitPriceMinor, item.DiscountMinor, item.TaxMinor, lineTotal, cost); err != nil {
			return "", "", 0, err
		}
	}
	return id, number, total, nil
}

func (s *Server) handleInvoiceCreate(w http.ResponseWriter, r *http.Request) {
	var input invoicePayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if validation := validateInvoice(input); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	user, _ := userFromContext(r.Context())
	var id, number string
	var total int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		id, number, total, err = s.createInvoiceTx(r, tx, input, user, false)
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "INVOICE_CREATE_FAILED", "Could not create the invoice.")
		return
	}
	s.audit(r.Context(), &user, "create", "invoice", id, "Created invoice "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "invoice.created", EntityType: "invoice", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "invoiceNumber": number, "totalMinor": total, "status": "issued", "version": 1})
}

func (s *Server) recordPaymentTx(r *http.Request, tx *sql.Tx, invoiceID string, input paymentPayload, user AuthUser) (id, receipt string, balance int64, err error) {
	if input.AmountMinor <= 0 || input.PaymentMethodID == "" {
		return "", "", 0, &APIError{Code: "INVALID_PAYMENT", Message: "Payment method and a positive amount are required."}
	}
	var total int64
	var status, invoiceCurrency string
	if err = tx.QueryRowContext(r.Context(), "SELECT total_minor,status,currency FROM invoices WHERE id=? AND archived_at IS NULL", invoiceID).Scan(&total, &status, &invoiceCurrency); err != nil {
		return "", "", 0, err
	}
	if status == "cancelled" || status == "refunded" {
		return "", "", 0, &APIError{Code: "INVOICE_NOT_PAYABLE", Message: "This invoice cannot receive payments."}
	}
	var paid, refunded int64
	_ = tx.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(amount_minor),0) FROM payments WHERE invoice_id=?", invoiceID).Scan(&paid)
	_ = tx.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(r.amount_minor),0) FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE p.invoice_id=?", invoiceID).Scan(&refunded)
	balance = total - (paid - refunded)
	if input.AmountMinor > balance {
		return "", "", balance, &APIError{Code: "PAYMENT_EXCEEDS_BALANCE", Message: "Payment exceeds the outstanding invoice balance."}
	}
	if input.Currency == "" {
		input.Currency = invoiceCurrency
	}
	if input.ExchangeRate == "" {
		input.ExchangeRate = "1"
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.Currency != invoiceCurrency {
		return "", "", balance, &APIError{Code: "PAYMENT_CURRENCY_MISMATCH", Message: "Payment currency must match the invoice currency."}
	}
	if !validExchangeRate(input.ExchangeRate) {
		return "", "", balance, &APIError{Code: "INVALID_EXCHANGE_RATE", Message: "Exchange rate must be a positive decimal value."}
	}
	id = uuid.NewString()
	receipt, err = s.nextNumber(r.Context(), tx, "receipt", "RCT", true)
	if err != nil {
		return "", "", 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO payments(id,receipt_number,invoice_id,register_session_id,payment_method_id,amount_minor,currency,exchange_rate,reference,notes,received_at,created_at,created_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, receipt, invoiceID, nilIfEmpty(input.RegisterSessionID), input.PaymentMethodID, input.AmountMinor, input.Currency, input.ExchangeRate, nilIfEmpty(input.Reference), nilIfEmpty(input.Notes), now, now, user.ID); err != nil {
		return "", "", 0, err
	}
	balance -= input.AmountMinor
	newStatus := "partially_paid"
	if balance == 0 {
		newStatus = "paid"
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE invoices SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=?", newStatus, now, user.ID, invoiceID)
	return id, receipt, balance, err
}

func (s *Server) handlePaymentCreate(w http.ResponseWriter, r *http.Request) {
	var input paymentPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	user, _ := userFromContext(r.Context())
	invoiceID := chi.URLParam(r, "id")
	var id, receipt string
	var balance int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		id, receipt, balance, err = s.recordPaymentTx(r, tx, invoiceID, input, user)
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "INVOICE_NOT_FOUND", "Invoice was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "PAYMENT_CREATE_FAILED", "Could not record payment.")
		return
	}
	s.audit(r.Context(), &user, "payment", "invoice", invoiceID, "Recorded payment "+receipt, "", "", r)
	s.broker.Publish(realtime.Event{Type: "invoice.paid", EntityType: "invoice", EntityID: invoiceID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "receiptNumber": receipt, "balanceMinor": balance})
}

type posPayload struct {
	Invoice invoicePayload  `json:"invoice"`
	Payment *paymentPayload `json:"payment"`
}

func (s *Server) handlePOSCheckout(w http.ResponseWriter, r *http.Request) {
	var input posPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if validation := validateInvoice(input.Invoice); validation != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	input.Invoice.Status = "issued"
	user, _ := userFromContext(r.Context())
	var invoiceID, invoiceNumber, paymentID, receipt string
	var total, balance int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		invoiceID, invoiceNumber, total, err = s.createInvoiceTx(r, tx, input.Invoice, user, true)
		if err != nil {
			return err
		}
		balance = total
		if input.Payment != nil && input.Payment.AmountMinor > 0 {
			paymentID, receipt, balance, err = s.recordPaymentTx(r, tx, invoiceID, *input.Payment, user)
		}
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "POS_CHECKOUT_FAILED", "Checkout was rolled back. No stock or financial records were changed.")
		return
	}
	s.audit(r.Context(), &user, "checkout", "invoice", invoiceID, "Completed POS checkout "+invoiceNumber, "", "", r)
	s.broker.Publish(realtime.Event{Type: "pos.completed", EntityType: "invoice", EntityID: invoiceID})
	s.broker.Publish(realtime.Event{Type: "inventory.changed", EntityType: "invoice", EntityID: invoiceID})
	writeJSON(w, http.StatusCreated, map[string]any{"invoiceId": invoiceID, "invoiceNumber": invoiceNumber, "totalMinor": total, "balanceMinor": balance, "paymentId": paymentID, "receiptNumber": receipt})
}

func (s *Server) handleRefundCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AmountMinor int64  `json:"amountMinor"`
		Reason      string `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil || input.AmountMinor <= 0 || strings.TrimSpace(input.Reason) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Positive amount and refund reason are required.")
		return
	}
	user, _ := userFromContext(r.Context())
	paymentID, refundID, now := chi.URLParam(r, "id"), uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var invoiceID string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var paid, refunded int64
		if err := tx.QueryRowContext(r.Context(), "SELECT invoice_id,amount_minor FROM payments WHERE id=?", paymentID).Scan(&invoiceID, &paid); err != nil {
			return err
		}
		_ = tx.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(amount_minor),0) FROM refunds WHERE payment_id=?", paymentID).Scan(&refunded)
		if input.AmountMinor > paid-refunded {
			return &APIError{Code: "REFUND_EXCEEDS_PAYMENT", Message: "Refund exceeds the refundable payment amount."}
		}
		if _, err := tx.ExecContext(r.Context(), "INSERT INTO refunds(id,payment_id,amount_minor,reason,refunded_at,created_by) VALUES(?,?,?,?,?,?)", refundID, paymentID, input.AmountMinor, strings.TrimSpace(input.Reason), now, user.ID); err != nil {
			return err
		}
		var total, netPaid int64
		_ = tx.QueryRowContext(r.Context(), "SELECT total_minor FROM invoices WHERE id=?", invoiceID).Scan(&total)
		_ = tx.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(p.amount_minor),0)-COALESCE((SELECT SUM(r.amount_minor) FROM refunds r JOIN payments p2 ON p2.id=r.payment_id WHERE p2.invoice_id=?),0) FROM payments p WHERE p.invoice_id=?`, invoiceID, invoiceID).Scan(&netPaid)
		status := "partially_paid"
		if netPaid == 0 {
			status = "refunded"
		}
		_, err := tx.ExecContext(r.Context(), "UPDATE invoices SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=?", status, now, user.ID, invoiceID)
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "REFUND_FAILED", "Could not record refund.")
		return
	}
	s.audit(r.Context(), &user, "refund", "payment", paymentID, "Recorded payment refund", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": refundID, "invoiceId": invoiceID})
}

func validExchangeRate(value string) bool {
	rate, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	return ok && rate.Sign() > 0
}

func (s *Server) handlePaymentMethodsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,name FROM payment_methods WHERE active=1 ORDER BY sort_order,name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYMENT_METHODS_FAILED", "Could not load payment methods.")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, name string
		if rows.Scan(&id, &name) == nil {
			items = append(items, map[string]string{"id": id, "name": name})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type registerOpenPayload struct {
	Currency          string `json:"currency"`
	OpeningFloatMinor int64  `json:"openingFloatMinor"`
	Notes             string `json:"notes"`
}

func (s *Server) handleCashRegisterOpen(w http.ResponseWriter, r *http.Request) {
	var input registerOpenPayload
	if err := decodeJSON(r, &input); err != nil || input.OpeningFloatMinor < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Currency and a non-negative opening float are required.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	var open int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM cash_register_sessions WHERE currency=? AND closed_at IS NULL", strings.ToUpper(input.Currency)).Scan(&open)
	if open > 0 {
		writeError(w, http.StatusConflict, "REGISTER_ALREADY_OPEN", "A cash register is already open for this currency.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), "INSERT INTO cash_register_sessions(id,opened_by,currency,opening_float_minor,opening_notes,opened_at) VALUES(?,?,?,?,?,?)", id, user.ID, strings.ToUpper(input.Currency), input.OpeningFloatMinor, nilIfEmpty(input.Notes), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTER_OPEN_FAILED", "Could not open cash register.")
		return
	}
	s.audit(r.Context(), &user, "open", "cash_register", id, "Opened cash register", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "openedAt": now})
}

func (s *Server) handleCashRegisterCurrent(w http.ResponseWriter, r *http.Request) {
	var id, currency, openedAt, openedBy, notes string
	var opening int64
	err := s.db.QueryRowContext(r.Context(), `SELECT c.id,c.currency,c.opening_float_minor,c.opened_at,u.display_name,COALESCE(c.opening_notes,'') FROM cash_register_sessions c JOIN users u ON u.id=c.opened_by WHERE c.closed_at IS NULL ORDER BY c.opened_at DESC LIMIT 1`).Scan(&id, &currency, &opening, &openedAt, &openedBy, &notes)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"open": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTER_LOAD_FAILED", "Could not load cash register.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"open": true, "id": id, "currency": currency, "openingFloatMinor": opening, "openedAt": openedAt, "openedBy": openedBy, "notes": notes})
}

type registerClosePayload struct {
	CountedCashMinor int64  `json:"countedCashMinor"`
	Notes            string `json:"notes"`
}

func (s *Server) handleCashRegisterClose(w http.ResponseWriter, r *http.Request) {
	var input registerClosePayload
	if err := decodeJSON(r, &input); err != nil || input.CountedCashMinor < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A non-negative counted cash amount is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	var expected, difference int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var opening int64
		if err := tx.QueryRowContext(r.Context(), "SELECT opening_float_minor FROM cash_register_sessions WHERE id=? AND closed_at IS NULL", id).Scan(&opening); err != nil {
			return err
		}
		var cashPayments, cashRefunds int64
		_ = tx.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(p.amount_minor),0) FROM payments p JOIN payment_methods pm ON pm.id=p.payment_method_id WHERE p.register_session_id=? AND lower(pm.name)='cash'`, id).Scan(&cashPayments)
		_ = tx.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(r.amount_minor),0) FROM refunds r JOIN payments p ON p.id=r.payment_id JOIN payment_methods pm ON pm.id=p.payment_method_id WHERE p.register_session_id=? AND lower(pm.name)='cash'`, id).Scan(&cashRefunds)
		expected = opening + cashPayments - cashRefunds
		difference = input.CountedCashMinor - expected
		_, err := tx.ExecContext(r.Context(), "UPDATE cash_register_sessions SET closed_by=?,counted_cash_minor=?,expected_cash_minor=?,difference_minor=?,closing_notes=?,closed_at=? WHERE id=? AND closed_at IS NULL", user.ID, input.CountedCashMinor, expected, difference, nilIfEmpty(input.Notes), now, id)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTER_CLOSE_FAILED", "Could not close cash register.")
		return
	}
	s.audit(r.Context(), &user, "close", "cash_register", id, "Closed cash register", "", "", r)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "closedAt": now, "expectedCashMinor": expected, "countedCashMinor": input.CountedCashMinor, "differenceMinor": difference})
}
