package server

import "testing"

// These mirror the acceptance examples in the currency audit: a coverage
// split must never lose or invent a minor unit to rounding, and it must round
// to the nearest unit rather than always truncating downward.
func TestApplyPercentMinorRoundsHalfUp(t *testing.T) {
	cases := []struct {
		amount  int64
		percent float64
		want    int64
	}{
		{100, 50, 50},
		{65000, 80, 52000},     // 650.00 HTG at 80% = 520.00 HTG, exact
		{100000, 33.33, 33330}, // 1,000.00 HTG at 33.33% = 333.30 HTG
		{1, 50, 1},             // 0.01 at 50% rounds up to 0.01, never to 0
		{3, 50, 2},             // 1.5 rounds half-up to 2
		{0, 80, 0},
		{100, 0, 0},
		{100, 100, 100},
	}
	for _, c := range cases {
		if got := applyPercentMinor(c.amount, c.percent); got != c.want {
			t.Errorf("applyPercentMinor(%d, %v) = %d, want %d", c.amount, c.percent, got, c.want)
		}
	}
}

func TestSplitMinorNeverLosesAMinorUnit(t *testing.T) {
	cases := []struct {
		total   int64
		percent float64
	}{
		{1000000, 70}, {1, 1}, {999999, 33.33}, {65050, 62.5}, {0, 50}, {7, 100},
	}
	for _, c := range cases {
		first, remainder := splitMinor(c.total, c.percent)
		if first+remainder != c.total {
			t.Errorf("splitMinor(%d, %v) = (%d, %d), sum %d != total %d", c.total, c.percent, first, remainder, first+remainder, c.total)
		}
		if first < 0 || remainder < 0 {
			t.Errorf("splitMinor(%d, %v) produced a negative share: (%d, %d)", c.total, c.percent, first, remainder)
		}
	}
}
