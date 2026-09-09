package server

import (
	"net/http"
	"strings"
	"testing"
)

func (a *testApp) createLabOrder(t *testing.T, patientID, note string) string {
	t.Helper()
	response := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{"patientId": patientID, "lensType": "single_vision", "notes": note}, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("create lab order: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](t, response)["id"].(string)
}

func TestADayOfLabOrdersIsSentInOneAction(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Batch", "Lab")
	ids := []string{a.createLabOrder(t, patient.ID, "First"), a.createLabOrder(t, patient.ID, "Second"), a.createLabOrder(t, patient.ID, "Third")}

	sent := a.request(http.MethodPost, "/api/v1/lab-orders/bulk-status", map[string]any{"orderIds": ids, "status": "at_lab", "notes": "Collected by courier"}, a.doctor)
	if sent.Code != http.StatusOK {
		t.Fatalf("bulk send: %d %s", sent.Code, sent.Body.String())
	}
	if decodeResponse[map[string]any](t, sent)["updated"] != float64(3) {
		t.Fatalf("bulk send moved %v orders", decodeResponse[map[string]any](t, sent)["updated"])
	}
	for _, id := range ids {
		var status string
		_ = a.server.db.QueryRow("SELECT status FROM lab_orders WHERE id=?", id).Scan(&status)
		if status != "at_lab" {
			t.Fatalf("order %s is %s", id, status)
		}
		var history int
		_ = a.server.db.QueryRow("SELECT COUNT(*) FROM lab_status_history WHERE lab_order_id=? AND to_status='at_lab'", id).Scan(&history)
		if history != 1 {
			t.Fatalf("order %s recorded %d history entries for the batch move", id, history)
		}
	}
}

func TestAFailedBatchChangesNothing(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Rolled", "Back")
	good := a.createLabOrder(t, patient.ID, "Will not move")
	response := a.request(http.MethodPost, "/api/v1/lab-orders/bulk-status", map[string]any{"orderIds": []string{good, "does-not-exist"}, "status": "at_lab"}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("batch with an unknown order = %d, want 422: %s", response.Code, response.Body.String())
	}
	var status string
	_ = a.server.db.QueryRow("SELECT status FROM lab_orders WHERE id=?", good).Scan(&status)
	if status != "draft" {
		t.Fatalf("a rolled-back batch left an order at %s", status)
	}
}

func TestABatchRefusesAnOrderThatIsAlreadyFinished(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Closed", "Order")
	id := a.createLabOrder(t, patient.ID, "Cancelled already")
	if code := a.request(http.MethodPatch, "/api/v1/lab-orders/"+id+"/status", map[string]any{"status": "cancelled", "version": 1}, a.doctor).Code; code != http.StatusOK {
		t.Fatalf("cancel: %d", code)
	}
	response := a.request(http.MethodPost, "/api/v1/lab-orders/bulk-status", map[string]any{"orderIds": []string{id}, "status": "at_lab"}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("batching a cancelled order = %d, want 422", response.Code)
	}
}

func TestABatchCannotDeliverOrPassQualityControl(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Per", "Order")
	id := a.createLabOrder(t, patient.ID, "Needs QC")
	for _, status := range []string{"ready", "delivered", "quality_control"} {
		response := a.request(http.MethodPost, "/api/v1/lab-orders/bulk-status", map[string]any{"orderIds": []string{id}, "status": status}, a.doctor)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("batch to %s = %d, want 422", status, response.Code)
		}
	}
}

func TestOneRequisitionCoversTheWholeBatch(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Requisition", "Batch")
	ids := []string{a.createLabOrder(t, patient.ID, "Left lens"), a.createLabOrder(t, patient.ID, "Right lens")}

	requisition := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/lab-orders/requisition?orderIds="+strings.Join(ids, ","), nil, a.doctor))
	orders, _ := requisition["orders"].([]any)
	if len(orders) != 2 {
		t.Fatalf("the requisition covers %d orders, want 2", len(orders))
	}
	first, _ := orders[0].(map[string]any)
	if first["patientName"] != "Requisition Batch" || first["orderNumber"] == "" {
		t.Fatalf("the requisition line is missing its patient or number: %v", first)
	}
	if requisition["clinic"] == nil {
		t.Fatal("the requisition carries no clinic identity for the lab to read")
	}
	if response := a.request(http.MethodGet, "/api/v1/lab-orders/requisition?orderIds=", nil, a.doctor); response.Code != http.StatusBadRequest {
		t.Fatalf("an empty requisition request = %d, want 400", response.Code)
	}
}
