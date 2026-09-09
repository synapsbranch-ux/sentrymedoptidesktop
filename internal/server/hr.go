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

// Staff records, pay and contracts are administration, not clinical work, so the
// whole module sits behind the doctor role the way finance does.
func (s *Server) registerHumanResourcesRoutes(r chi.Router) {
	hr := r.With(s.requireDoctor)
	hr.Get("/hr/positions", s.handlePositionsList)
	hr.Post("/hr/positions", s.handlePositionCreate)
	hr.Get("/hr/employees", s.handleEmployeesList)
	hr.Get("/hr/employees/{id}", s.handleEmployeeGet)
	hr.Post("/hr/employees", s.handleEmployeeCreate)
	hr.Put("/hr/employees/{id}", s.handleEmployeeUpdate)
	hr.Get("/hr/attendance", s.handleAttendanceList)
	hr.Post("/hr/attendance", s.handleAttendanceRecord)
	hr.Post("/hr/attendance/clock-out", s.handleAttendanceClockOut)
	hr.Get("/hr/payroll-runs", s.handlePayrollRunsList)
	hr.Get("/hr/payroll-runs/{id}", s.handlePayrollRunGet)
	hr.Post("/hr/payroll-runs", s.handlePayrollRunCreate)
	hr.Patch("/hr/payroll-runs/{id}/status", s.handlePayrollRunStatus)
	hr.Get("/hr/document-templates", s.handleDocumentTemplatesList)
	hr.Get("/hr/documents", s.handleHRDocumentsList)
	hr.Post("/hr/documents", s.handleHRDocumentGenerate)
}

// ---- Positions -------------------------------------------------------------

func (s *Server) handlePositionsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.title,COALESCE(p.description,''),p.default_salary_minor,p.currency,p.active,p.version,
		(SELECT COUNT(*) FROM employees e WHERE e.position_id=p.id AND e.archived_at IS NULL)
		FROM hr_positions p ORDER BY p.active DESC, p.title`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "POSITIONS_FAILED", "Could not load positions.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, title, description, currency string
		var salary int64
		var active bool
		var version, staff int
		if err := rows.Scan(&id, &title, &description, &salary, &currency, &active, &version, &staff); err != nil {
			writeError(w, http.StatusInternalServerError, "POSITIONS_FAILED", "Could not load positions.")
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "description": description, "defaultSalaryMinor": salary, "currency": currency, "active": active, "version": version, "staffCount": staff})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePositionCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title              string `json:"title"`
		Description        string `json:"description"`
		DefaultSalaryMinor int64  `json:"defaultSalaryMinor"`
		Currency           string `json:"currency"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || input.DefaultSalaryMinor < 0 || input.DefaultSalaryMinor > maxExactMinor {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_POSITION", "A title and a non-negative default salary are required.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), "INSERT INTO hr_positions(id,title,description,default_salary_minor,currency,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?)",
		id, input.Title, nilIfEmpty(input.Description), input.DefaultSalaryMinor, strings.ToUpper(input.Currency), now, now, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "POSITION_EXISTS", "A position with that title already exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "POSITION_CREATE_FAILED", "Could not create the position.")
		return
	}
	s.audit(r.Context(), &user, "create", "hr_position", id, "Created position "+input.Title, "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "title": input.Title, "version": 1})
}

// ---- Employees -------------------------------------------------------------

type employeePayload struct {
	FirstName       string           `json:"firstName"`
	LastName        string           `json:"lastName"`
	PositionID      string           `json:"positionId"`
	UserID          string           `json:"userId"`
	EmploymentType  string           `json:"employmentType"`
	Status          string           `json:"status"`
	StartedOn       string           `json:"startedOn"`
	EndedOn         string           `json:"endedOn"`
	PayBasis        string           `json:"payBasis"`
	BaseSalaryMinor int64            `json:"baseSalaryMinor"`
	HourlyRateMinor int64            `json:"hourlyRateMinor"`
	Currency        string           `json:"currency"`
	Deductions      []payDeduction   `json:"deductions"`
	Phone           string           `json:"phone"`
	Email           string           `json:"email"`
	Address         string           `json:"address"`
	NationalID      string           `json:"nationalId"`
	BankAccount     string           `json:"bankAccount"`
	Notes           string           `json:"notes"`
	Version         int              `json:"version"`
	_               struct{ _ byte } // keeps the struct addressable-only by name
}

// A deduction is either a fixed amount or a percentage of gross. Percentages are
// resolved against gross at the moment the payslip is computed and then stored,
// so changing a rate never rewrites a slip somebody has already been paid on.
type payDeduction struct {
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}

func validateEmployee(input *employeePayload) string {
	input.FirstName, input.LastName = strings.TrimSpace(input.FirstName), strings.TrimSpace(input.LastName)
	if input.FirstName == "" || input.LastName == "" {
		return "An employee needs a first and last name."
	}
	if input.EmploymentType == "" {
		input.EmploymentType = "employee"
	}
	if !allowedValue(input.EmploymentType, []string{"employee", "contractor"}) {
		return "Employment type must be employee or contractor."
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if !allowedValue(input.Status, []string{"active", "on_leave", "suspended", "terminated"}) {
		return "Employment status is invalid."
	}
	if input.PayBasis == "" {
		input.PayBasis = "monthly"
	}
	if !allowedValue(input.PayBasis, []string{"monthly", "biweekly", "weekly", "hourly"}) {
		return "Pay basis must be monthly, biweekly, weekly or hourly."
	}
	if input.BaseSalaryMinor < 0 || input.HourlyRateMinor < 0 || input.BaseSalaryMinor > maxExactMinor || input.HourlyRateMinor > maxExactMinor {
		return "Pay amounts must be non-negative."
	}
	if input.PayBasis == "hourly" && input.HourlyRateMinor == 0 {
		return "An hourly employee needs an hourly rate."
	}
	for _, date := range []string{input.StartedOn, input.EndedOn} {
		if date == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return "Employment dates must be valid dates."
		}
	}
	if input.EndedOn != "" && input.StartedOn != "" && input.EndedOn < input.StartedOn {
		return "An employment end date cannot come before its start date."
	}
	for _, deduction := range input.Deductions {
		if strings.TrimSpace(deduction.Name) == "" || (deduction.Type != "fixed" && deduction.Type != "percent") || deduction.Value < 0 {
			return "Every deduction needs a name, a type of fixed or percent, and a non-negative value."
		}
		if deduction.Type == "percent" && deduction.Value > 100 {
			return "A percentage deduction cannot exceed 100%."
		}
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	input.Currency = strings.ToUpper(input.Currency)
	if len(input.Currency) != 3 {
		return "Currency must be a three-letter code."
	}
	return ""
}

func (s *Server) handleEmployeesList(w http.ResponseWriter, r *http.Request) {
	where, args := "e.archived_at IS NULL", []any{}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where += " AND e.status=?"
		args = append(args, status)
	}
	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
		where += " AND (e.first_name LIKE ? OR e.last_name LIKE ? OR e.employee_number LIKE ?)"
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}
	paging := paginationFrom(r, 50, 200)
	total, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM employees e WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMPLOYEE_LIST_FAILED", "Could not load employees.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT e.id,e.employee_number,e.first_name,e.last_name,COALESCE(p.title,''),COALESCE(e.position_id,''),e.employment_type,e.status,
		COALESCE(e.started_on,''),COALESCE(e.ended_on,''),e.pay_basis,e.base_salary_minor,e.hourly_rate_minor,e.currency,COALESCE(e.phone,''),COALESCE(e.email,''),e.version,e.updated_at
		FROM employees e LEFT JOIN hr_positions p ON p.id=e.position_id
		WHERE `+where+` ORDER BY e.last_name, e.first_name LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMPLOYEE_LIST_FAILED", "Could not load employees.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, first, last, position, positionID, employmentType, status, startedOn, endedOn, payBasis, currency, phone, email, updatedAt string
		var salary, hourly int64
		var version int
		if err := rows.Scan(&id, &number, &first, &last, &position, &positionID, &employmentType, &status, &startedOn, &endedOn, &payBasis, &salary, &hourly, &currency, &phone, &email, &version, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "EMPLOYEE_LIST_FAILED", "Could not load employees.")
			return
		}
		items = append(items, map[string]any{"id": id, "employeeNumber": number, "firstName": first, "lastName": last, "position": position, "positionId": positionID,
			"employmentType": employmentType, "status": status, "startedOn": startedOn, "endedOn": endedOn, "payBasis": payBasis,
			"baseSalaryMinor": salary, "hourlyRateMinor": hourly, "currency": currency, "phone": phone, "email": email, "version": version, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(total)))
}

func (s *Server) handleEmployeeGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var number, first, last, position, positionID, userID, employmentType, status, startedOn, endedOn, payBasis, currency, deductions, phone, email, address, nationalID, bank, notes, createdAt, updatedAt string
	var salary, hourly int64
	var version int
	err := s.db.QueryRowContext(r.Context(), `SELECT e.employee_number,e.first_name,e.last_name,COALESCE(p.title,''),COALESCE(e.position_id,''),COALESCE(e.user_id,''),e.employment_type,e.status,
		COALESCE(e.started_on,''),COALESCE(e.ended_on,''),e.pay_basis,e.base_salary_minor,e.hourly_rate_minor,e.currency,e.deductions_json,
		COALESCE(e.phone,''),COALESCE(e.email,''),COALESCE(e.address,''),COALESCE(e.national_id,''),COALESCE(e.bank_account,''),COALESCE(e.notes,''),e.version,e.created_at,e.updated_at
		FROM employees e LEFT JOIN hr_positions p ON p.id=e.position_id WHERE e.id=? AND e.archived_at IS NULL`, id).
		Scan(&number, &first, &last, &position, &positionID, &userID, &employmentType, &status, &startedOn, &endedOn, &payBasis, &salary, &hourly, &currency, &deductions,
			&phone, &email, &address, &nationalID, &bank, &notes, &version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "Employee was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMPLOYEE_LOAD_FAILED", "Could not load the employee.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "employeeNumber": number, "firstName": first, "lastName": last, "position": position, "positionId": positionID,
		"userId": userID, "employmentType": employmentType, "status": status, "startedOn": startedOn, "endedOn": endedOn, "payBasis": payBasis,
		"baseSalaryMinor": salary, "hourlyRateMinor": hourly, "currency": currency, "deductions": rawJSON(deductions),
		"phone": phone, "email": email, "address": address, "nationalId": nationalID, "bankAccount": bank, "notes": notes,
		"version": version, "createdAt": createdAt, "updatedAt": updatedAt})
}

func (s *Server) handleEmployeeCreate(w http.ResponseWriter, r *http.Request) {
	var input employeePayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if message := validateEmployee(&input); message != "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	deductions, _ := json.Marshal(input.Deductions)
	if input.Deductions == nil {
		deductions = []byte("[]")
	}
	var number string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "employee", "EMP", false)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO employees(id,employee_number,user_id,first_name,last_name,position_id,employment_type,status,started_on,ended_on,pay_basis,base_salary_minor,hourly_rate_minor,currency,deductions_json,phone,email,address,national_id,bank_account,notes,created_at,updated_at,created_by,updated_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, number, nilIfEmpty(input.UserID), input.FirstName, input.LastName, nilIfEmpty(input.PositionID),
			input.EmploymentType, input.Status, nilIfEmpty(input.StartedOn), nilIfEmpty(input.EndedOn), input.PayBasis, input.BaseSalaryMinor, input.HourlyRateMinor, input.Currency, string(deductions),
			nilIfEmpty(input.Phone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.NationalID), nilIfEmpty(input.BankAccount), nilIfEmpty(input.Notes), now, now, user.ID, user.ID)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMPLOYEE_CREATE_FAILED", "Could not create the employee record.")
		return
	}
	s.audit(r.Context(), &user, "create", "employee", id, "Created employee "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "employee.changed", EntityType: "employee", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "employeeNumber": number, "version": 1})
}

func (s *Server) handleEmployeeUpdate(w http.ResponseWriter, r *http.Request) {
	var input employeePayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Employee details and the current version are required.")
		return
	}
	if message := validateEmployee(&input); message != "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	deductions, _ := json.Marshal(input.Deductions)
	if input.Deductions == nil {
		deductions = []byte("[]")
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE employees SET user_id=?,first_name=?,last_name=?,position_id=?,employment_type=?,status=?,started_on=?,ended_on=?,pay_basis=?,base_salary_minor=?,hourly_rate_minor=?,currency=?,deductions_json=?,phone=?,email=?,address=?,national_id=?,bank_account=?,notes=?,version=version+1,updated_at=?,updated_by=?
		WHERE id=? AND version=? AND archived_at IS NULL`, nilIfEmpty(input.UserID), input.FirstName, input.LastName, nilIfEmpty(input.PositionID), input.EmploymentType, input.Status,
		nilIfEmpty(input.StartedOn), nilIfEmpty(input.EndedOn), input.PayBasis, input.BaseSalaryMinor, input.HourlyRateMinor, input.Currency, string(deductions),
		nilIfEmpty(input.Phone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.NationalID), nilIfEmpty(input.BankAccount), nilIfEmpty(input.Notes), now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMPLOYEE_UPDATE_FAILED", "Could not update the employee record.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This employee record changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "employee", id, "Updated employee record", "", "", r)
	s.broker.Publish(realtime.Event{Type: "employee.changed", EntityType: "employee", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}
