package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

func (s *Server) registerIncomeRoutes(r chi.Router) {
	r.With(s.requireDoctor).Get("/income", s.handleIncomeList)
	r.With(s.requireDoctor).Post("/income", s.handleIncomeCreate)
	r.With(s.requireDoctor).Get("/finance/profit-and-loss", s.handleProfitAndLoss)
}

type incomePayload struct {
	Category        string `json:"category"`
	Description     string `json:"description"`
	AmountMinor     int64  `json:"amountMinor"`
	Currency        string `json:"currency"`
	ExchangeRate    string `json:"exchangeRate"`
	ReceivedOn      string `json:"receivedOn"`
	PaymentMethodID string `json:"paymentMethodId"`
	Payer           string `json:"payer"`
	PatientID       string `json:"patientId"`
	Notes           string `json:"notes"`
}

// handleIncomeList reads the income ledger for a period. Sales, refunds and
// insurer remittances are written into it by the database itself as they happen,
// so what is listed here always reconciles with the till.
func (s *Server) handleIncomeList(w http.ResponseWriter, r *http.Request) {
	from, to := reportRange(r)
	where, args := "i.received_on>=? AND i.received_on<=?", []any{from, to}
	if source := strings.TrimSpace(r.URL.Query().Get("source")); source != "" {
		where += " AND i.source=?"
		args = append(args, source)
	}
	paging := paginationFrom(r, 50, 200)
	total, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM income_entries i WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INCOME_LIST_FAILED", "Could not load income.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT i.id,i.source,i.category,i.description,i.amount_minor,i.currency,i.exchange_rate,i.received_on,
		COALESCE(pm.name,''),COALESCE(i.payer,''),COALESCE(i.invoice_id,''),COALESCE(inv.invoice_number,''),COALESCE(i.patient_id,''),COALESCE(pt.first_name||' '||pt.last_name,''),COALESCE(i.notes,''),u.display_name,i.created_at
		FROM income_entries i
		LEFT JOIN payment_methods pm ON pm.id=i.payment_method_id
		LEFT JOIN invoices inv ON inv.id=i.invoice_id
		LEFT JOIN patients pt ON pt.id=i.patient_id
		JOIN users u ON u.id=i.created_by
		WHERE `+where+` ORDER BY i.received_on DESC, i.created_at DESC LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INCOME_LIST_FAILED", "Could not load income.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, source, category, description, currency, rate, receivedOn, method, payer, invoiceID, invoiceNumber, patientID, patientName, notes, recordedBy, createdAt string
		var amount int64
		if err := rows.Scan(&id, &source, &category, &description, &amount, &currency, &rate, &receivedOn, &method, &payer, &invoiceID, &invoiceNumber, &patientID, &patientName, &notes, &recordedBy, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "INCOME_LIST_FAILED", "Could not load income.")
			return
		}
		items = append(items, map[string]any{"id": id, "source": source, "category": category, "description": description, "amountMinor": amount,
			"currency": currency, "exchangeRate": rate, "receivedOn": receivedOn, "paymentMethod": method, "payer": payer,
			"invoiceId": invoiceID, "invoiceNumber": invoiceNumber, "patientId": patientID, "patientName": patientName,
			"notes": notes, "recordedBy": recordedBy, "createdAt": createdAt, "automatic": source != "manual"})
	}
	writeJSON(w, http.StatusOK, withItems(items, mergeMeta(paging.Meta(total), map[string]any{"from": from, "to": to})))
}

// handleIncomeCreate records money that arrived outside the till — a bank
// transfer, a grant, a reimbursement settled directly. Everything the clinic
// sells is already recorded automatically, so a manual entry is only ever for
// what the till never saw.
func (s *Server) handleIncomeCreate(w http.ResponseWriter, r *http.Request) {
	var input incomePayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input.Category, input.Description = strings.TrimSpace(input.Category), strings.TrimSpace(input.Description)
	if input.Category == "" || input.Description == "" || input.AmountMinor <= 0 || input.AmountMinor > maxExactMinor {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_INCOME", "A category, a description and a positive amount are required.")
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
	if input.ReceivedOn == "" {
		input.ReceivedOn = time.Now().UTC().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", input.ReceivedOn); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_INCOME_DATE", "The date received must be a valid date.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO income_entries(id,source,category,description,amount_minor,currency,exchange_rate,received_on,payment_method_id,payer,patient_id,notes,created_at,created_by)
		VALUES(?,'manual',?,?,?,?,?,?,?,?,?,?,?,?)`, id, input.Category, input.Description, input.AmountMinor, input.Currency, input.ExchangeRate, input.ReceivedOn,
		nilIfEmpty(input.PaymentMethodID), nilIfEmpty(strings.TrimSpace(input.Payer)), nilIfEmpty(input.PatientID), nilIfEmpty(strings.TrimSpace(input.Notes)), now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INCOME_CREATE_FAILED", "Could not record the income.")
		return
	}
	s.audit(r.Context(), &user, "create", "income", id, "Recorded income "+input.Description, "", "", r)
	s.broker.Publish(realtime.Event{Type: "income.recorded", EntityType: "income", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "amountMinor": input.AmountMinor, "receivedOn": input.ReceivedOn})
}

// handleProfitAndLoss follows the money end to end for a period: how many visits
// happened, what was invoiced against them, what actually arrived, and what it
// cost. Every figure is converted with the rate stored on the record itself, so
// a period that mixes currencies still adds up in the clinic's base currency.
func (s *Server) handleProfitAndLoss(w http.ResponseWriter, r *http.Request) {
	from, to := reportRange(r)
	var baseCurrency string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.currency'),'HTG') FROM settings WHERE key='clinic'`).Scan(&baseCurrency)
	if baseCurrency == "" {
		baseCurrency = "HTG"
	}
	scalar := func(query string, args ...any) (int64, error) {
		var value int64
		err := s.db.QueryRowContext(r.Context(), query, args...).Scan(&value)
		return value, err
	}
	incomeTotal, err := scalar("SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM income_entries WHERE received_on>=? AND received_on<=?", from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PROFIT_AND_LOSS_FAILED", "Could not build the profit and loss report.")
		return
	}
	expensesTotal, err := scalar("SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM expenses WHERE expense_date>=? AND expense_date<=?", from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PROFIT_AND_LOSS_FAILED", "Could not build the profit and loss report.")
		return
	}
	costOfSales, err := scalar(`SELECT COALESCE(SUM(CAST(ROUND(ii.cost_minor*ii.quantity*CAST(i.exchange_rate AS REAL)) AS INTEGER)),0)
		FROM invoice_items ii JOIN invoices i ON i.id=ii.invoice_id
		WHERE substr(i.created_at,1,10)>=? AND substr(i.created_at,1,10)<=? AND i.status<>'cancelled'`, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PROFIT_AND_LOSS_FAILED", "Could not build the profit and loss report.")
		return
	}
	invoiced, err := scalar("SELECT COALESCE(SUM(CAST(ROUND(total_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM invoices WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND status<>'cancelled'", from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PROFIT_AND_LOSS_FAILED", "Could not build the profit and loss report.")
		return
	}
	count := func(query string) int {
		var value int
		_ = s.db.QueryRowContext(r.Context(), query, from, to).Scan(&value)
		return value
	}
	byCategory, err := s.incomeByCategory(r, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PROFIT_AND_LOSS_FAILED", "Could not build the profit and loss report.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "baseCurrency": baseCurrency,
		"appointments":     count("SELECT COUNT(*) FROM appointments WHERE substr(starts_at,1,10)>=? AND substr(starts_at,1,10)<=? AND archived_at IS NULL AND status<>'cancelled'"),
		"consultations":    count("SELECT COUNT(*) FROM encounters WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND archived_at IS NULL"),
		"sales":            count("SELECT COUNT(*) FROM invoices WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND status<>'cancelled'"),
		"invoicedMinor":    invoiced,
		"incomeMinor":      incomeTotal,
		"costOfSalesMinor": costOfSales,
		"expensesMinor":    expensesTotal,
		"netMinor":         incomeTotal - expensesTotal,
		"grossMarginMinor": incomeTotal - costOfSales,
		"incomeByCategory": byCategory,
	})
}

func (s *Server) incomeByCategory(r *http.Request, from, to string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT category,source,COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0),COUNT(*)
		FROM income_entries WHERE received_on>=? AND received_on<=? GROUP BY category,source ORDER BY 3 DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var category, source string
		var amount int64
		var entries int
		if err := rows.Scan(&category, &source, &amount, &entries); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"category": category, "source": source, "amountMinor": amount, "entries": entries})
	}
	return items, rows.Err()
}
