package server

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestInventoryItemFullCRUD(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{
		"sku": "FR-CRUD-1", "category": "frame", "name": "Test Frame", "costMinor": 50000, "salePriceMinor": 150000, "currency": "HTG",
		"quantity": 10, "trackStock": true, "unit": "unit", "batchNumber": "B-1", "notes": "Shelf 3",
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)

	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory/"+id, nil, a.nurse))
	if detail["name"] != "Test Frame" || detail["batchNumber"] != "B-1" || detail["quantity"] != float64(10) || detail["archived"] != false {
		t.Fatalf("unexpected detail: %v", detail)
	}

	updated := a.request(http.MethodPut, "/api/v1/inventory/"+id, map[string]any{
		"sku": "FR-CRUD-1", "category": "frame", "name": "Renamed Frame", "costMinor": 55000, "salePriceMinor": 160000, "currency": "HTG",
		"trackStock": true, "unit": "unit", "notes": "Shelf 4", "version": 1,
	}, a.doctor)
	if updated.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	detail = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory/"+id, nil, a.doctor))
	if detail["name"] != "Renamed Frame" || detail["notes"] != "Shelf 4" || detail["quantity"] != float64(10) {
		t.Fatalf("update did not persist (or touched quantity): %v", detail)
	}

	forbiddenArchive := a.request(http.MethodDelete, "/api/v1/inventory/"+id, map[string]any{"version": 2}, a.nurse)
	if forbiddenArchive.Code != http.StatusForbidden {
		t.Fatalf("nurse archive status = %d, want 403", forbiddenArchive.Code)
	}
	archived := a.request(http.MethodDelete, "/api/v1/inventory/"+id, map[string]any{"version": 2}, a.doctor)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", archived.Code, archived.Body.String())
	}
	detail = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory/"+id, nil, a.doctor))
	if detail["archived"] != true {
		t.Fatalf("expected archived=true: %v", detail)
	}
	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory?q=FR-CRUD-1", nil, a.doctor))
	if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("an archived item should not appear in the default list: %v", items)
	}

	reactivated := a.request(http.MethodPost, "/api/v1/inventory/"+id+"/reactivate", map[string]any{"version": 3}, a.doctor)
	if reactivated.Code != http.StatusOK {
		t.Fatalf("reactivate: %d %s", reactivated.Code, reactivated.Body.String())
	}

	// This item has stock movements (its opening stock), so a permanent
	// delete must be refused in favor of archiving.
	blockedDelete := a.request(http.MethodPost, "/api/v1/inventory/"+id+"/permanent-delete?version=4", nil, a.doctor)
	if blockedDelete.Code != http.StatusUnprocessableEntity {
		t.Fatalf("delete with movement history = %d, want 422", blockedDelete.Code)
	}

	// A fresh item with zero quantity has no movement history and can be
	// permanently deleted.
	freshID := a.createInventoryItem(map[string]any{"sku": "FR-CRUD-2", "category": "frame", "name": "Deletable Frame", "salePriceMinor": 100000, "currency": "HTG", "trackStock": true, "quantity": 0})
	deleted := a.request(http.MethodPost, "/api/v1/inventory/"+freshID+"/permanent-delete?version=1", nil, a.doctor)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete unused item: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestReceivingStockCreatesAnAttributableMovement(t *testing.T) {
	a := newTestApp(t)
	id := a.createInventoryItem(map[string]any{"sku": "LEN-1", "category": "ophthalmic_lens", "name": "Progressive lens", "salePriceMinor": 200000, "currency": "HTG", "trackStock": true, "quantity": 50})
	received := a.request(http.MethodPost, "/api/v1/inventory/"+id+"/movements", map[string]any{"type": "purchase", "quantity": 30, "reason": "Supplier delivery", "version": 1}, a.doctor)
	if received.Code != http.StatusCreated {
		t.Fatalf("receive stock: %d %s", received.Code, received.Body.String())
	}
	body := decodeResponse[map[string]any](t, received)
	if body["resultingQuantity"] != float64(80) {
		t.Fatalf("50 + 30 should be 80: %v", body)
	}
	movements := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory/"+id+"/movements", nil, a.doctor))
	items, _ := movements["items"].([]any)
	if len(items) != 2 { // opening stock + this receipt
		t.Fatalf("expected 2 movements (opening + receipt), got %d: %v", len(items), items)
	}
	latest, _ := items[0].(map[string]any)
	if latest["type"] != "purchase" || latest["previousQuantity"] != float64(50) || latest["quantityChange"] != float64(30) || latest["resultingQuantity"] != float64(80) {
		t.Fatalf("receipt movement not recorded correctly: %v", latest)
	}
}

func TestDamagedExpiredAndLostAlwaysReduceStock(t *testing.T) {
	a := newTestApp(t)
	id := a.createInventoryItem(map[string]any{"sku": "ACC-1", "category": "accessory", "name": "Lens cloth", "salePriceMinor": 5000, "currency": "HTG", "trackStock": true, "quantity": 80})
	version := 1
	for _, movementType := range []string{"damage", "expired", "loss"} {
		// Quantity is sent positive, as a person would type "2 damaged" — the
		// server must still subtract, never add.
		response := a.request(http.MethodPost, "/api/v1/inventory/"+id+"/movements", map[string]any{"type": movementType, "quantity": 2, "reason": movementType, "version": version}, a.doctor)
		if response.Code != http.StatusCreated {
			t.Fatalf("%s movement: %d %s", movementType, response.Code, response.Body.String())
		}
		version++
	}
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory/"+id, nil, a.doctor))
	// 80 - 2 (damage) - 2 (expired) - 2 (loss) = 74
	if detail["quantity"] != float64(74) {
		t.Fatalf("expected 74 remaining, got %v", detail["quantity"])
	}
}

// TestConcurrentStockMovementsCannotCorruptQuantity is the acceptance test
// for section 38 of the redesign brief: two near-simultaneous mutations of
// the same item must not both succeed against a stale quantity. Optimistic
// locking (the item's version) must let exactly one of two racing requests
// win; the other must see CONCURRENT_MODIFICATION rather than silently
// applying its change on top of stale data.
func TestConcurrentStockMovementsCannotCorruptQuantity(t *testing.T) {
	a := newTestApp(t)
	id := a.createInventoryItem(map[string]any{"sku": "FR-RACE-1", "category": "frame", "name": "Racey Frame", "salePriceMinor": 100000, "currency": "HTG", "trackStock": true, "quantity": 5})

	var wg sync.WaitGroup
	var successes int32
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			// Both requests believe the item is still at version 1 — exactly
			// what two staff members who both loaded the page before either
			// saved would send.
			response := a.request(http.MethodPost, "/api/v1/inventory/"+id+"/movements", map[string]any{"type": "sale", "quantity": 3, "reason": "POS sale", "version": 1}, a.doctor)
			codes[index] = response.Code
			if response.Code == http.StatusCreated {
				atomic.AddInt32(&successes, 1)
			}
		}(i)
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly one of two racing stock movements to succeed, got %d (codes: %v)", successes, codes)
	}
	hasConflict := codes[0] == http.StatusConflict || codes[1] == http.StatusConflict
	if !hasConflict {
		t.Fatalf("the losing request should see 409 CONCURRENT_MODIFICATION, got codes %v", codes)
	}
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/inventory/"+id, nil, a.doctor))
	// Exactly one sale of 3 applied: 5 - 3 = 2, never 5-3-3=-1 and never
	// left at 5 as if nothing happened.
	if detail["quantity"] != float64(2) {
		t.Fatalf("expected exactly one deduction to apply (5-3=2), got %v", detail["quantity"])
	}
}

func TestExpiredMovementTypeIsAccepted(t *testing.T) {
	a := newTestApp(t)
	id := a.createInventoryItem(map[string]any{"sku": "CL-EXP-1", "category": "contact_lens", "name": "Monthly contacts", "salePriceMinor": 300000, "currency": "HTG", "trackStock": true, "quantity": 20})
	response := a.request(http.MethodPost, "/api/v1/inventory/"+id+"/movements", map[string]any{"type": "expired", "quantity": 5, "reason": "Past expiration date on the shelf", "version": 1}, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("expired movement: %d %s", response.Code, response.Body.String())
	}
}

func TestLowStockReportListsOnlyItemsAtOrBelowReorderLevel(t *testing.T) {
	a := newTestApp(t)
	a.createInventoryItem(map[string]any{"sku": "LOW-1", "category": "accessory", "name": "Almost out", "salePriceMinor": 1000, "currency": "HTG", "trackStock": true, "quantity": 2, "reorderLevel": 5})
	a.createInventoryItem(map[string]any{"sku": "OK-1", "category": "accessory", "name": "Well stocked", "salePriceMinor": 1000, "currency": "HTG", "trackStock": true, "quantity": 50, "reorderLevel": 5})
	report := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/reports/low-stock", nil, a.doctor))
	rows, _ := report["items"].([]any)
	found, wellStockedLeaked := false, false
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		label, _ := row["label"].(string)
		if strings.Contains(label, "LOW-1") {
			found = true
		}
		if strings.Contains(label, "OK-1") {
			wellStockedLeaked = true
		}
	}
	if !found {
		t.Fatalf("expected the low-stock item to appear in the report: %v", rows)
	}
	if wellStockedLeaked {
		t.Fatalf("a well-stocked item should not appear in the low-stock report: %v", rows)
	}
}
