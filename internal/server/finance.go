package server

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type expensePayload struct {
	Category        string `json:"category"`
	Description     string `json:"description"`
	AmountMinor     int64  `json:"amountMinor"`
	Currency        string `json:"currency"`
	ExchangeRate    string `json:"exchangeRate"`
	ExpenseDate     string `json:"expenseDate"`
	PaymentMethodID string `json:"paymentMethodId"`
	Vendor          string `json:"vendor"`
	DocumentID      string `json:"documentId"`
}

func (s *Server) handleExpensesList(w http.ResponseWriter, r *http.Request) {
	from, to := reportRange(r)
	rows, err := s.db.QueryContext(r.Context(), `SELECT e.id,e.category,e.description,e.amount_minor,e.currency,e.exchange_rate,e.expense_date,COALESCE(pm.name,''),COALESCE(e.vendor,''),COALESCE(e.document_id,''),u.display_name,e.created_at FROM expenses e LEFT JOIN payment_methods pm ON pm.id=e.payment_method_id JOIN users u ON u.id=e.created_by WHERE e.expense_date>=? AND e.expense_date<=? ORDER BY e.expense_date DESC`, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EXPENSE_LIST_FAILED", "Could not load expenses.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, category, description, currency, rate, date, method, vendor, documentID, user, createdAt string
		var amount int64
		if rows.Scan(&id, &category, &description, &amount, &currency, &rate, &date, &method, &vendor, &documentID, &user, &createdAt) == nil {
			items = append(items, map[string]any{"id": id, "category": category, "description": description, "amountMinor": amount, "currency": currency, "exchangeRate": rate, "expenseDate": date, "paymentMethod": method, "vendor": vendor, "documentId": documentID, "createdBy": user, "createdAt": createdAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "from": from, "to": to})
}

func (s *Server) handleExpenseCreate(w http.ResponseWriter, r *http.Request) {
	var input expensePayload
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.Category) == "" || strings.TrimSpace(input.Description) == "" || input.AmountMinor <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Category, description, and a positive expense amount are required.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	if input.ExchangeRate == "" {
		input.ExchangeRate = "1"
	}
	if !validExchangeRate(input.ExchangeRate) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_EXCHANGE_RATE", "Exchange rate must be a positive decimal value.")
		return
	}
	if input.ExpenseDate == "" {
		input.ExpenseDate = time.Now().UTC().Format("2006-01-02")
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO expenses(id,category,description,amount_minor,currency,exchange_rate,expense_date,payment_method_id,vendor,document_id,created_at,created_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, strings.TrimSpace(input.Category), strings.TrimSpace(input.Description), input.AmountMinor, strings.ToUpper(input.Currency), input.ExchangeRate, input.ExpenseDate, nilIfEmpty(input.PaymentMethodID), nilIfEmpty(input.Vendor), nilIfEmpty(input.DocumentID), now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EXPENSE_CREATE_FAILED", "Could not record expense.")
		return
	}
	s.audit(r.Context(), &user, "create", "expense", id, "Recorded clinic expense", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "createdAt": now})
}

func (s *Server) handleFinanceSummary(w http.ResponseWriter, r *http.Request) {
	from, to := reportRange(r)
	var baseCurrency string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.currency'),'HTG') FROM settings WHERE key='clinic'`).Scan(&baseCurrency)
	if baseCurrency == "" {
		baseCurrency = "HTG"
	}
	var grossSales, payments, expenses, outstanding, cost int64
	_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(total_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM invoices WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND status NOT IN ('cancelled')", from, to).Scan(&grossSales)
	_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,10)>=? AND substr(received_at,1,10)<=?", from, to).Scan(&payments)
	_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM expenses WHERE expense_date>=? AND expense_date<=?", from, to).Scan(&expenses)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(CAST(ROUND((i.total_minor-COALESCE(p.paid,0)+COALESCE(ref.refunded,0))*CAST(i.exchange_rate AS REAL)) AS INTEGER)),0) FROM invoices i LEFT JOIN (SELECT invoice_id,SUM(amount_minor) paid FROM payments GROUP BY invoice_id) p ON p.invoice_id=i.id LEFT JOIN (SELECT p.invoice_id,SUM(r.amount_minor) refunded FROM refunds r JOIN payments p ON p.id=r.payment_id GROUP BY p.invoice_id) ref ON ref.invoice_id=i.id WHERE i.status IN ('issued','partially_paid','overdue')`).Scan(&outstanding)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(CAST(ROUND(ii.cost_minor*ii.quantity*CAST(i.exchange_rate AS REAL)) AS INTEGER)),0) FROM invoice_items ii JOIN invoices i ON i.id=ii.invoice_id WHERE substr(i.created_at,1,10)>=? AND substr(i.created_at,1,10)<=? AND i.status<>'cancelled'`, from, to).Scan(&cost)
	byMethod := []map[string]any{}
	rows, _ := s.db.QueryContext(r.Context(), `SELECT pm.name,COALESCE(SUM(CAST(ROUND(p.amount_minor*CAST(p.exchange_rate AS REAL)) AS INTEGER)),0) FROM payment_methods pm LEFT JOIN payments p ON p.payment_method_id=pm.id AND substr(p.received_at,1,10)>=? AND substr(p.received_at,1,10)<=? GROUP BY pm.id,pm.name ORDER BY 2 DESC`, from, to)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var amount int64
			if rows.Scan(&name, &amount) == nil {
				byMethod = append(byMethod, map[string]any{"name": name, "amountMinor": amount})
			}
		}
	}
	byCurrency := []map[string]any{}
	currencyRows, _ := s.db.QueryContext(r.Context(), `SELECT currency,COUNT(*),COALESCE(SUM(total_minor),0) FROM invoices WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND status<>'cancelled' GROUP BY currency ORDER BY currency`, from, to)
	if currencyRows != nil {
		defer currencyRows.Close()
		for currencyRows.Next() {
			var currency string
			var count, amount int64
			if currencyRows.Scan(&currency, &count, &amount) == nil {
				byCurrency = append(byCurrency, map[string]any{"currency": currency, "count": count, "amountMinor": amount})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "baseCurrency": baseCurrency, "grossSalesMinor": grossSales, "paymentsReceivedMinor": payments, "expensesMinor": expenses, "outstandingReceivablesMinor": outstanding, "estimatedCostMinor": cost, "estimatedGrossMarginMinor": grossSales - cost, "paymentsByMethod": byMethod, "salesByCurrency": byCurrency})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	report := chi.URLParam(r, "report")
	from, to := reportRange(r)
	format := r.URL.Query().Get("format")
	type row struct {
		Label  string
		Count  int64
		Amount int64
	}
	rowsOut := []row{}
	var query string
	switch report {
	case "appointments":
		query = `SELECT status,COUNT(*),0 FROM appointments WHERE substr(starts_at,1,10)>=? AND substr(starts_at,1,10)<=? GROUP BY status ORDER BY status`
	case "sales":
		query = `SELECT substr(created_at,1,10)||' · '||currency,COUNT(*),SUM(total_minor) FROM invoices WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND status<>'cancelled' GROUP BY substr(created_at,1,10),currency ORDER BY 1`
	case "inventory":
		query = `SELECT category||' · '||currency,COUNT(*),SUM(quantity*cost_minor) FROM inventory_items WHERE archived_at IS NULL GROUP BY category,currency ORDER BY category,currency`
	case "lab":
		query = `SELECT status,COUNT(*),0 FROM lab_orders WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? GROUP BY status ORDER BY status`
	case "patients":
		query = `SELECT 'new_patients',COUNT(*),0 FROM patients WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? AND archived_at IS NULL`
	case "clinical":
		query = `SELECT status,COUNT(*),0 FROM encounters WHERE substr(created_at,1,10)>=? AND substr(created_at,1,10)<=? GROUP BY status ORDER BY status`
	default:
		writeError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "Report was not found.")
		return
	}
	var rows interface {
		Next() bool
		Scan(...any) error
		Close() error
	}
	var err error
	if report == "inventory" {
		rows, err = s.db.QueryContext(r.Context(), query)
	} else {
		rows, err = s.db.QueryContext(r.Context(), query, from, to)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REPORT_FAILED", "Could not generate report.")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item row
		if rows.Scan(&item.Label, &item.Count, &item.Amount) == nil {
			rowsOut = append(rowsOut, item)
		}
	}
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="sentrymed-`+report+`.csv"`)
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"label", "count", "amount_minor"})
		for _, item := range rowsOut {
			_ = writer.Write([]string{item.Label, strconv.FormatInt(item.Count, 10), strconv.FormatInt(item.Amount, 10)})
		}
		writer.Flush()
		return
	}
	items := []map[string]any{}
	for _, item := range rowsOut {
		items = append(items, map[string]any{"label": item.Label, "count": item.Count, "amountMinor": item.Amount})
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": report, "from": from, "to": to, "items": items})
}

func reportRange(r *http.Request) (string, string) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	now := time.Now().UTC()
	if from == "" {
		from = now.AddDate(0, 0, -30).Format("2006-01-02")
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}
	return from, to
}
