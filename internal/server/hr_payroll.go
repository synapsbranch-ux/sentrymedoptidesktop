package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// ---- Attendance ------------------------------------------------------------

type attendancePayload struct {
	EmployeeID    string `json:"employeeId"`
	WorkDate      string `json:"workDate"`
	ClockIn       string `json:"clockIn"`
	ClockOut      string `json:"clockOut"`
	MinutesWorked int    `json:"minutesWorked"`
	EntryType     string `json:"entryType"`
	Notes         string `json:"notes"`
}

func (s *Server) handleAttendanceList(w http.ResponseWriter, r *http.Request) {
	from, to := reportRange(r)
	where, args := "a.work_date>=? AND a.work_date<=?", []any{from, to}
	if employeeID := strings.TrimSpace(r.URL.Query().Get("employeeId")); employeeID != "" {
		where += " AND a.employee_id=?"
		args = append(args, employeeID)
	}
	paging := paginationFrom(r, 100, 500)
	total, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM attendance_entries a WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not load attendance.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT a.id,a.employee_id,e.first_name||' '||e.last_name,e.employee_number,a.work_date,COALESCE(a.clock_in,''),COALESCE(a.clock_out,''),a.minutes_worked,a.entry_type,COALESCE(a.notes,''),a.version
		FROM attendance_entries a JOIN employees e ON e.id=a.employee_id WHERE `+where+` ORDER BY a.work_date DESC, e.last_name LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not load attendance.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, employeeID, name, number, workDate, clockIn, clockOut, entryType, notes string
		var minutes, version int
		if err := rows.Scan(&id, &employeeID, &name, &number, &workDate, &clockIn, &clockOut, &minutes, &entryType, &notes, &version); err != nil {
			writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not load attendance.")
			return
		}
		items = append(items, map[string]any{"id": id, "employeeId": employeeID, "employeeName": name, "employeeNumber": number, "workDate": workDate,
			"clockIn": clockIn, "clockOut": clockOut, "minutesWorked": minutes, "entryType": entryType, "notes": notes, "version": version, "openShift": entryType == "clocked" && clockOut == ""})
	}
	writeJSON(w, http.StatusOK, withItems(items, mergeMeta(paging.Meta(total), map[string]any{"from": from, "to": to})))
}

// handleAttendanceRecord covers both ways hours arrive: someone clocking in at
// the door, and someone entering a day afterwards from the book. A clocked
// entry with no clock-out is an open shift, and the unique index means a second
// one cannot be opened for the same person on the same day.
func (s *Server) handleAttendanceRecord(w http.ResponseWriter, r *http.Request) {
	var input attendancePayload
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.EmployeeID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "An employee is required.")
		return
	}
	if input.EntryType == "" {
		input.EntryType = "manual"
	}
	if !allowedValue(input.EntryType, []string{"clocked", "manual", "leave", "absent", "holiday"}) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_ATTENDANCE_TYPE", "Attendance type is invalid.")
		return
	}
	now := time.Now().UTC()
	if input.WorkDate == "" {
		input.WorkDate = now.Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", input.WorkDate); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_ATTENDANCE_DATE", "The work date must be a valid date.")
		return
	}
	if input.MinutesWorked < 0 || input.MinutesWorked > 24*60 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_ATTENDANCE_MINUTES", "Minutes worked must be between 0 and a full day.")
		return
	}
	if input.EntryType == "clocked" && input.ClockIn == "" {
		input.ClockIn = now.Format(time.RFC3339)
	}
	var employeeStatus string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM employees WHERE id=? AND archived_at IS NULL", input.EmployeeID).Scan(&employeeStatus); err == sql.ErrNoRows {
		writeError(w, http.StatusUnprocessableEntity, "EMPLOYEE_NOT_FOUND", "That employee was not found.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not record attendance.")
		return
	}
	if employeeStatus == "terminated" {
		writeError(w, http.StatusUnprocessableEntity, "EMPLOYEE_TERMINATED", "Attendance cannot be recorded for a terminated employee.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, timestamp := uuid.NewString(), now.Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO attendance_entries(id,employee_id,work_date,clock_in,clock_out,minutes_worked,entry_type,notes,created_at,updated_at,created_by,updated_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, input.EmployeeID, input.WorkDate, nilIfEmpty(input.ClockIn), nilIfEmpty(input.ClockOut), input.MinutesWorked, input.EntryType, nilIfEmpty(strings.TrimSpace(input.Notes)), timestamp, timestamp, user.ID, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SHIFT_ALREADY_OPEN", "This employee is already clocked in today. Clock them out first.")
			return
		}
		writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not record attendance.")
		return
	}
	s.audit(r.Context(), &user, "create", "attendance", id, "Recorded attendance for "+input.WorkDate, "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "workDate": input.WorkDate, "entryType": input.EntryType, "clockIn": input.ClockIn})
}

// handleAttendanceClockOut closes the open shift and derives the minutes from
// the two timestamps, so the worked total is never typed by hand for a clocked
// shift and never disagrees with the times shown beside it.
func (s *Server) handleAttendanceClockOut(w http.ResponseWriter, r *http.Request) {
	var input struct {
		EmployeeID string `json:"employeeId"`
		Notes      string `json:"notes"`
	}
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.EmployeeID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "An employee is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC()
	var id, clockIn string
	err := s.db.QueryRowContext(r.Context(), "SELECT id,COALESCE(clock_in,'') FROM attendance_entries WHERE employee_id=? AND entry_type='clocked' AND clock_out IS NULL ORDER BY work_date DESC LIMIT 1", input.EmployeeID).Scan(&id, &clockIn)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnprocessableEntity, "NO_OPEN_SHIFT", "This employee has no open shift to close.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not close the shift.")
		return
	}
	minutes := 0
	if started, parseErr := time.Parse(time.RFC3339, clockIn); parseErr == nil {
		if elapsed := int(now.Sub(started).Minutes()); elapsed > 0 {
			minutes = min(elapsed, 24*60)
		}
	}
	if _, err := s.db.ExecContext(r.Context(), "UPDATE attendance_entries SET clock_out=?,minutes_worked=?,notes=COALESCE(?,notes),version=version+1,updated_at=?,updated_by=? WHERE id=?",
		now.Format(time.RFC3339), minutes, nilIfEmpty(strings.TrimSpace(input.Notes)), now.Format(time.RFC3339Nano), user.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "ATTENDANCE_FAILED", "Could not close the shift.")
		return
	}
	s.audit(r.Context(), &user, "update", "attendance", id, "Closed a shift", "", "", r)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "clockOut": now.Format(time.RFC3339), "minutesWorked": minutes})
}

// ---- Payroll ---------------------------------------------------------------

// payFor computes one employee's gross for a period. Salaried staff are paid
// their agreed figure; hourly staff are paid for the minutes attendance
// actually recorded, because that is the only number anyone can check.
func payFor(payBasis string, baseSalary, hourlyRate int64, minutes int) (int64, string) {
	if payBasis == "hourly" {
		gross := hourlyRate * int64(minutes) / 60
		return gross, fmt.Sprintf("%d min at %d per hour", minutes, hourlyRate)
	}
	return baseSalary, "Agreed " + payBasis + " salary"
}

func applyDeductions(gross int64, raw string) (int64, []map[string]any) {
	var configured []payDeduction
	_ = json.Unmarshal([]byte(raw), &configured)
	applied := []map[string]any{}
	var total int64
	for _, deduction := range configured {
		amount := int64(deduction.Value)
		if deduction.Type == "percent" {
			amount = int64(float64(gross) * deduction.Value / 100)
		}
		if amount < 0 {
			amount = 0
		}
		// Deductions never take a payslip below zero: what cannot be taken this
		// period is a matter for the clinic, not something to invent as a debt.
		if total+amount > gross {
			amount = gross - total
		}
		total += amount
		applied = append(applied, map[string]any{"name": deduction.Name, "type": deduction.Type, "value": deduction.Value, "amountMinor": amount})
	}
	return total, applied
}

func (s *Server) handlePayrollRunCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PeriodStart string `json:"periodStart"`
		PeriodEnd   string `json:"periodEnd"`
		Notes       string `json:"notes"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	start, startErr := time.Parse("2006-01-02", input.PeriodStart)
	end, endErr := time.Parse("2006-01-02", input.PeriodEnd)
	if startErr != nil || endErr != nil || end.Before(start) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PAYROLL_PERIOD", "Give a valid period with an end on or after its start.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	var gross, deductions, net int64
	var slips int
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var overlapping int
		// Two runs covering the same day would pay the same work twice.
		if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM payroll_runs WHERE status<>'cancelled' AND period_start<=? AND period_end>=?", input.PeriodEnd, input.PeriodStart).Scan(&overlapping); err != nil {
			return err
		}
		if overlapping > 0 {
			return &APIError{Code: "PAYROLL_PERIOD_OVERLAPS", Message: "Another payroll run already covers part of this period."}
		}
		var err error
		number, err = s.nextNumber(r.Context(), tx, "payroll", "PAY", true)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(r.Context(), `SELECT e.id,e.pay_basis,e.base_salary_minor,e.hourly_rate_minor,e.currency,e.deductions_json,
			COALESCE((SELECT SUM(a.minutes_worked) FROM attendance_entries a WHERE a.employee_id=e.id AND a.work_date>=? AND a.work_date<=?),0),
			COALESCE((SELECT COUNT(DISTINCT a.work_date) FROM attendance_entries a WHERE a.employee_id=e.id AND a.work_date>=? AND a.work_date<=? AND a.minutes_worked>0),0)
			FROM employees e WHERE e.archived_at IS NULL AND e.status IN ('active','on_leave')`,
			input.PeriodStart, input.PeriodEnd, input.PeriodStart, input.PeriodEnd)
		if err != nil {
			return err
		}
		type computed struct {
			employeeID, currency, basis string
			gross, deducted             int64
			minutes, days               int
			breakdown                   map[string]any
		}
		payslips := []computed{}
		var runCurrency string
		for rows.Next() {
			var employeeID, basis, currency, deductionsJSON string
			var salary, hourly int64
			var minutes, days int
			if err := rows.Scan(&employeeID, &basis, &salary, &hourly, &currency, &deductionsJSON, &minutes, &days); err != nil {
				_ = rows.Close()
				return err
			}
			employeeGross, note := payFor(basis, salary, hourly, minutes)
			deducted, applied := applyDeductions(employeeGross, deductionsJSON)
			if runCurrency == "" {
				runCurrency = currency
			}
			payslips = append(payslips, computed{employeeID: employeeID, currency: currency, basis: basis, gross: employeeGross, deducted: deducted, minutes: minutes, days: days,
				breakdown: map[string]any{"basis": basis, "note": note, "currency": currency, "deductions": applied}})
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(payslips) == 0 {
			return &APIError{Code: "NO_EMPLOYEES_TO_PAY", Message: "No active employees fall in this period."}
		}
		if runCurrency == "" {
			runCurrency = "HTG"
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO payroll_runs(id,run_number,period_start,period_end,status,currency,notes,created_at,updated_at,created_by,updated_by)
			VALUES(?,?,?,?, 'draft',?,?,?,?,?,?)`, id, number, input.PeriodStart, input.PeriodEnd, runCurrency, nilIfEmpty(strings.TrimSpace(input.Notes)), now, now, user.ID, user.ID); err != nil {
			return err
		}
		for _, slip := range payslips {
			breakdown, _ := json.Marshal(slip.breakdown)
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO payslips(id,payroll_run_id,employee_id,gross_minor,deductions_minor,net_minor,minutes_worked,days_present,breakdown_json,created_at)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), id, slip.employeeID, slip.gross, slip.deducted, slip.gross-slip.deducted, slip.minutes, slip.days, string(breakdown), now); err != nil {
				return err
			}
			gross += slip.gross
			deductions += slip.deducted
			net += slip.gross - slip.deducted
		}
		slips = len(payslips)
		_, err = tx.ExecContext(r.Context(), "UPDATE payroll_runs SET gross_minor=?,deductions_minor=?,net_minor=? WHERE id=?", gross, deductions, net, id)
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "PAYROLL_RUN_FAILED", "Could not prepare the payroll run.")
		return
	}
	s.audit(r.Context(), &user, "create", "payroll_run", id, fmt.Sprintf("Prepared payroll %s for %d employee(s)", number, slips), "", "", r)
	s.broker.Publish(realtime.Event{Type: "payroll.changed", EntityType: "payroll_run", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "runNumber": number, "status": "draft", "payslips": slips, "grossMinor": gross, "deductionsMinor": deductions, "netMinor": net, "version": 1})
}

func (s *Server) handlePayrollRunsList(w http.ResponseWriter, r *http.Request) {
	paging := paginationFrom(r, 50, 200)
	total, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM payroll_runs")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYROLL_LIST_FAILED", "Could not load payroll runs.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT r.id,r.run_number,r.period_start,r.period_end,r.status,r.currency,r.gross_minor,r.deductions_minor,r.net_minor,r.version,r.created_at,
		(SELECT COUNT(*) FROM payslips p WHERE p.payroll_run_id=r.id)
		FROM payroll_runs r ORDER BY r.period_start DESC LIMIT ? OFFSET ?`, paging.Limit, paging.Offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYROLL_LIST_FAILED", "Could not load payroll runs.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, start, end, status, currency, createdAt string
		var gross, deductions, net int64
		var version, slips int
		if err := rows.Scan(&id, &number, &start, &end, &status, &currency, &gross, &deductions, &net, &version, &createdAt, &slips); err != nil {
			writeError(w, http.StatusInternalServerError, "PAYROLL_LIST_FAILED", "Could not load payroll runs.")
			return
		}
		items = append(items, map[string]any{"id": id, "runNumber": number, "periodStart": start, "periodEnd": end, "status": status, "currency": currency,
			"grossMinor": gross, "deductionsMinor": deductions, "netMinor": net, "payslips": slips, "version": version, "createdAt": createdAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(total)))
}

func (s *Server) handlePayrollRunGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var number, start, end, status, currency, notes, approvedAt, paidAt, createdAt string
	var gross, deductions, net int64
	var version int
	err := s.db.QueryRowContext(r.Context(), `SELECT run_number,period_start,period_end,status,currency,COALESCE(notes,''),gross_minor,deductions_minor,net_minor,COALESCE(approved_at,''),COALESCE(paid_at,''),version,created_at
		FROM payroll_runs WHERE id=?`, id).Scan(&number, &start, &end, &status, &currency, &notes, &gross, &deductions, &net, &approvedAt, &paidAt, &version, &createdAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PAYROLL_RUN_NOT_FOUND", "That payroll run was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYROLL_LOAD_FAILED", "Could not load the payroll run.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.employee_id,e.employee_number,e.first_name||' '||e.last_name,COALESCE(pos.title,''),p.gross_minor,p.deductions_minor,p.net_minor,p.minutes_worked,p.days_present,p.breakdown_json
		FROM payslips p JOIN employees e ON e.id=p.employee_id LEFT JOIN hr_positions pos ON pos.id=e.position_id WHERE p.payroll_run_id=? ORDER BY e.last_name, e.first_name`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYROLL_LOAD_FAILED", "Could not load the payroll run.")
		return
	}
	defer rows.Close()
	slips := []map[string]any{}
	for rows.Next() {
		var slipID, employeeID, employeeNumber, name, position, breakdown string
		var slipGross, slipDeductions, slipNet int64
		var minutes, days int
		if err := rows.Scan(&slipID, &employeeID, &employeeNumber, &name, &position, &slipGross, &slipDeductions, &slipNet, &minutes, &days, &breakdown); err != nil {
			writeError(w, http.StatusInternalServerError, "PAYROLL_LOAD_FAILED", "Could not load the payroll run.")
			return
		}
		slips = append(slips, map[string]any{"id": slipID, "employeeId": employeeID, "employeeNumber": employeeNumber, "employeeName": name, "position": position,
			"grossMinor": slipGross, "deductionsMinor": slipDeductions, "netMinor": slipNet, "minutesWorked": minutes, "daysPresent": days, "breakdown": rawJSON(breakdown)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "runNumber": number, "periodStart": start, "periodEnd": end, "status": status, "currency": currency, "notes": notes,
		"grossMinor": gross, "deductionsMinor": deductions, "netMinor": net, "approvedAt": approvedAt, "paidAt": paidAt, "version": version, "createdAt": createdAt, "payslips": slips})
}

// handlePayrollRunStatus moves a run forward. The order matters: a run is
// approved before it is paid, and neither a paid nor a cancelled run changes
// again, so a payslip somebody has been paid on stays as it was.
func (s *Server) handlePayrollRunStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A status and the current version are required.")
		return
	}
	allowedFrom := map[string][]string{"approved": {"draft"}, "paid": {"approved"}, "cancelled": {"draft", "approved"}}
	sources, valid := allowedFrom[input.Status]
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PAYROLL_STATUS", "A payroll run can be approved, paid or cancelled.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(sources)), ",")
	args := []any{input.Status}
	if input.Status == "approved" {
		args = append(args, now, user.ID, nil)
	} else if input.Status == "paid" {
		args = append(args, nil, nil, now)
	} else {
		args = append(args, nil, nil, nil)
	}
	args = append(args, now, user.ID, id, input.Version)
	for _, source := range sources {
		args = append(args, source)
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE payroll_runs SET status=?,approved_at=COALESCE(?,approved_at),approved_by=COALESCE(?,approved_by),paid_at=COALESCE(?,paid_at),version=version+1,updated_at=?,updated_by=?
		WHERE id=? AND version=? AND status IN (`+placeholders+`)`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYROLL_STATUS_FAILED", "Could not update the payroll run.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "PAYROLL_NOT_IN_THAT_STATE", "This payroll run changed, or it cannot move to that state from where it is.")
		return
	}
	s.audit(r.Context(), &user, "update", "payroll_run", id, "Payroll run marked "+input.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "payroll.changed", EntityType: "payroll_run", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status, "version": input.Version + 1})
}

// ---- Documents -------------------------------------------------------------

func (s *Server) handleDocumentTemplatesList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,kind,title,body,active,version FROM hr_document_templates WHERE active=1 ORDER BY title")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TEMPLATES_FAILED", "Could not load document templates.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, kind, title, body string
		var active bool
		var version int
		if err := rows.Scan(&id, &kind, &title, &body, &active, &version); err != nil {
			writeError(w, http.StatusInternalServerError, "TEMPLATES_FAILED", "Could not load document templates.")
			return
		}
		items = append(items, map[string]any{"id": id, "kind": kind, "title": title, "body": body, "active": active, "version": version})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleHRDocumentsList(w http.ResponseWriter, r *http.Request) {
	employeeID := strings.TrimSpace(r.URL.Query().Get("employeeId"))
	where, args := "1=1", []any{}
	if employeeID != "" {
		where = "d.employee_id=?"
		args = append(args, employeeID)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT d.id,d.employee_id,e.first_name||' '||e.last_name,d.kind,d.title,d.body,d.generated_at,u.display_name
		FROM hr_documents d JOIN employees e ON e.id=d.employee_id JOIN users u ON u.id=d.generated_by WHERE `+where+` ORDER BY d.generated_at DESC LIMIT 200`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "HR_DOCUMENTS_FAILED", "Could not load staff documents.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, employee, name, kind, title, body, generatedAt, generatedBy string
		if err := rows.Scan(&id, &employee, &name, &kind, &title, &body, &generatedAt, &generatedBy); err != nil {
			writeError(w, http.StatusInternalServerError, "HR_DOCUMENTS_FAILED", "Could not load staff documents.")
			return
		}
		items = append(items, map[string]any{"id": id, "employeeId": employee, "employeeName": name, "kind": kind, "title": title, "body": body, "generatedAt": generatedAt, "generatedBy": generatedBy})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleHRDocumentGenerate fills a template from the employee's own record and
// stores the result as generated. Editing the template afterwards never changes
// a document already handed to someone.
func (s *Server) handleHRDocumentGenerate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		EmployeeID string `json:"employeeId"`
		TemplateID string `json:"templateId"`
		Title      string `json:"title"`
	}
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.EmployeeID) == "" || strings.TrimSpace(input.TemplateID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "An employee and a template are required.")
		return
	}
	var kind, templateTitle, body string
	if err := s.db.QueryRowContext(r.Context(), "SELECT kind,title,body FROM hr_document_templates WHERE id=? AND active=1", input.TemplateID).Scan(&kind, &templateTitle, &body); err == sql.ErrNoRows {
		writeError(w, http.StatusUnprocessableEntity, "TEMPLATE_NOT_FOUND", "That document template was not found.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "HR_DOCUMENT_FAILED", "Could not generate the document.")
		return
	}
	fields, err := s.employeeTemplateFields(r, input.EmployeeID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnprocessableEntity, "EMPLOYEE_NOT_FOUND", "That employee was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "HR_DOCUMENT_FAILED", "Could not generate the document.")
		return
	}
	for placeholder, value := range fields {
		body = strings.ReplaceAll(body, "{{"+placeholder+"}}", value)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = templateTitle + " — " + fields["employeeName"]
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(r.Context(), "INSERT INTO hr_documents(id,employee_id,kind,title,body,generated_at,generated_by) VALUES(?,?,?,?,?,?,?)",
		id, input.EmployeeID, kind, title, body, now, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "HR_DOCUMENT_FAILED", "Could not store the generated document.")
		return
	}
	s.audit(r.Context(), &user, "generate", "hr_document", id, "Generated "+kind+" for an employee", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "kind": kind, "title": title, "body": body, "generatedAt": now})
}

func (s *Server) employeeTemplateFields(r *http.Request, employeeID string) (map[string]string, error) {
	var first, last, position, employmentType, startedOn, payBasis, currency, address string
	var salary, hourly int64
	err := s.db.QueryRowContext(r.Context(), `SELECT e.first_name,e.last_name,COALESCE(p.title,''),e.employment_type,COALESCE(e.started_on,''),e.pay_basis,e.base_salary_minor,e.hourly_rate_minor,e.currency,COALESCE(e.address,'')
		FROM employees e LEFT JOIN hr_positions p ON p.id=e.position_id WHERE e.id=? AND e.archived_at IS NULL`, employeeID).
		Scan(&first, &last, &position, &employmentType, &startedOn, &payBasis, &salary, &hourly, &currency, &address)
	if err != nil {
		return nil, err
	}
	var clinicName, clinicAddress string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.name'),''),COALESCE(json_extract(value_json,'$.address'),'') FROM settings WHERE key='clinic'`).Scan(&clinicName, &clinicAddress)
	payDescription := fmt.Sprintf("%d (minor units) %s", salary, payBasis)
	if payBasis == "hourly" {
		payDescription = fmt.Sprintf("%d (minor units) per hour worked", hourly)
	}
	return map[string]string{
		"employeeName": first + " " + last, "employeeAddress": address, "position": position,
		"employmentType": employmentType, "startedOn": startedOn, "payDescription": payDescription, "currency": currency,
		"clinicName": clinicName, "clinicAddress": clinicAddress, "today": time.Now().UTC().Format("2006-01-02"),
	}, nil
}
