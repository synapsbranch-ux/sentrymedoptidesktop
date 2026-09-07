package server

import "testing"

func numberPointer(value float64) *float64 { return &value }

func TestProgressionSourceDoesNotMixMeasurements(t *testing.T) {
	points := []trendPoint{
		{Date: "2024-01-01", Refractions: map[string]*refractionPoint{"autorefraction": {Source: "autorefraction", ODSE: numberPointer(-2)}}},
		{Date: "2025-01-01", Refractions: map[string]*refractionPoint{"subjective": {Source: "subjective", ODSE: numberPointer(-3)}, "autorefraction": {Source: "autorefraction", ODSE: numberPointer(-2.5)}}},
	}
	source, selector := progressionSource(points, "OD")
	if source != "autorefraction" {
		t.Fatalf("source = %q, want autorefraction", source)
	}
	if rate, _, ok := progressionRate(points, selector); !ok || rate != -0.5 {
		t.Fatalf("rate = %v, ok=%v, want -0.5 from a single source", rate, ok)
	}
}

func TestIOPCorrectionIsExplicitlyOptIn(t *testing.T) {
	measured, cornea := 20.0, 500.0
	settings := defaultClinicalAnalyticsSettings()
	settings.CCTMicronsPerMmHg = 25
	without := iopContext(&measured, &cornea, settings)
	if without == nil || without.Estimated != nil {
		t.Fatalf("disabled correction must not create an estimate: %#v", without)
	}
	settings.CCTCorrectionEnabled = true
	with := iopContext(&measured, &cornea, settings)
	if with == nil || with.Estimated == nil || *with.Estimated != 21.8 || *with.Measured != 20 {
		t.Fatalf("configured correction = %#v, want measured 20 and estimate 21.8", with)
	}
}

func TestFundusClockHour(t *testing.T) {
	for _, test := range []struct {
		x, y float64
		want int
	}{{50, 5, 12}, {95, 50, 3}, {50, 95, 6}, {5, 50, 9}} {
		if got := fundusClockHour(test.x, test.y); got != test.want {
			t.Errorf("fundusClockHour(%v,%v)=%d, want %d", test.x, test.y, got, test.want)
		}
	}
}
