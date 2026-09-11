// Package thermal renders narrow receipts without relying on a browser or PDF.
package thermal

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	initialize = "\x1b@"
	center     = "\x1ba\x01"
	left       = "\x1ba\x00"
	boldOn     = "\x1bE\x01"
	boldOff    = "\x1bE\x00"
	feedCut    = "\n\n\n\x1dV\x00"
)

type Line struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   string `json:"unitPrice"`
	Total       string `json:"total"`
}
type Payment struct {
	Method    string `json:"method"`
	Amount    string `json:"amount"`
	Reference string `json:"reference"`
}
type Receipt struct {
	Locale        string `json:"locale"`
	ClinicName    string `json:"clinicName"`
	Address       string `json:"address"`
	Phone         string `json:"phone"`
	NIF           string `json:"nif"`
	InvoiceNumber string `json:"invoiceNumber"`
	Date          string `json:"date"`
	Patient       string `json:"patient"`
	PatientID     string `json:"patientId"`
	Cashier       string `json:"cashier"`
	Lines         []Line
	Subtotal      string `json:"subtotal"`
	Discount      string `json:"discount"`
	Tax           string `json:"tax"`
	Total         string `json:"total"`
	Tendered      string `json:"tendered"`
	Change        string `json:"change"`
	Status        string `json:"status"`
	Payments      []Payment
	Footer        string `json:"footer"`
	LogoRaster    []byte `json:"-"`
}

type receiptLabels struct {
	Title, Invoice, Date, Patient, PatientID, Cashier string
	Subtotal, Discount, Tax, Total, Reference         string
	Received, Change, Paid, Partial, Thanks           string
}

var labelsByLocale = map[string]receiptLabels{
	"en":    {"PAYMENT RECEIPT", "Invoice", "Date", "Customer", "Patient ID", "Cashier", "Subtotal", "Discount", "Tax", "TOTAL", "Reference", "Received", "Change", "PAID", "PARTIAL", "Thank you for your trust."},
	"fr":    {"RECU DE PAIEMENT", "Facture", "Date", "Patient", "ID patient", "Caissier", "Sous-total", "Remise", "Taxes", "TOTAL", "Reference", "Recu", "Monnaie", "PAYE", "PARTIEL", "Merci pour votre confiance."},
	"ht":    {"RESI PEMAN", "Fakti", "Dat", "Pasyan", "ID pasyan", "Kesye", "Sou-total", "Rabè", "Taks", "TOTAL", "Referans", "Resevwa", "Monnen", "PEYE", "PASYEL", "Mèsi pou konfyans ou."},
	"pt":    {"RECIBO DE PAGAMENTO", "Fatura", "Data", "Cliente", "ID paciente", "Caixa", "Subtotal", "Desconto", "Impostos", "TOTAL", "Referencia", "Recebido", "Troco", "PAGO", "PARCIAL", "Obrigado pela sua confiança."},
	"es":    {"RECIBO DE PAGO", "Factura", "Fecha", "Cliente", "ID paciente", "Cajero", "Subtotal", "Descuento", "Impuestos", "TOTAL", "Referencia", "Recibido", "Cambio", "PAGADO", "PARCIAL", "Gracias por su confianza."},
	"de":    {"ZAHLUNGSBELEG", "Rechnung", "Datum", "Kunde", "Patienten-ID", "Kasse", "Zwischensumme", "Rabatt", "Steuer", "GESAMT", "Referenz", "Erhalten", "Ruckgeld", "BEZAHLT", "TEILZAHLUNG", "Vielen Dank fur Ihr Vertrauen."},
	"zh-CN": {"SHOUKUAN SHOUJU", "Fapiao", "Riqi", "Kehu", "Huanzhe ID", "Shouyinyuan", "Xiaoji", "Youhui", "Shui", "ZONGJI", "Cankao", "Shishou", "Zhaoling", "YI FUKUAN", "BUFen FUKUAN", "Ganxie nin de xinren."},
	"ru":    {"KVITANTSIYA OB OPLATE", "Schet", "Data", "Klient", "ID patsienta", "Kassir", "Poditog", "Skidka", "Nalog", "ITOGO", "Ssylka", "Polucheno", "Sdacha", "OPLACHENO", "CHASTICHNO", "Spasibo za doverie."},
	"ja":    {"OSHIHARAI RESHIITO", "Seikyusho", "Hizuke", "Okyakusama", "Kanja ID", "Kashaa", "Shokei", "Waribiki", "Zei", "GOKEI", "Sansho", "Oazukari", "Otsuri", "SHIHARAI ZUMI", "ICHIBU SHIHARAI", "Goriyo arigato gozaimasu."},
	"ko":    {"GYEOLJE YEONGSUJEUNG", "Cheongguseo", "Naljja", "Gogaek", "Hwanja ID", "Gyesanwon", "Sogye", "Halin", "Segum", "HAPGYE", "Chamjo", "Badeun geumaek", "Geoseureumdon", "GYEOLJE WANRYO", "ILBU GYEOLJE", "Mideum-e gamsahamnida."},
	"id":    {"TANDA TERIMA PEMBAYARAN", "Faktur", "Tanggal", "Pelanggan", "ID pasien", "Kasir", "Subtotal", "Diskon", "Pajak", "TOTAL", "Referensi", "Diterima", "Kembalian", "LUNAS", "SEBAGIAN", "Terima kasih atas kepercayaan Anda."},
}

func labels(locale string) receiptLabels {
	if value, ok := labelsByLocale[locale]; ok {
		return value
	}
	return labelsByLocale["en"]
}

// Render58mm returns ESC/POS bytes for a conservative 32-character text mode.
// Text is deliberately converted to the printer-safe subset; this avoids
// assuming UTF-8 support from inexpensive portable printers.
func Render58mm(r Receipt, width int) []byte {
	if width < 24 || width > 64 {
		width = 32
	}
	var b strings.Builder
	l := labels(r.Locale)
	if len(r.LogoRaster) > 0 {
		b.WriteString(initialize + center)
		_, _ = b.Write(r.LogoRaster)
		b.WriteString("\n")
	}
	b.WriteString(initialize + center + boldOn + lineCenter(r.ClinicName, width) + boldOff)
	for _, value := range []string{r.Address, r.Phone, r.NIF} {
		if value != "" {
			b.WriteString(lineCenter(value, width))
		}
	}
	b.WriteString(left + lineCenter(l.Title, width) + sep(width))
	for _, pair := range [][2]string{{l.Invoice, r.InvoiceNumber}, {l.Date, r.Date}, {l.Patient, r.Patient}, {l.PatientID, r.PatientID}, {l.Cashier, r.Cashier}} {
		if pair[1] != "" {
			b.WriteString(leftRight(pair[0]+" :", pair[1], width))
		}
	}
	b.WriteString(sep(width))
	for _, item := range r.Lines {
		b.WriteString(itemLine(item, width))
	}
	b.WriteString(sep(width))
	for _, pair := range [][2]string{{l.Subtotal, r.Subtotal}, {l.Discount, r.Discount}, {l.Tax, r.Tax}} {
		if pair[1] != "" && pair[1] != "0" {
			b.WriteString(leftRight(pair[0], pair[1], width))
		}
	}
	b.WriteString(boldOn + leftRight(l.Total, r.Total, width) + boldOff + sep(width))
	for _, payment := range r.Payments {
		b.WriteString(leftRight(payment.Method, payment.Amount, width))
		if payment.Reference != "" {
			b.WriteString(leftRight(l.Reference, payment.Reference, width))
		}
	}
	if r.Tendered != "" {
		b.WriteString(leftRight(l.Received, r.Tendered, width))
	}
	if r.Change != "" {
		b.WriteString(leftRight(l.Change, r.Change, width))
	}
	status := r.Status
	if status == "" {
		status = l.Paid
	} else if status == "PAID" {
		status = l.Paid
	} else if status == "PARTIAL" {
		status = l.Partial
	}
	b.WriteString(center + boldOn + ascii(status) + "\n" + boldOff + sep(width))
	if r.Footer == "" {
		r.Footer = l.Thanks
	}
	b.WriteString(lineCenter(r.Footer, width) + lineCenter(r.ClinicName, width))
	b.WriteString(feedCut)
	return []byte(b.String())
}

// RenderTestPage exercises text, formatting and the optional bitmap without
// needing an invoice. The same transport path is used by real receipts.
func RenderTestPage(clinicName, printerName, model, locale string, logo []byte, width int) []byte {
	testTitle := map[string]string{"fr": "Test imprimante", "ht": "Tès enprimant", "pt": "Teste da impressora", "es": "Prueba de impresora", "de": "Druckertest", "zh-CN": "Dayinji ceshi", "ru": "Test printera", "ja": "Purinta tesuto", "ko": "Peurinteo teseuteu", "id": "Tes printer"}[locale]
	success := map[string]string{"fr": "TEST REUSSI", "ht": "TES LA REYISI", "pt": "TESTE CONCLUIDO", "es": "PRUEBA CORRECTA", "de": "TEST ERFOLGREICH", "zh-CN": "CESHI CHENGGONG", "ru": "TEST USPESHEN", "ja": "TESUTO SEIKO", "ko": "TESEUTEU SEONGGONG", "id": "TES BERHASIL"}[locale]
	if testTitle == "" {
		testTitle = "Printer test"
	}
	if success == "" {
		success = "TEST SUCCESSFUL"
	}
	var b strings.Builder
	b.WriteString(initialize + center)
	if len(logo) > 0 {
		_, _ = b.Write(logo)
		b.WriteByte('\n')
	}
	b.WriteString(boldOn + lineCenter(clinicName, width) + boldOff + lineCenter(testTitle, width) + sep(width))
	b.WriteString(left + leftRight("Printer", printerName, width) + leftRight("Model", model, width))
	b.WriteString("Bluetooth / RFCOMM: OK\nESC/POS: OK\n\nABCDEFGHIJKLMNOPQRSTUVWXYZ\nabcdefghijklmnopqrstuvwxyz\n0123456789\n")
	b.WriteString(sep(width) + center + boldOn + ascii(success) + "\n" + boldOff + feedCut)
	return []byte(b.String())
}
func sep(width int) string { return strings.Repeat("-", width) + "\n" }
func lineCenter(value string, width int) string {
	value = ascii(value)
	if len(value) > width {
		value = value[:width]
	}
	return strings.Repeat(" ", (width-len(value))/2) + value + "\n"
}
func leftRight(l, r string, width int) string {
	l, r = ascii(l), ascii(r)
	if len(r) >= width {
		return r[:width] + "\n"
	}
	if len(l)+len(r) > width {
		l = l[:width-len(r)]
	}
	return l + strings.Repeat(" ", width-len(l)-len(r)) + r + "\n"
}
func itemLine(item Line, width int) string {
	name := ascii(item.Description)
	if item.Quantity > 1 && item.UnitPrice != "" {
		var b strings.Builder
		for len(name) > 0 {
			n := min(width, len(name))
			if len(name) > n {
				if cut := strings.LastIndex(name[:n], " "); cut > 0 {
					n = cut
				}
			}
			b.WriteString(name[:n] + "\n")
			name = strings.TrimSpace(name[n:])
		}
		b.WriteString(leftRight(fmt.Sprintf("%d x %s", item.Quantity, item.UnitPrice), item.Total, width))
		return b.String()
	}
	if len(name)+len(item.Total) <= width {
		return leftRight(name, item.Total, width)
	}
	var b strings.Builder
	for len(name) > 0 {
		n := min(width, len(name))
		if len(name) > n {
			if cut := strings.LastIndex(name[:n], " "); cut > 0 {
				n = cut
			}
		}
		b.WriteString(name[:n] + "\n")
		name = strings.TrimSpace(name[n:])
	}
	b.WriteString(leftRight("", item.Total, width))
	return b.String()
}
func ascii(s string) string {
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
		} else if r == '\n' || r == '\t' {
			b.WriteRune(r)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}
