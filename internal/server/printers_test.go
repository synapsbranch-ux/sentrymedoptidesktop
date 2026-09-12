package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/thermal"
)

func TestParseBluetoothDevicesFindsPT280AndIgnoresInvalidRows(t *testing.T) {
	devices := parseBluetoothDevices([]byte("Device 10:22:33:90:6e:27 PT280_6E27\nController AA:BB:CC:DD:EE:FF host\nDevice invalid Broken\n"))
	if len(devices) != 1 || devices[0].Address != knownPT280Address || devices[0].Name != "PT280_6E27" {
		t.Fatalf("unexpected devices: %#v", devices)
	}
	if got := parseBluetoothDevices(nil); len(got) != 0 {
		t.Fatalf("absent printer returned %#v", got)
	}
}

func TestParseSDPChannelDoesNotAssumeChannelOne(t *testing.T) {
	if got := parseSDPChannel([]byte("Protocol Descriptor List:\n  Channel: 7\n")); got != 7 {
		t.Fatalf("channel=%d, want 7", got)
	}
	for _, invalid := range [][]byte{nil, []byte("Channel: 0"), []byte("Channel: 31")} {
		if got := parseSDPChannel(invalid); got != 0 {
			t.Fatalf("invalid SDP returned channel %d", got)
		}
	}
}

func TestPrinterConfigurationValidationAndPersistence(t *testing.T) {
	a := newTestApp(t)
	printer := knownPT280()
	if response := a.request(http.MethodPut, "/api/v1/printers/default", printer, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse save status=%d, want 403", response.Code)
	}
	if response := a.request(http.MethodPut, "/api/v1/printers/default", printer, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("doctor save status=%d: %s", response.Code, response.Body.String())
	}
	loaded, ok := a.server.loadPrinter(t.Context())
	if !ok || loaded.Address != knownPT280Address || loaded.Channel != 1 || !loaded.IsDefault {
		t.Fatalf("saved printer not preserved: %#v", loaded)
	}
	printer.Address = "not-an-address"
	if response := a.request(http.MethodPut, "/api/v1/printers/default", printer, a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid printer status=%d, want 422", response.Code)
	}
}

func TestPrinterErrorCodes(t *testing.T) {
	cases := map[string]string{
		"permission denied":            "PRINTER_PERMISSION_DENIED",
		"printer is offline":           "PRINTER_OFFLINE",
		"printer connection is busy":   "PRINTER_BUSY",
		"printer connection timed out": "PRINTER_TIMEOUT",
		"RFCOMM connection failed":     "PRINTER_CONNECTION_FAILED",
		"cannot open printer device":   "PRINTER_CONNECTION_FAILED",
		"printer write failed":         "PRINTER_WRITE_FAILED",
		"unknown":                      "PRINTER_UNAVAILABLE",
	}
	for message, want := range cases {
		if got := printerErrorCode(errors.New(message)); got != want {
			t.Fatalf("%q: got %s, want %s", message, got, want)
		}
	}
	// A transport that states its own code is taken at its word rather than
	// having its wording re-read, and the remedy reaches the caller intact.
	typed := printerFailure("PRINTER_BUSY", "Another receipt is still printing.", errors.New("resource busy"))
	if got := printerErrorCode(typed); got != "PRINTER_BUSY" {
		t.Fatalf("typed failure code=%s, want PRINTER_BUSY", got)
	}
	if typed.Error() != "Another receipt is still printing." || printerCause(typed) != "resource busy" {
		t.Fatalf("typed failure lost its message or cause: %q / %q", typed.Error(), printerCause(typed))
	}
	if printerCause(errors.New("plain")) != "" {
		t.Fatal("a plain error reported a cause it does not have")
	}
}

// One physical printer accepts one connection at a time, so a second job waits
// for the first instead of racing it into a Bluetooth refusal.
func TestPrintJobsAreSerialisedAcrossClients(t *testing.T) {
	a := newTestApp(t)
	if err := a.server.acquirePrinter(t.Context()); err != nil {
		t.Fatalf("first print job was refused the printer: %v", err)
	}
	abandoned, cancel := context.WithCancel(t.Context())
	cancel()
	err := a.server.acquirePrinter(abandoned)
	if err == nil {
		t.Fatal("two print jobs held the printer at once")
	}
	if got := printerErrorCode(err); got != "PRINTER_BUSY" {
		t.Fatalf("queued job code=%s, want PRINTER_BUSY", got)
	}
	<-a.server.printSlots
	if err = a.server.acquirePrinter(t.Context()); err != nil {
		t.Fatalf("printer stayed locked after the first job finished: %v", err)
	}
	<-a.server.printSlots
}

// The receipt handed to a customer at the till has to be the sale they just
// paid for. A clinic logo is a bitmap: slow on a 58 mm head and large enough to
// fill a portable printer's buffer, which leaves the transaction unprinted.
func TestSaleReceiptCarriesTheTransactionAndNoBitmap(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Receipt", "Customer")
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-RCPT", "category": "frame", "name": "Monture Aviator", "salePriceMinor": 125000, "currency": "HTG", "quantity": 4, "trackStock": true})
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice":  map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Monture Aviator", "quantity": 2, "unitPriceMinor": 125000}}},
		"payments": []map[string]any{{"paymentMethodId": a.cashMethodID(t), "amountMinor": 250000, "currency": "HTG"}},
	}, a.doctor)
	if checkout.Code != http.StatusCreated {
		t.Fatalf("checkout: %d %s", checkout.Code, checkout.Body.String())
	}
	invoiceID, _ := decodeResponse[map[string]any](t, checkout)["invoiceId"].(string)

	receipt, invoiceNumber, err := a.server.receiptForInvoice(t.Context(), invoiceID)
	if err != nil {
		t.Fatalf("could not build the receipt for the sale: %v", err)
	}
	printed := string(thermal.Render58mm(receipt, 32))
	for _, expected := range []string{invoiceNumber, "Monture Aviator", "2 x 1250.00 HTG", "2500.00 HTG", "PAID"} {
		if !strings.Contains(printed, expected) {
			t.Fatalf("the receipt does not show %q:\n%s", expected, printed)
		}
	}
	if strings.Contains(printed, "\x1dv0") {
		t.Fatalf("a sale receipt carried a bitmap:\n%q", printed)
	}
	// Text only: a receipt this size reaches the head in about a second.
	if len(printed) > 2000 {
		t.Fatalf("receipt is %d bytes, far past what its text needs", len(printed))
	}
}

// The test page is the one place a bitmap belongs: it is what proves the
// printer renders one at all.
func TestPrinterTestPageStillProvesBitmapSupport(t *testing.T) {
	a := newTestApp(t)
	raster := a.server.brandingRaster(t.Context(), 384)
	if len(raster) < 8 || !strings.HasPrefix(string(raster), "\x1dv0") {
		t.Fatalf("the test page lost its bitmap: %d bytes", len(raster))
	}
	page := string(thermal.RenderTestPage("Clinic", "PT280_6E27", "PT280UB", "fr", raster, 32))
	if !strings.Contains(page, "\x1dv0") || !strings.Contains(page, "TEST REUSSI") {
		t.Fatal("the test page no longer exercises both the bitmap and the text path")
	}
}
