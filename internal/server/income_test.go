package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func todayDate() string { return time.Now().UTC().Format("2006-01-02") }

func (a *testApp) incomeFor(t *testing.T, query string) []map[string]any {
	t.Helper()
	response := a.request(http.MethodGet, "/api/v1/income?"+query, nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("income list: %d %s", response.Code, response.Body.String())
	}
	raw, _ := decodeResponse[map[string]any](t, response)["items"].([]any)
	items := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		row, _ := entry.(map[string]any)
		items = append(items, row)
	}
	return items
}

func TestATillPaymentBecomesIncomeWithoutAnyoneRecordingIt(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Income", "FromSale")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-INC", "category": "frame", "name": "Frame", "salePriceMinor": 80000, "currency": "HTG", "quantity": 5, "trackStock": true})
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice":  map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 1, "unitPriceMinor": 80000}}},
		"payments": []map[string]any{{"paymentMethodId": a.cashMethodID(t), "amountMinor": 80000, "currency": "HTG"}},
	}, a.doctor)
	if checkout.Code != http.StatusCreated {
		t.Fatalf("checkout: %d %s", checkout.Code, checkout.Body.String())
	}

	entries := a.incomeFor(t, "from=2000-01-01&to=2100-01-01")
	if len(entries) != 1 {
		t.Fatalf("a paid sale produced %d income entries, want 1", len(entries))
	}
	if entries[0]["source"] != "pos_payment" || entries[0]["amountMinor"] != float64(80000) || entries[0]["automatic"] != true {
		t.Fatalf("the income entry does not describe the sale: %v", entries[0])
	}
	if entries[0]["patientId"] != patient.ID {
		t.Fatalf("the income entry lost the customer: %v", entries[0])
	}
}

func TestARefundReducesIncomeRatherThanHidingIt(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Refunded", "Income")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-REF", "category": "frame", "name": "Frame", "salePriceMinor": 90000, "currency": "HTG", "quantity": 5, "trackStock": true})
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice":  map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 1, "unitPriceMinor": 90000}}},
		"payments": []map[string]any{{"paymentMethodId": a.cashMethodID(t), "amountMinor": 90000, "currency": "HTG"}},
	}, a.doctor)
	paymentID := decodeResponse[map[string]any](t, checkout)["paymentId"].(string)

	refund := a.request(http.MethodPost, "/api/v1/payments/"+paymentID+"/refunds", map[string]any{"amountMinor": 30000, "reason": "Frame returned"}, a.doctor)
	if refund.Code != http.StatusCreated {
		t.Fatalf("refund: %d %s", refund.Code, refund.Body.String())
	}

	entries := a.incomeFor(t, "from=2000-01-01&to=2100-01-01")
	var net float64
	sources := map[string]bool{}
	for _, entry := range entries {
		net += entry["amountMinor"].(float64)
		sources[entry["source"].(string)] = true
	}
	if !sources["refund"] {
		t.Fatalf("the refund left no trace in the income ledger: %v", entries)
	}
	if net != 60000 {
		t.Fatalf("net income after a partial refund = %v, want 60000", net)
	}
}

func TestIncomeCanBeRecordedForMoneyTheTillNeverSaw(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/income", map[string]any{
		"category": "Grants", "description": "Vision screening programme grant", "amountMinor": 500000, "currency": "HTG", "receivedOn": "2026-05-04", "payer": "Ministry of Health",
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("manual income: %d %s", created.Code, created.Body.String())
	}
	entries := a.incomeFor(t, "from=2026-05-01&to=2026-05-31")
	if len(entries) != 1 || entries[0]["source"] != "manual" || entries[0]["automatic"] != false {
		t.Fatalf("manual income was not recorded as such: %v", entries)
	}
	if entries[0]["payer"] != "Ministry of Health" {
		t.Fatalf("the payer was lost: %v", entries[0])
	}
	// Outside the period it is not counted.
	if outside := a.incomeFor(t, "from=2026-06-01&to=2026-06-30"); len(outside) != 0 {
		t.Fatalf("income leaked outside its period: %v", outside)
	}
}

func TestIncomeRejectsAnEmptyOrNegativeEntry(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []map[string]any{
		{"category": "", "description": "No category", "amountMinor": 100},
		{"category": "Grants", "description": "", "amountMinor": 100},
		{"category": "Grants", "description": "Negative", "amountMinor": -100},
		{"category": "Grants", "description": "Bad date", "amountMinor": 100, "receivedOn": "not-a-date"},
	} {
		if response := a.request(http.MethodPost, "/api/v1/income", body, a.doctor); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v = %d, want 422", body, response.Code)
		}
	}
}

func TestOnlyADoctorSeesTheIncomeLedger(t *testing.T) {
	a := newTestApp(t)
	if response := a.request(http.MethodGet, "/api/v1/income", nil, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse reading income = %d, want 403", response.Code)
	}
}

func TestProfitAndLossTracesVisitsThroughToMoney(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Traced", "Revenue")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-PL", "category": "frame", "name": "Frame", "costMinor": 30000, "salePriceMinor": 100000, "currency": "HTG", "quantity": 5, "trackStock": true})
	if code := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Exam"}, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("consultation: %d", code)
	}
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice":  map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 1, "unitPriceMinor": 100000}}},
		"payments": []map[string]any{{"paymentMethodId": a.cashMethodID(t), "amountMinor": 100000, "currency": "HTG"}},
	}, a.doctor)
	if checkout.Code != http.StatusCreated {
		t.Fatalf("checkout: %d %s", checkout.Code, checkout.Body.String())
	}
	if code := a.request(http.MethodPost, "/api/v1/expenses", map[string]any{"category": "Rent", "description": "Clinic rent", "amountMinor": 40000, "currency": "HTG", "expenseDate": todayDate()}, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("expense: %d", code)
	}

	report := decodeResponse[map[string]any](t, a.request(http.MethodGet, fmt.Sprintf("/api/v1/finance/profit-and-loss?from=%s&to=%s", todayDate(), todayDate()), nil, a.doctor))
	if report["consultations"] != float64(1) || report["sales"] != float64(1) {
		t.Fatalf("the report lost the visit or the sale: %v", report)
	}
	if report["incomeMinor"] != float64(100000) || report["expensesMinor"] != float64(40000) || report["netMinor"] != float64(60000) {
		t.Fatalf("profit and loss does not add up: %v", report)
	}
	if report["costOfSalesMinor"] != float64(30000) || report["grossMarginMinor"] != float64(70000) {
		t.Fatalf("cost of sales was not carried through: %v", report)
	}
}
