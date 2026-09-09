package server

import (
	"net/http"
	"strings"
	"testing"
)

func (a *testApp) createPosition(t *testing.T, title string, salary int64) string {
	t.Helper()
	response := a.request(http.MethodPost, "/api/v1/hr/positions", map[string]any{"title": title, "defaultSalaryMinor": salary, "currency": "HTG"}, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("create position: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](t, response)["id"].(string)
}

func (a *testApp) createEmployee(t *testing.T, body map[string]any) string {
	t.Helper()
	response := a.request(http.MethodPost, "/api/v1/hr/employees", body, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("create employee: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](t, response)["id"].(string)
}

func TestStaffRecordsCarryPositionAndPayTerms(t *testing.T) {
	a := newTestApp(t)
	positionID := a.createPosition(t, "Optical assistant", 3000000)
	id := a.createEmployee(t, map[string]any{
		"firstName": "Nadège", "lastName": "Pierre", "positionId": positionID, "employmentType": "employee",
		"payBasis": "monthly", "baseSalaryMinor": 3000000, "currency": "HTG", "startedOn": "2026-01-15",
		"deductions": []map[string]any{{"name": "Pension", "type": "percent", "value": 5}},
	})
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/hr/employees/"+id, nil, a.doctor))
	if detail["position"] != "Optical assistant" || detail["employeeNumber"] == "" {
		t.Fatalf("the employee record is missing its position or number: %v", detail)
	}
	deductions, _ := detail["deductions"].([]any)
	if len(deductions) != 1 {
		t.Fatalf("the agreed deduction was lost: %v", detail["deductions"])
	}
}

func TestAStaffRecordIsValidatedBeforeItIsKept(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []map[string]any{
		{"firstName": " ", "lastName": "Pierre"},
		{"firstName": "Nadège", "lastName": "Pierre", "employmentType": "volunteer"},
		{"firstName": "Nadège", "lastName": "Pierre", "payBasis": "hourly", "hourlyRateMinor": 0},
		{"firstName": "Nadège", "lastName": "Pierre", "startedOn": "2026-05-01", "endedOn": "2026-01-01"},
		{"firstName": "Nadège", "lastName": "Pierre", "deductions": []map[string]any{{"name": "Tax", "type": "percent", "value": 140}}},
	} {
		if response := a.request(http.MethodPost, "/api/v1/hr/employees", body, a.doctor); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v = %d, want 422", body, response.Code)
		}
	}
}

func TestAShiftIsClockedInAndOutOnceEach(t *testing.T) {
	a := newTestApp(t)
	id := a.createEmployee(t, map[string]any{"firstName": "Jean", "lastName": "Baptiste", "payBasis": "hourly", "hourlyRateMinor": 50000})

	clockedIn := a.request(http.MethodPost, "/api/v1/hr/attendance", map[string]any{"employeeId": id, "entryType": "clocked"}, a.doctor)
	if clockedIn.Code != http.StatusCreated {
		t.Fatalf("clock in: %d %s", clockedIn.Code, clockedIn.Body.String())
	}
	// A second clock-in without a clock-out would double the day's hours.
	if again := a.request(http.MethodPost, "/api/v1/hr/attendance", map[string]any{"employeeId": id, "entryType": "clocked"}, a.doctor); again.Code != http.StatusConflict {
		t.Fatalf("second clock in = %d, want 409", again.Code)
	}
	out := a.request(http.MethodPost, "/api/v1/hr/attendance/clock-out", map[string]any{"employeeId": id}, a.doctor)
	if out.Code != http.StatusOK {
		t.Fatalf("clock out: %d %s", out.Code, out.Body.String())
	}
	if second := a.request(http.MethodPost, "/api/v1/hr/attendance/clock-out", map[string]any{"employeeId": id}, a.doctor); second.Code != http.StatusUnprocessableEntity {
		t.Fatalf("clocking out with no open shift = %d, want 422", second.Code)
	}
}

func TestAttendanceIsRefusedForSomeoneWhoHasLeft(t *testing.T) {
	a := newTestApp(t)
	id := a.createEmployee(t, map[string]any{"firstName": "Gone", "lastName": "Already", "status": "terminated"})
	if response := a.request(http.MethodPost, "/api/v1/hr/attendance", map[string]any{"employeeId": id, "minutesWorked": 480}, a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("attendance for a terminated employee = %d, want 422", response.Code)
	}
}

func TestPayrollPaysSalaryFlatAndHourlyByTheHoursRecorded(t *testing.T) {
	a := newTestApp(t)
	salaried := a.createEmployee(t, map[string]any{"firstName": "Salaried", "lastName": "Staff", "payBasis": "monthly", "baseSalaryMinor": 4000000, "currency": "HTG",
		"deductions": []map[string]any{{"name": "Pension", "type": "percent", "value": 10}}})
	hourly := a.createEmployee(t, map[string]any{"firstName": "Hourly", "lastName": "Staff", "payBasis": "hourly", "hourlyRateMinor": 30000, "currency": "HTG"})

	// Ten hours across the period for the hourly employee.
	for _, day := range []string{"2026-06-01", "2026-06-02"} {
		if code := a.request(http.MethodPost, "/api/v1/hr/attendance", map[string]any{"employeeId": hourly, "workDate": day, "minutesWorked": 300, "entryType": "manual"}, a.doctor).Code; code != http.StatusCreated {
			t.Fatalf("attendance %s: %d", day, code)
		}
	}

	run := a.request(http.MethodPost, "/api/v1/hr/payroll-runs", map[string]any{"periodStart": "2026-06-01", "periodEnd": "2026-06-30"}, a.doctor)
	if run.Code != http.StatusCreated {
		t.Fatalf("payroll run: %d %s", run.Code, run.Body.String())
	}
	runID := decodeResponse[map[string]any](t, run)["id"].(string)

	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/hr/payroll-runs/"+runID, nil, a.doctor))
	slips, _ := detail["payslips"].([]any)
	if len(slips) != 2 {
		t.Fatalf("the run produced %d payslips, want one per active employee", len(slips))
	}
	byEmployee := map[string]map[string]any{}
	for _, entry := range slips {
		slip, _ := entry.(map[string]any)
		byEmployee[slip["employeeId"].(string)] = slip
	}
	if byEmployee[salaried]["grossMinor"] != float64(4000000) {
		t.Fatalf("a salaried employee was paid %v, want their agreed salary", byEmployee[salaried]["grossMinor"])
	}
	if byEmployee[salaried]["deductionsMinor"] != float64(400000) || byEmployee[salaried]["netMinor"] != float64(3600000) {
		t.Fatalf("the 10%% deduction was not applied: %v", byEmployee[salaried])
	}
	// 600 minutes at 30000 per hour is 300000.
	if byEmployee[hourly]["grossMinor"] != float64(300000) || byEmployee[hourly]["minutesWorked"] != float64(600) {
		t.Fatalf("the hourly employee was not paid from their recorded hours: %v", byEmployee[hourly])
	}
	if detail["netMinor"] != float64(3900000) {
		t.Fatalf("the run total does not match its payslips: %v", detail["netMinor"])
	}
}

func TestTwoPayrollRunsCannotCoverTheSameDay(t *testing.T) {
	a := newTestApp(t)
	a.createEmployee(t, map[string]any{"firstName": "Only", "lastName": "Employee", "baseSalaryMinor": 100000})
	if code := a.request(http.MethodPost, "/api/v1/hr/payroll-runs", map[string]any{"periodStart": "2026-07-01", "periodEnd": "2026-07-31"}, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("first run: %d", code)
	}
	overlapping := a.request(http.MethodPost, "/api/v1/hr/payroll-runs", map[string]any{"periodStart": "2026-07-15", "periodEnd": "2026-08-15"}, a.doctor)
	if overlapping.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an overlapping payroll period = %d, want 422", overlapping.Code)
	}
}

func TestAPayrollRunIsApprovedBeforeItIsPaidAndThenFrozen(t *testing.T) {
	a := newTestApp(t)
	a.createEmployee(t, map[string]any{"firstName": "Paid", "lastName": "Once", "baseSalaryMinor": 100000})
	run := a.request(http.MethodPost, "/api/v1/hr/payroll-runs", map[string]any{"periodStart": "2026-08-01", "periodEnd": "2026-08-31"}, a.doctor)
	created := decodeResponse[map[string]any](t, run)
	id, version := created["id"].(string), int(created["version"].(float64))

	// Paying a run nobody approved is refused.
	if early := a.request(http.MethodPatch, "/api/v1/hr/payroll-runs/"+id+"/status", map[string]any{"status": "paid", "version": version}, a.doctor); early.Code != http.StatusConflict {
		t.Fatalf("paying an unapproved run = %d, want 409", early.Code)
	}
	if code := a.request(http.MethodPatch, "/api/v1/hr/payroll-runs/"+id+"/status", map[string]any{"status": "approved", "version": version}, a.doctor).Code; code != http.StatusOK {
		t.Fatalf("approve: %d", code)
	}
	if code := a.request(http.MethodPatch, "/api/v1/hr/payroll-runs/"+id+"/status", map[string]any{"status": "paid", "version": version + 1}, a.doctor).Code; code != http.StatusOK {
		t.Fatalf("pay: %d", code)
	}
	// A paid run is finished; it cannot be cancelled out from under the payslips.
	if late := a.request(http.MethodPatch, "/api/v1/hr/payroll-runs/"+id+"/status", map[string]any{"status": "cancelled", "version": version + 2}, a.doctor); late.Code != http.StatusConflict {
		t.Fatalf("cancelling a paid run = %d, want 409", late.Code)
	}
}

func TestAContractIsFilledFromTheEmployeesOwnRecord(t *testing.T) {
	a := newTestApp(t)
	positionID := a.createPosition(t, "Receptionist", 2000000)
	id := a.createEmployee(t, map[string]any{"firstName": "Marie", "lastName": "Louis", "positionId": positionID, "startedOn": "2026-02-01", "baseSalaryMinor": 2000000, "address": "12 Rue Capois"})
	templates := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/hr/document-templates", nil, a.doctor))
	template, _ := templates["items"].([]any)[0].(map[string]any)

	generated := a.request(http.MethodPost, "/api/v1/hr/documents", map[string]any{"employeeId": id, "templateId": template["id"]}, a.doctor)
	if generated.Code != http.StatusCreated {
		t.Fatalf("generate contract: %d %s", generated.Code, generated.Body.String())
	}
	body := decodeResponse[map[string]any](t, generated)["body"].(string)
	for _, expected := range []string{"Marie Louis", "Receptionist", "2026-02-01", "12 Rue Capois"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("the contract does not mention %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "{{") {
		t.Fatalf("the contract still has unfilled placeholders:\n%s", body)
	}
}

func TestOnlyADoctorReachesStaffRecords(t *testing.T) {
	a := newTestApp(t)
	for _, path := range []string{"/api/v1/hr/employees", "/api/v1/hr/payroll-runs", "/api/v1/hr/attendance", "/api/v1/hr/documents"} {
		if response := a.request(http.MethodGet, path, nil, a.nurse); response.Code != http.StatusForbidden {
			t.Fatalf("nurse reading %s = %d, want 403", path, response.Code)
		}
	}
}
