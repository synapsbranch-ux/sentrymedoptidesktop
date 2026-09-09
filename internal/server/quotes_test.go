package server

import (
	"net/http"
	"testing"
)

func (a *testApp) createQuote(t *testing.T, body map[string]any) (string, int) {
	t.Helper()
	response := a.request(http.MethodPost, "/api/v1/quotes", body, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("create quote: %d %s", response.Code, response.Body.String())
	}
	created := decodeResponse[map[string]any](t, response)
	return created["id"].(string), int(created["version"].(float64))
}

func TestAQuoteBecomesAnInvoiceOnlyOnce(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Quoted", "Customer")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-QUO", "category": "frame", "name": "Frame", "salePriceMinor": 120000, "currency": "HTG", "quantity": 4, "trackStock": true})
	quoteID, version := a.createQuote(t, map[string]any{
		"patientId": patient.ID, "currency": "HTG", "validUntil": "2030-01-01", "status": "sent",
		"items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 2, "unitPriceMinor": 120000, "discountMinor": 20000}},
	})

	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/quotes/"+quoteID, nil, a.doctor))
	if detail["totalMinor"] != float64(220000) {
		t.Fatalf("quote total = %v, want the line total after its discount", detail["totalMinor"])
	}

	converted := a.request(http.MethodPost, "/api/v1/quotes/"+quoteID+"/convert", map[string]any{"version": version}, a.doctor)
	if converted.Code != http.StatusCreated {
		t.Fatalf("convert quote: %d %s", converted.Code, converted.Body.String())
	}
	result := decodeResponse[map[string]any](t, converted)
	if result["totalMinor"] != float64(220000) {
		t.Fatalf("the invoice does not carry the quoted total: %v", result)
	}

	// A second conversion — a double click, or two people at once — bills nothing.
	again := a.request(http.MethodPost, "/api/v1/quotes/"+quoteID+"/convert", map[string]any{"version": version + 1}, a.doctor)
	if again.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second conversion = %d, want 422: %s", again.Code, again.Body.String())
	}
	var invoices int
	_ = a.server.db.QueryRow("SELECT COUNT(*) FROM invoices").Scan(&invoices)
	if invoices != 1 {
		t.Fatalf("converting twice produced %d invoices", invoices)
	}
}

func TestConvertingAQuoteDoesNotMoveStock(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Stock", "Untouched")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-STK", "category": "frame", "name": "Frame", "salePriceMinor": 50000, "currency": "HTG", "quantity": 3, "trackStock": true})
	quoteID, version := a.createQuote(t, map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 2, "unitPriceMinor": 50000}}})
	if code := a.request(http.MethodPost, "/api/v1/quotes/"+quoteID+"/convert", map[string]any{"version": version}, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("convert: %d", code)
	}
	var quantity int
	_ = a.server.db.QueryRow("SELECT quantity FROM inventory_items WHERE id=?", itemID).Scan(&quantity)
	if quantity != 3 {
		t.Fatalf("stock fell to %d when a quote was billed; goods leave at the till, not on an invoice", quantity)
	}
}

func TestADeclinedQuoteCannotBeBilled(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Declined", "Quote")
	quoteID, version := a.createQuote(t, map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"description": "Progressive lenses", "quantity": 1, "unitPriceMinor": 90000}}})
	if code := a.request(http.MethodPatch, "/api/v1/quotes/"+quoteID+"/status", map[string]any{"status": "declined", "version": version}, a.doctor).Code; code != http.StatusOK {
		t.Fatalf("decline: %d", code)
	}
	response := a.request(http.MethodPost, "/api/v1/quotes/"+quoteID+"/convert", map[string]any{"version": version + 1}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("billing a declined quote = %d, want 422", response.Code)
	}
}

func TestAQuoteCanBeForSomebodyWhoIsNotAPatientYet(t *testing.T) {
	a := newTestApp(t)
	quoteID, _ := a.createQuote(t, map[string]any{"customerName": "Walk-in enquiry", "currency": "HTG", "items": []map[string]any{{"description": "Frame and lenses", "quantity": 1, "unitPriceMinor": 250000}}})
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/quotes/"+quoteID, nil, a.nurse))
	if detail["customerName"] != "Walk-in enquiry" {
		t.Fatalf("the enquirer's name was lost: %v", detail)
	}
}

func TestAQuoteNeedsACustomerAndItems(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []map[string]any{
		{"currency": "HTG", "items": []map[string]any{{"description": "Frame", "quantity": 1, "unitPriceMinor": 1000}}},
		{"customerName": "Someone", "currency": "HTG", "items": []map[string]any{}},
	} {
		if response := a.request(http.MethodPost, "/api/v1/quotes", body, a.doctor); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v = %d, want 422", body, response.Code)
		}
	}
}

func TestAQuoteReportsItselfExpiredWithoutANightlyJob(t *testing.T) {
	a := newTestApp(t)
	quoteID, _ := a.createQuote(t, map[string]any{"customerName": "Old enquiry", "currency": "HTG", "validUntil": "2020-01-01", "items": []map[string]any{{"description": "Frame", "quantity": 1, "unitPriceMinor": 1000}}})
	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/quotes", nil, a.doctor))
	for _, entry := range list["items"].([]any) {
		quote, _ := entry.(map[string]any)
		if quote["id"] == quoteID && quote["expired"] != true {
			t.Fatalf("a quote past its validity date is not reported as expired: %v", quote)
		}
	}
}
