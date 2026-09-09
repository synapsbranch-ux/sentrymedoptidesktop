package server

import (
	"net/http"
	"testing"
)

func (a *testApp) createInventoryItem(body map[string]any) string {
	a.t.Helper()
	response := a.request(http.MethodPost, "/api/v1/inventory", body, a.doctor)
	if response.Code != http.StatusCreated {
		a.t.Fatalf("create inventory item: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](a.t, response)["id"].(string)
}

func (a *testApp) cashMethodID(t *testing.T) string {
	t.Helper()
	methods := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/payment-methods", nil, a.doctor))
	for _, item := range methods["items"].([]any) {
		method, _ := item.(map[string]any)
		if method["name"] == "Cash" {
			return method["id"].(string)
		}
	}
	t.Fatalf("no cash payment method: %v", methods)
	return ""
}

func TestAppointmentTypesAreSellableServicesWithAPrice(t *testing.T) {
	a := newTestApp(t)
	a.createInventoryItem(map[string]any{"sku": "SVC-EXAM", "category": "service", "name": "Comprehensive eye examination", "salePriceMinor": 250000, "currency": "HTG", "trackStock": false, "durationMinutes": 45, "bookable": true})
	a.createInventoryItem(map[string]any{"sku": "SVC-BACK", "category": "service", "name": "Back-office fitting fee", "salePriceMinor": 50000, "currency": "HTG", "trackStock": false, "bookable": false})

	all := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/service-types", nil, a.nurse))["items"].([]any)
	if len(all) < 2 {
		t.Fatalf("the till sees %d services, want the exam and the fitting fee", len(all))
	}
	bookable := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/service-types?bookable=true", nil, a.nurse))["items"].([]any)
	for _, item := range bookable {
		service, _ := item.(map[string]any)
		if service["bookable"] != true {
			t.Fatalf("a non-bookable service was offered as an appointment type: %v", service)
		}
	}
}

func TestOnlyAServiceCanBeOfferedAsAnAppointmentType(t *testing.T) {
	a := newTestApp(t)
	response := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "FR-1", "category": "frame", "name": "Frame", "salePriceMinor": 100, "trackStock": true, "bookable": true}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a bookable frame = %d, want 422", response.Code)
	}
}

func TestBookingTakesItsNameAndDurationFromTheService(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Booked", "Service")
	serviceID := a.createInventoryItem(map[string]any{"sku": "SVC-CL", "category": "service", "name": "Contact lens fitting", "salePriceMinor": 180000, "currency": "HTG", "trackStock": false, "durationMinutes": 60, "bookable": true})

	created := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": patient.ID, "startsAt": "2026-10-01T14:00:00Z", "serviceItemId": serviceID}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("book from a service: %d %s", created.Code, created.Body.String())
	}
	body := decodeResponse[map[string]any](t, created)
	if body["type"] != "Contact lens fitting" || body["durationMinutes"] != float64(60) {
		t.Fatalf("the booking did not take the service's name and duration: %v", body)
	}
	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/appointments?date=2026-10-01", nil, a.doctor))
	entry, _ := list["items"].([]any)[0].(map[string]any)
	if entry["serviceItemId"] != serviceID || entry["servicePriceMinor"] != float64(180000) {
		t.Fatalf("the appointment does not carry its service price: %v", entry)
	}
}

func TestBookingRefusesAServiceThatIsNotBookable(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Not", "Bookable")
	serviceID := a.createInventoryItem(map[string]any{"sku": "SVC-INT", "category": "service", "name": "Internal handling fee", "salePriceMinor": 1000, "trackStock": false, "bookable": false})
	response := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": patient.ID, "startsAt": "2026-10-02T14:00:00Z", "serviceItemId": serviceID}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("booking a non-bookable service = %d, want 422", response.Code)
	}
}

func TestAnAppointmentNeedsNoPaymentButASaleCanBeLinkedToOne(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Pays", "Later")
	serviceID := a.createInventoryItem(map[string]any{"sku": "SVC-EX2", "category": "service", "name": "Eye examination", "salePriceMinor": 200000, "currency": "HTG", "trackStock": false, "durationMinutes": 30, "bookable": true})
	booked := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": patient.ID, "startsAt": "2026-10-03T09:00:00Z", "serviceItemId": serviceID}, a.doctor)
	if booked.Code != http.StatusCreated {
		t.Fatalf("booking without any payment = %d %s", booked.Code, booked.Body.String())
	}
	appointmentID := decodeResponse[map[string]any](t, booked)["id"].(string)

	// The visit happened; the money arrives afterwards and names what it paid for.
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice": map[string]any{"patientId": patient.ID, "appointmentId": appointmentID, "currency": "HTG", "items": []map[string]any{
			{"inventoryItemId": serviceID, "description": "Eye examination", "quantity": 1, "unitPriceMinor": 200000},
		}},
		"payments": []map[string]any{{"paymentMethodId": a.cashMethodID(t), "amountMinor": 200000, "currency": "HTG"}},
	}, a.doctor)
	if checkout.Code != http.StatusCreated {
		t.Fatalf("checkout linked to an appointment: %d %s", checkout.Code, checkout.Body.String())
	}
	sales := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/sales?patientId="+patient.ID, nil, a.doctor))
	sale, _ := sales["items"].([]any)[0].(map[string]any)
	if sale["appointmentId"] != appointmentID {
		t.Fatalf("the sale is not linked to the appointment it paid for: %v", sale)
	}
}

func TestASaleCannotBeFiledAgainstSomebodyElsesAppointment(t *testing.T) {
	a := newTestApp(t)
	owner := a.createPatient(a.doctor, "Appointment", "Owner")
	other := a.createPatient(a.doctor, "Different", "Customer")
	booked := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": owner.ID, "startsAt": "2026-10-04T09:00:00Z", "type": "eye_exam"}, a.doctor)
	appointmentID := decodeResponse[map[string]any](t, booked)["id"].(string)
	itemID := a.createInventoryItem(map[string]any{"sku": "SVC-EX3", "category": "service", "name": "Exam", "salePriceMinor": 1000, "currency": "HTG", "trackStock": false})

	response := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice": map[string]any{"patientId": other.ID, "appointmentId": appointmentID, "currency": "HTG", "items": []map[string]any{
			{"inventoryItemId": itemID, "description": "Exam", "quantity": 1, "unitPriceMinor": 1000},
		}},
	}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-patient appointment link = %d, want 422: %s", response.Code, response.Body.String())
	}
}

func TestOneSaleCanBeSettledWithSeveralTenders(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Split", "Payment")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-SPLIT", "category": "frame", "name": "Frame", "salePriceMinor": 100000, "currency": "HTG", "quantity": 5, "trackStock": true})
	methods := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/payment-methods", nil, a.doctor))["items"].([]any)
	if len(methods) < 2 {
		t.Skip("this clinic has only one payment method configured")
	}
	first, _ := methods[0].(map[string]any)
	second, _ := methods[1].(map[string]any)

	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice": map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{
			{"inventoryItemId": itemID, "description": "Frame", "quantity": 1, "unitPriceMinor": 100000},
		}},
		"payments": []map[string]any{
			{"paymentMethodId": first["id"], "amountMinor": 60000, "currency": "HTG"},
			{"paymentMethodId": second["id"], "amountMinor": 40000, "currency": "HTG"},
		},
	}, a.doctor)
	if checkout.Code != http.StatusCreated {
		t.Fatalf("split payment checkout: %d %s", checkout.Code, checkout.Body.String())
	}
	body := decodeResponse[map[string]any](t, checkout)
	if body["balanceMinor"] != float64(0) {
		t.Fatalf("a fully split payment left a balance of %v", body["balanceMinor"])
	}
	if payments, _ := body["payments"].([]any); len(payments) != 2 {
		t.Fatalf("the sale recorded %d payments, want one per tender", len(payments))
	}
}

func TestAPrescriptionBecomesAPricedCart(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Prescribed", "Cart")
	a.createInventoryItem(map[string]any{"sku": "FR-CART", "category": "frame", "name": "Classic frame", "salePriceMinor": 150000, "currency": "HTG", "quantity": 3, "trackStock": true})
	a.createInventoryItem(map[string]any{"sku": "LN-CART", "category": "ophthalmic_lens", "name": "Single vision lens", "salePriceMinor": 90000, "currency": "HTG", "quantity": 0, "trackStock": true})
	issued := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{"patientId": patient.ID, "type": "spectacle", "od": map[string]string{"sphere": "-1.50"}}, a.doctor)
	if issued.Code != http.StatusCreated {
		t.Fatalf("issue prescription: %d %s", issued.Code, issued.Body.String())
	}

	cart := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/prescription-cart?patientId="+patient.ID, nil, a.nurse))
	prescription, _ := cart["prescription"].(map[string]any)
	if prescription["patientId"] != patient.ID {
		t.Fatalf("the cart is for the wrong patient: %v", prescription)
	}
	lines, _ := cart["lines"].([]any)
	if len(lines) != 2 {
		t.Fatalf("a spectacle prescription proposed %d lines, want a frame and a lens", len(lines))
	}
	frame, _ := lines[0].(map[string]any)
	options, _ := frame["options"].([]any)
	if len(options) == 0 {
		t.Fatal("the frame line offered nothing from the catalogue")
	}
	// Availability is read from inventory, so the counter is not offered
	// something the clinic cannot hand over today without being told.
	firstOption, _ := options[0].(map[string]any)
	if firstOption["inStock"] != true || firstOption["salePriceMinor"] != float64(150000) {
		t.Fatalf("the frame option is not priced and stocked from inventory: %v", firstOption)
	}
	lens, _ := lines[1].(map[string]any)
	lensOptions, _ := lens["options"].([]any)
	outOfStock, _ := lensOptions[0].(map[string]any)
	if outOfStock["inStock"] != false {
		t.Fatalf("a lens with no stock was reported as available: %v", outOfStock)
	}
}

func TestPrescriptionCartReportsAnExpiredPrescription(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Expired", "Prescription")
	issued := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{"patientId": patient.ID, "type": "spectacle", "od": map[string]string{"sphere": "-1.00"}, "expiresAt": "2020-01-01"}, a.doctor)
	if issued.Code != http.StatusCreated {
		t.Fatalf("issue prescription: %d %s", issued.Code, issued.Body.String())
	}
	cart := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/prescription-cart?patientId="+patient.ID, nil, a.nurse))
	prescription, _ := cart["prescription"].(map[string]any)
	if prescription["expired"] != true {
		t.Fatalf("an expired prescription was offered without warning: %v", prescription)
	}
}

func TestASaleCanBeParkedAndResumedExactlyOnce(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Parked", "Sale")
	parked := a.request(http.MethodPost, "/api/v1/pos/parked", map[string]any{
		"label": "Mme Joseph — waiting on husband", "patientId": patient.ID, "currency": "HTG", "totalMinor": 150000,
		"cart": []map[string]any{{"inventoryItemId": "x", "description": "Classic frame", "quantity": 1, "unitPriceMinor": 150000}},
	}, a.nurse)
	if parked.Code != http.StatusCreated {
		t.Fatalf("park a sale: %d %s", parked.Code, parked.Body.String())
	}
	parkedID := decodeResponse[map[string]any](t, parked)["id"].(string)

	open := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/parked", nil, a.doctor))
	if items, _ := open["items"].([]any); len(items) != 1 {
		t.Fatalf("the till shows %d parked sales, want 1", len(items))
	}

	resumed := a.request(http.MethodPost, "/api/v1/pos/parked/"+parkedID+"/resume", map[string]any{}, a.doctor)
	if resumed.Code != http.StatusOK {
		t.Fatalf("resume a parked sale: %d %s", resumed.Code, resumed.Body.String())
	}
	body := decodeResponse[map[string]any](t, resumed)
	// The held line named an item that does not exist, so it is reported rather
	// than restored — a parked cart is a list of choices, not of prices.
	if unavailable, _ := body["unavailable"].([]any); len(unavailable) != 1 {
		t.Fatalf("a line that left the catalogue was not reported: %v", body)
	}

	// A second till cannot resume the same hold and ring the sale up twice.
	again := a.request(http.MethodPost, "/api/v1/pos/parked/"+parkedID+"/resume", map[string]any{}, a.nurse)
	if again.Code != http.StatusNotFound {
		t.Fatalf("resuming a used hold = %d, want 404", again.Code)
	}
	after := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/parked", nil, a.doctor))
	if items, _ := after["items"].([]any); len(items) != 0 {
		t.Fatalf("a resumed sale is still listed as parked: %v", items)
	}
}

func TestCashRegisterReportBreaksTakingsDownByMethod(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Register", "Report")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-REG", "category": "frame", "name": "Frame", "salePriceMinor": 50000, "currency": "HTG", "quantity": 10, "trackStock": true})
	opened := a.request(http.MethodPost, "/api/v1/cash-register/open", map[string]any{"currency": "HTG", "openingFloatMinor": 20000}, a.doctor)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open register: %d %s", opened.Code, opened.Body.String())
	}
	sessionID := decodeResponse[map[string]any](t, opened)["id"].(string)
	cash := a.cashMethodID(t)

	for index := 0; index < 2; index++ {
		checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
			"invoice":  map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 1, "unitPriceMinor": 50000}}},
			"payments": []map[string]any{{"paymentMethodId": cash, "registerSessionId": sessionID, "amountMinor": 50000, "currency": "HTG"}},
		}, a.doctor)
		if checkout.Code != http.StatusCreated {
			t.Fatalf("sale %d: %d %s", index, checkout.Code, checkout.Body.String())
		}
	}

	report := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/cash-register/"+sessionID+"/report", nil, a.doctor))
	if report["open"] != true || report["takingsMinor"] != float64(100000) {
		t.Fatalf("register report: %v", report)
	}
	if report["expectedCashMinor"] != float64(120000) {
		t.Fatalf("expected drawer contents = %v, want the float plus the cash taken", report["expectedCashMinor"])
	}
	if report["salesCount"] != float64(2) {
		t.Fatalf("register report counted %v sales", report["salesCount"])
	}
	byMethod, _ := report["byMethod"].([]any)
	if len(byMethod) != 1 {
		t.Fatalf("takings were broken into %d methods, want one", len(byMethod))
	}
	if line, _ := byMethod[0].(map[string]any); line["method"] != "Cash" || line["netMinor"] != float64(100000) {
		t.Fatalf("the cash line reads %v", byMethod[0])
	}
}

func TestSalesHistoryFiltersByCashierAndMethod(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Sales", "History")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-HIST", "category": "frame", "name": "Frame", "salePriceMinor": 10000, "currency": "HTG", "quantity": 10, "trackStock": true})
	cash := a.cashMethodID(t)
	sale := map[string]any{
		"invoice":  map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Frame", "quantity": 1, "unitPriceMinor": 10000}}},
		"payments": []map[string]any{{"paymentMethodId": cash, "amountMinor": 10000, "currency": "HTG"}},
	}
	if code := a.request(http.MethodPost, "/api/v1/pos/checkout", sale, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("doctor sale: %d", code)
	}
	if code := a.request(http.MethodPost, "/api/v1/pos/checkout", sale, a.nurse).Code; code != http.StatusCreated {
		t.Fatalf("nurse sale: %d", code)
	}

	all := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/sales", nil, a.doctor))
	if total, _ := all["total"].(float64); total != 2 {
		t.Fatalf("the sales history holds %v sales, want 2", all["total"])
	}
	byMethod := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/sales?paymentMethodId="+cash, nil, a.doctor))
	if total, _ := byMethod["total"].(float64); total != 2 {
		t.Fatalf("filtering by cash returned %v sales", byMethod["total"])
	}
	unknownMethod := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/pos/sales?paymentMethodId=does-not-exist", nil, a.doctor))
	if total, _ := unknownMethod["total"].(float64); total != 0 {
		t.Fatalf("filtering by an unused method returned %v sales", unknownMethod["total"])
	}
	first, _ := all["items"].([]any)[0].(map[string]any)
	if methods, _ := first["paymentMethods"].([]any); len(methods) != 1 || methods[0] != "Cash" {
		t.Fatalf("the sale does not report how it was paid: %v", first["paymentMethods"])
	}
}

func TestAParkedSaleIsResumedAtTodaysPriceAndStock(t *testing.T) {
	a := newTestApp(t)
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-REPRICE", "category": "frame", "name": "Repriced frame", "salePriceMinor": 100000, "currency": "HTG", "quantity": 4, "trackStock": true})
	parked := a.request(http.MethodPost, "/api/v1/pos/parked", map[string]any{
		"label": "Held over the weekend", "currency": "HTG", "totalMinor": 100000,
		"cart": []map[string]any{{"inventoryItemId": itemID, "description": "Repriced frame", "quantity": 2, "discountMinor": 5000}},
	}, a.doctor)
	parkedID := decodeResponse[map[string]any](t, parked)["id"].(string)

	// The price rises and stock falls while the sale is held.
	if _, err := a.server.db.Exec("UPDATE inventory_items SET sale_price_minor=130000, quantity=1 WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}

	resumed := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/pos/parked/"+parkedID+"/resume", map[string]any{}, a.doctor))
	cart, _ := resumed["cart"].([]any)
	if len(cart) != 1 {
		t.Fatalf("the resumed cart holds %d lines", len(cart))
	}
	line, _ := cart[0].(map[string]any)
	if line["salePriceMinor"] != float64(130000) {
		t.Fatalf("the sale resumed at the old price: %v", line["salePriceMinor"])
	}
	if line["cartQuantity"] != float64(2) || line["discountMinor"] != float64(5000) {
		t.Fatalf("the cashier's own quantity and discount were lost: %v", line)
	}
	// Two were wanted and one is left, so the cashier is told before ringing it up.
	if line["inStock"] != false {
		t.Fatalf("a line the clinic can no longer fill was reported as available: %v", line)
	}
}
