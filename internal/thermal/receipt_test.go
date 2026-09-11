package thermal

import (
	"strings"
	"testing"
)

func TestRender58mmUsesEscPosAndKeepsAmountsAligned(t *testing.T) {
	output := string(Render58mm(Receipt{Locale: "fr", ClinicName: "Clinique Le Bon Spécialiste", InvoiceNumber: "INV-2026-00124", Lines: []Line{{Description: "Verres progressifs photochromiques", Quantity: 1, Total: "15,500"}}, Total: "15,500"}, 32))
	if !strings.HasPrefix(output, initialize) || !strings.Contains(output, "PAYE") || !strings.HasSuffix(output, feedCut) {
		t.Fatalf("invalid ESC/POS receipt: %q", output)
	}
	if strings.Contains(output, "é") || !strings.Contains(output, "Specialiste") {
		t.Fatalf("receipt was not converted to printer-safe text: %q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		visible := strings.ReplaceAll(strings.ReplaceAll(line, boldOn, ""), boldOff, "")
		if strings.Contains(visible, "15,500") && len(visible) > 0 && len(visible) != 32 {
			t.Fatalf("amount line width=%d: %q", len(visible), visible)
		}
	}
}

func TestRender58mmHandlesLargeReceiptsAndLocales(t *testing.T) {
	items := make([]Line, 24)
	for index := range items {
		items[index] = Line{Description: "Article avec un nom particulièrement long numéro", Quantity: index + 1, UnitPrice: "5,144.03 HTG", Total: "123,456.78 HTG"}
	}
	for _, locale := range []string{"en", "fr", "ht", "pt", "es", "de", "zh-CN", "ru", "ja", "ko", "id"} {
		output := string(Render58mm(Receipt{Locale: locale, ClinicName: "Clinique Le Bon Spécialiste", Lines: items, Discount: "500 HTG", Total: "123,456.78 HTG", Status: "PAID"}, 32))
		if strings.ContainsRune(output, 'é') || !strings.Contains(output, "123,456.78 HTG") {
			t.Fatalf("locale %s produced unsafe or incomplete output", locale)
		}
	}
}

func TestItemQuantityAndUnitPriceAreVisible(t *testing.T) {
	output := string(Render58mm(Receipt{Locale: "en", ClinicName: "Clinic", Lines: []Line{{Description: "Consultation", Quantity: 2, UnitPrice: "1,250 HTG", Total: "2,500 HTG"}}, Total: "2,500 HTG"}, 32))
	if !strings.Contains(output, "2 x 1,250 HTG") || !strings.Contains(output, "2,500 HTG") {
		t.Fatalf("quantity or unit price missing: %q", output)
	}
}

func TestRenderTestPageExercisesCommunication(t *testing.T) {
	output := string(RenderTestPage("Clinique Le Bon Spécialiste", "PT280_6E27", "PT280UB", "fr", []byte{0x1d, 'v', '0', 0}, 32))
	for _, expected := range []string{"PT280_6E27", "PT280UB", "Bluetooth / RFCOMM: OK", "ESC/POS: OK", "TEST REUSSI"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("test page missing %q", expected)
		}
	}
}
