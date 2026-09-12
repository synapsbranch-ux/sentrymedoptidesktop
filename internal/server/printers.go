package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/thermal"
)

const knownPT280Address = "10:22:33:90:6E:27"

type thermalPrinter struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Model              string `json:"model"`
	Transport          string `json:"transport"`
	Address            string `json:"address"`
	DevicePath         string `json:"devicePath"`
	Protocol           string `json:"protocol"`
	Status             string `json:"status"`
	Channel            int    `json:"channel"`
	PaperWidthMM       int    `json:"paperWidthMm"`
	PrintableWidthDots int    `json:"printableWidthDots"`
	CharactersPerLine  int    `json:"charactersPerLine"`
	IsDefault          bool   `json:"isDefault"`
	LastSeenAt         string `json:"lastSeenAt"`
}

var (
	bluetoothAddress = regexp.MustCompile(`^[0-9A-Fa-f]{2}(:[0-9A-Fa-f]{2}){5}$`)
	linuxDevicePath  = regexp.MustCompile(`^/dev/(rfcomm[0-9]+|tty[A-Za-z0-9._-]+|usb/lp[0-9]+)$`)
	windowsCOMPort   = regexp.MustCompile(`(?i)^COM([1-9][0-9]{0,2})$`)
	sdpChannel       = regexp.MustCompile(`(?m)^\s*Channel:\s*([0-9]+)\s*$`)
)

type bluetoothDevice struct{ Address, Name string }

func (s *Server) registerPrinterRoutes(r chi.Router) {
	r.Get("/printers", s.handlePrintersGet)
	r.Get("/printers/discover", s.handlePrintersDiscover)
	r.Get("/printers/jobs", s.handlePrintJobsList)
	r.With(s.requireDoctor).Put("/printers/default", s.handlePrinterSave)
	r.With(s.requireDoctor).Post("/printers/default/test", s.handlePrinterTest)
	r.Post("/printers/default/print-invoice/{id}", s.handlePrinterInvoice)
}

func knownPT280() thermalPrinter {
	return thermalPrinter{ID: "bluetooth:" + knownPT280Address, Name: "PT280_6E27", Model: "PT280UB", Transport: "bluetooth", Address: knownPT280Address, Protocol: "escpos", Status: "offline", Channel: 1, PaperWidthMM: 58, PrintableWidthDots: 384, CharactersPerLine: 32}
}

func normalizePrinter(p thermalPrinter) thermalPrinter {
	p.Name, p.Model, p.Address, p.DevicePath = strings.TrimSpace(p.Name), strings.TrimSpace(p.Model), strings.ToUpper(strings.TrimSpace(p.Address)), strings.TrimSpace(p.DevicePath)
	if p.Protocol == "" {
		p.Protocol = "escpos"
	}
	if p.Transport == "" {
		p.Transport = "bluetooth"
	}
	if p.Channel == 0 && p.Address == knownPT280Address {
		p.Channel = 1
	}
	if p.PaperWidthMM == 0 {
		p.PaperWidthMM = 58
	}
	if p.PrintableWidthDots == 0 {
		p.PrintableWidthDots = 384
	}
	if p.CharactersPerLine == 0 {
		p.CharactersPerLine = 32
	}
	if p.ID == "" && bluetoothAddress.MatchString(p.Address) {
		p.ID = "bluetooth:" + p.Address
	}
	return p
}

func validPrinter(p thermalPrinter) bool {
	if p.Protocol != "escpos" || p.PaperWidthMM < 48 || p.PaperWidthMM > 112 || p.CharactersPerLine < 24 || p.CharactersPerLine > 64 || p.PrintableWidthDots < 256 || p.PrintableWidthDots > 832 {
		return false
	}
	switch p.Transport {
	case "bluetooth":
		return bluetoothAddress.MatchString(p.Address) && p.Channel > 0 && p.Channel < 31
	case "serial", "usb":
		return validDevicePath(p.DevicePath)
	default:
		return false
	}
}

func validDevicePath(path string) bool {
	if runtime.GOOS == "windows" {
		return windowsCOMPort.MatchString(path)
	}
	return linuxDevicePath.MatchString(path)
}

func (s *Server) loadPrinter(ctx context.Context) (thermalPrinter, bool) {
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='thermal_printer'").Scan(&raw); err != nil {
		return thermalPrinter{}, false
	}
	var p thermalPrinter
	if json.Unmarshal([]byte(raw), &p) != nil {
		return thermalPrinter{}, false
	}
	p = normalizePrinter(p)
	return p, validPrinter(p)
}

func (s *Server) handlePrintersGet(w http.ResponseWriter, r *http.Request) {
	p, configured := s.loadPrinter(r.Context())
	if configured {
		p.Status = printerStatus(r.Context(), p)
		p.IsDefault = true
	}
	var selected any
	if configured {
		selected = p
	}
	writeJSON(w, http.StatusOK, map[string]any{"default": selected, "configured": configured, "platform": runtime.GOOS, "serverManaged": true})
}

func (s *Server) handlePrintersDiscover(w http.ResponseWriter, r *http.Request) {
	items, warning := discoverPrinters(r.Context())
	if selected, ok := s.loadPrinter(r.Context()); ok {
		for index := range items {
			if items[index].ID == selected.ID {
				items[index].IsDefault = true
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "platform": runtime.GOOS, "warning": warning})
}

func discoverPrinters(ctx context.Context) ([]thermalPrinter, string) {
	if runtime.GOOS != "linux" {
		return []thermalPrinter{}, "Add the Bluetooth printer in Windows, then enter its COM port."
	}
	commandCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "bluetoothctl", "devices").Output()
	if err != nil {
		return []thermalPrinter{}, "Bluetooth is disabled or BlueZ is unavailable."
	}
	items := []thermalPrinter{}
	for _, device := range parseBluetoothDevices(output) {
		address, name := device.Address, device.Name
		infoCtx, infoCancel := context.WithTimeout(ctx, 2*time.Second)
		info, _ := exec.CommandContext(infoCtx, "bluetoothctl", "info", address).Output()
		infoCancel()
		text := string(info)
		if address != knownPT280Address && !strings.HasPrefix(strings.ToUpper(name), "PT280") && !strings.Contains(text, "Serial Port") && !strings.Contains(text, "Icon: printer") {
			continue
		}
		p := thermalPrinter{ID: "bluetooth:" + address, Name: name, Model: name, Transport: "bluetooth", Address: address, Protocol: "escpos", Status: "available", PaperWidthMM: 58, PrintableWidthDots: 384, CharactersPerLine: 32}
		p.Channel = discoverRFCOMMChannel(ctx, address)
		if address == knownPT280Address {
			p.Model = "PT280UB"
			p.Channel = 1
		}
		p.Status = "available"
		p.LastSeenAt = time.Now().UTC().Format(time.RFC3339Nano)
		if strings.Contains(text, "Connected: yes") {
			p.Status = "connected"
		}
		items = append(items, p)
	}
	sort.SliceStable(items, func(i, j int) bool {
		score := func(p thermalPrinter) int {
			if p.Address == knownPT280Address {
				return 0
			}
			if p.Name == "PT280_6E27" {
				return 1
			}
			if strings.HasPrefix(strings.ToUpper(p.Name), "PT280") {
				return 2
			}
			return 3
		}
		return score(items[i]) < score(items[j])
	})
	return items, ""
}

func parseBluetoothDevices(output []byte) []bluetoothDevice {
	devices := []bluetoothDevice{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "Device" || !bluetoothAddress.MatchString(fields[1]) {
			continue
		}
		devices = append(devices, bluetoothDevice{Address: strings.ToUpper(fields[1]), Name: strings.Join(fields[2:], " ")})
	}
	return devices
}

func discoverRFCOMMChannel(ctx context.Context, address string) int {
	if !bluetoothAddress.MatchString(address) {
		return 0
	}
	commandCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "sdptool", "browse", address).Output()
	if err != nil {
		return 0
	}
	return parseSDPChannel(output)
}

func parseSDPChannel(output []byte) int {
	match := sdpChannel.FindSubmatch(output)
	if len(match) != 2 {
		return 0
	}
	channel, _ := strconv.Atoi(string(match[1]))
	if channel < 1 || channel > 30 {
		return 0
	}
	return channel
}

func (s *Server) handlePrinterSave(w http.ResponseWriter, r *http.Request) {
	var p thermalPrinter
	if decodeJSON(r, &p) != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PRINTER", "Printer configuration is invalid.")
		return
	}
	p = normalizePrinter(p)
	p.IsDefault = true
	if !validPrinter(p) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PRINTER", "Choose a supported printer and valid connection settings.")
		return
	}
	actor, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	data, _ := json.Marshal(p)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO settings(key,value_json,updated_at,updated_by) VALUES('thermal_printer',?,?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,version=settings.version+1,updated_at=excluded.updated_at,updated_by=excluded.updated_by`, string(data), now, actor.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PRINTER_SAVE_FAILED", "Could not save printer settings.")
		return
	}
	s.audit(r.Context(), &actor, "update", "printer", p.ID, "Updated the default receipt printer", "", "", r)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePrinterTest(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadPrinter(r.Context())
	if !ok {
		writeError(w, http.StatusConflict, "PRINTER_NOT_CONFIGURED", "Choose a default printer first.")
		return
	}
	clinic, locale := s.clinicDisplayName(r.Context()), s.receiptLocale(r.Context())
	err := s.printAndRecord(r, p, "", "test", thermal.RenderTestPage(clinic, p.Name, p.Model, locale, s.brandingRaster(r.Context(), p.PrintableWidthDots), p.CharactersPerLine))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, printerErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "printed"})
}

func (s *Server) handlePrinterInvoice(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadPrinter(r.Context())
	if !ok {
		writeError(w, http.StatusConflict, "PRINTER_NOT_CONFIGURED", "Choose a default printer first.")
		return
	}
	receipt, invoiceNumber, err := s.receiptForInvoice(r.Context(), chi.URLParam(r, "id"))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "INVOICE_NOT_FOUND", "Invoice was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECEIPT_LOAD_FAILED", "Could not prepare the receipt.")
		return
	}
	if err = s.printAndRecord(r, p, chi.URLParam(r, "id"), invoiceNumber, thermal.Render58mm(receipt, p.CharactersPerLine)); err != nil {
		writeError(w, http.StatusServiceUnavailable, printerErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "printed", "invoiceNumber": invoiceNumber})
}

// brandingRaster renders the clinic logo as an ESC/POS bitmap for the test page.
//
// A sale receipt deliberately does not carry it. A 58 mm head prints a bitmap
// far slower than text, and a portable printer’s receive buffer is small
// enough that a full-width logo can fill it and leave the transaction that
// follows unprinted — a customer waiting at the till would be handed a picture
// instead of what they just paid for. The clinic name, address and phone are
// printed as text at the top of every receipt instead.
func (s *Server) brandingRaster(ctx context.Context, maximumWidth int) []byte {
	logo := defaultClinicLogo
	var filename string
	if err := s.db.QueryRowContext(ctx, "SELECT filename FROM branding_assets WHERE key='clinic_logo'").Scan(&filename); err == nil {
		if stored, readErr := os.ReadFile(filepath.Join(s.config.DataDir, "branding", filepath.Base(filename))); readErr == nil {
			logo = stored
		}
	}
	raster, err := thermal.RasterizeLogo(logo, maximumWidth)
	if err != nil {
		return nil
	}
	return raster
}

func (s *Server) receiptForInvoice(ctx context.Context, id string) (thermal.Receipt, string, error) {
	var r thermal.Receipt
	var currency, createdAt, patientID, nif, footer string
	var subtotal, discount, tax, total, paid int64
	err := s.db.QueryRowContext(ctx, `SELECT i.invoice_number,i.currency,i.created_at,COALESCE(i.patient_id,''),COALESCE(p.first_name||' '||p.last_name,'Retail customer'),i.subtotal_minor,i.discount_minor,i.tax_minor,i.total_minor,COALESCE((SELECT SUM(amount_minor) FROM payments WHERE invoice_id=i.id),0),COALESCE(json_extract(st.value_json,'$.name'),''),COALESCE(json_extract(st.value_json,'$.address'),''),COALESCE(json_extract(st.value_json,'$.phone'),''),COALESCE(json_extract(st.value_json,'$.nif'),''),COALESCE(json_extract(st.value_json,'$.receiptFooter'),''),COALESCE(u.display_name,'') FROM invoices i LEFT JOIN patients p ON p.id=i.patient_id LEFT JOIN settings st ON st.key='clinic' LEFT JOIN users u ON u.id=i.created_by WHERE i.id=? AND i.archived_at IS NULL`, id).Scan(&r.InvoiceNumber, &currency, &createdAt, &patientID, &r.Patient, &subtotal, &discount, &tax, &total, &paid, &r.ClinicName, &r.Address, &r.Phone, &nif, &footer, &r.Cashier)
	if err != nil {
		return r, "", err
	}
	r.Date = s.receiptDate(ctx, createdAt)
	r.PatientID = patientID
	r.NIF = nif
	r.Footer = footer
	r.Locale = s.receiptLocale(ctx)
	if strings.TrimSpace(r.ClinicName) == "" {
		r.ClinicName = s.clinicDisplayName(ctx)
	}
	r.Subtotal = formatMinor(subtotal, currency)
	r.Discount = formatMinor(discount, currency)
	r.Tax = formatMinor(tax, currency)
	r.Total = formatMinor(total, currency)
	r.Tendered = formatMinor(paid, currency)
	if paid > total {
		r.Change = formatMinor(paid-total, currency)
	}
	r.Status = "PAID"
	if paid < total {
		r.Status = "PARTIAL"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT description,quantity,unit_price_minor,line_total_minor FROM invoice_items WHERE invoice_id=? ORDER BY rowid`, id)
	if err != nil {
		return r, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var line thermal.Line
		var unitPrice, amount int64
		if err = rows.Scan(&line.Description, &line.Quantity, &unitPrice, &amount); err != nil {
			return r, "", err
		}
		line.UnitPrice = formatMinor(unitPrice, currency)
		line.Total = formatMinor(amount, currency)
		r.Lines = append(r.Lines, line)
	}
	if err = rows.Err(); err != nil {
		return r, "", err
	}
	payRows, err := s.db.QueryContext(ctx, `SELECT pm.name,p.amount_minor,COALESCE(p.reference,'') FROM payments p JOIN payment_methods pm ON pm.id=p.payment_method_id WHERE p.invoice_id=? ORDER BY p.received_at`, id)
	if err != nil {
		return r, "", err
	}
	defer payRows.Close()
	for payRows.Next() {
		var p thermal.Payment
		var amount int64
		if err = payRows.Scan(&p.Method, &amount, &p.Reference); err != nil {
			return r, "", err
		}
		p.Amount = formatMinor(amount, currency)
		r.Payments = append(r.Payments, p)
	}
	return r, r.InvoiceNumber, payRows.Err()
}

func (s *Server) receiptLocale(ctx context.Context) string {
	var locale string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(json_extract(value_json,'$.language'),'en') FROM settings WHERE key='localization'`).Scan(&locale); err != nil {
		return "en"
	}
	return locale
}

func (s *Server) receiptDate(ctx context.Context, value string) string {
	location := time.Local
	var timezone string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(json_extract(value_json,'$.timezone'),'') FROM settings WHERE key='clinic'`).Scan(&timezone); err == nil && timezone != "" {
		if configured, loadErr := time.LoadLocation(timezone); loadErr == nil {
			location = configured
		}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.In(location).Format("02/01/2006 15:04")
		}
	}
	return value
}

func formatMinor(amount int64, currency string) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	return fmt.Sprintf("%s%d.%02d %s", sign, amount/100, amount%100, currency)
}

// Printing is a physical act: once the head starts moving, abandoning the job
// because the browser tab closed leaves half a receipt on the roll. The job
// therefore runs on the request's values but on its own deadline, and the
// attempt is recorded either way.
const (
	printJobTimeout = 60 * time.Second
	printQueueWait  = 25 * time.Second
	// A portable thermal printer holds only a few kilobytes and discards what
	// arrives once that is full, so every transport feeds it in small pieces.
	// Roughly 12 kB/s: below what a 58 mm head consumes, and imperceptible on a
	// receipt that is under a kilobyte of text.
	printerChunkBytes = 256
	printerChunkPause = 20 * time.Millisecond
)

// acquirePrinter serialises jobs. The clinic has one physical printer and it
// accepts one connection at a time, so desktop, browser and phone clients queue
// here instead of racing each other into a BlueZ "busy" refusal.
func (s *Server) acquirePrinter(ctx context.Context) error {
	select {
	case s.printSlots <- struct{}{}:
		return nil
	default:
	}
	wait, cancel := context.WithTimeout(ctx, printQueueWait)
	defer cancel()
	select {
	case s.printSlots <- struct{}{}:
		return nil
	case <-wait.Done():
		return printerFailure("PRINTER_BUSY", "Another receipt is still printing. Wait for it to finish, then print again.", wait.Err())
	}
}

func (s *Server) printAndRecord(r *http.Request, p thermalPrinter, invoiceID, receiptID string, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), printJobTimeout)
	defer cancel()
	err := s.acquirePrinter(ctx)
	if err == nil {
		defer func() { <-s.printSlots }()
		err = printThermal(ctx, p, payload)
	}
	status, code := "printed", ""
	if err != nil {
		status = "failed"
		code = printerErrorCode(err)
		s.logger.Warn("thermal print failed", "printer_id", p.ID, "transport", p.Transport, "error_code", code, "error", err, "cause", printerCause(err))
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO print_jobs(id,invoice_id,receipt_id,printer_id,printer_name,status,error_code,created_at) VALUES(?,NULLIF(?,''),?,?,?,?,?,?)`, uuid.NewString(), invoiceID, receiptID, p.ID, p.Name, status, code, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func printerStatus(ctx context.Context, p thermalPrinter) string {
	if p.DevicePath != "" {
		if _, err := os.Stat(normalizedDevicePath(p.DevicePath)); err == nil {
			return "ready"
		}
	}
	if runtime.GOOS != "linux" {
		return "configured"
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(c, "bluetoothctl", "info", p.Address).Output()
	if err != nil {
		return "offline"
	}
	if bytes.Contains(output, []byte("Connected: yes")) {
		return "connected"
	}
	if bytes.Contains(output, []byte("Paired: yes")) {
		return "available"
	}
	return "offline"
}

func printThermal(ctx context.Context, p thermalPrinter, payload []byte) error {
	if !validPrinter(p) {
		return printerFailure("PRINTER_UNAVAILABLE", "The saved printer configuration is invalid. Select the printer again under System \u2192 Printers.", nil)
	}
	if p.DevicePath != "" {
		return writePrinterDevice(ctx, p.DevicePath, payload)
	}
	return writeBluetoothRFCOMM(ctx, p.Address, p.Channel, payload)
}

func normalizedDevicePath(path string) string {
	if runtime.GOOS == "windows" && windowsCOMPort.MatchString(path) {
		return `\\.\` + strings.ToUpper(path)
	}
	return filepath.Clean(path)
}
func writePrinterDevice(ctx context.Context, path string, payload []byte) error {
	if !validDevicePath(path) {
		return printerFailure("PRINTER_UNAVAILABLE", "The saved printer configuration is invalid. Select the printer again under System \u2192 Printers.", nil)
	}
	file, err := os.OpenFile(normalizedDevicePath(path), os.O_WRONLY, 0)
	if err != nil {
		return describeDeviceError(err)
	}
	defer file.Close()
	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > printerChunkBytes {
			chunk = chunk[:printerChunkBytes]
		}
		if _, err = file.Write(chunk); err != nil {
			return printerFailure("PRINTER_WRITE_FAILED", "The printer dropped the connection while the receipt was being sent. Check the paper roll and print again.", err)
		}
		payload = payload[len(chunk):]
		if len(payload) > 0 {
			select {
			case <-ctx.Done():
				return printerFailure("PRINTER_TIMEOUT", "The printer did not answer in time. Check that it is switched on, has paper and is within Bluetooth range.", ctx.Err())
			case <-time.After(printerChunkPause):
			}
		}
	}
	return nil
}

func describeDeviceError(err error) error {
	switch {
	case errors.Is(err, os.ErrPermission):
		return printerFailure("PRINTER_PERMISSION_DENIED", "This computer does not allow the clinic server to open the printer port. Grant the account that runs SentryMed access to the port, then print again.", err)
	case errors.Is(err, os.ErrNotExist):
		return printerFailure("PRINTER_OFFLINE", "The printer port no longer exists. Switch the printer on, reconnect it, then print again.", err)
	case errors.Is(err, syscall.EBUSY):
		return printerFailure("PRINTER_BUSY", "The printer port is held by another program. Close it, then print again.", err)
	default:
		return printerFailure("PRINTER_CONNECTION_FAILED", "Could not open the connection to the printer.", err)
	}
}

// printerError carries the code the clients switch on together with the remedy
// the clinic can act on, so the reason a receipt did not print survives the trip
// from the Bluetooth stack to the cashier's screen and the print history.
type printerError struct {
	code    string
	message string
	cause   error
}

func (e *printerError) Error() string { return e.message }
func (e *printerError) Unwrap() error { return e.cause }

func printerFailure(code, message string, cause error) error {
	return &printerError{code: code, message: message, cause: cause}
}

// printerCause is the underlying system error, kept for the server log only.
func printerCause(err error) string {
	var typed *printerError
	if errors.As(err, &typed) && typed.cause != nil {
		return typed.cause.Error()
	}
	return ""
}

func printerErrorCode(err error) string {
	var typed *printerError
	if errors.As(err, &typed) {
		return typed.code
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "permission"):
		return "PRINTER_PERMISSION_DENIED"
	case strings.Contains(text, "offline") || strings.Contains(text, "range"):
		return "PRINTER_OFFLINE"
	case strings.Contains(text, "busy"):
		return "PRINTER_BUSY"
	case strings.Contains(text, "timed out"):
		return "PRINTER_TIMEOUT"
	case strings.Contains(text, "rfcomm"):
		return "PRINTER_CONNECTION_FAILED"
	case strings.Contains(text, "open printer"):
		return "PRINTER_CONNECTION_FAILED"
	case strings.Contains(text, "write"):
		return "PRINTER_WRITE_FAILED"
	default:
		return "PRINTER_UNAVAILABLE"
	}
}

func (s *Server) handlePrintJobsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,COALESCE(invoice_id,''),receipt_id,printer_id,printer_name,status,COALESCE(error_code,''),created_at FROM print_jobs ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		writeError(w, 500, "PRINT_HISTORY_FAILED", "Could not load print history.")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, invoice, receipt, printerID, name, status, code, created string
		if err := rows.Scan(&id, &invoice, &receipt, &printerID, &name, &status, &code, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "PRINT_HISTORY_FAILED", "Could not read print history.")
			return
		}
		items = append(items, map[string]string{"id": id, "invoiceId": invoice, "receiptId": receipt, "printerId": printerID, "printerName": name, "status": status, "errorCode": code, "createdAt": created})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "PRINT_HISTORY_FAILED", "Could not read print history.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
