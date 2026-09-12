package server

import "math"

// This file centralizes the arithmetic the rest of the package needs for
// exact-money calculations. Every persisted amount is an integer count of
// minor currency units (see docs/DATABASE.md), so addition, subtraction and
// comparison are already exact int64 operations — nothing here should ever
// introduce a float64 into a stored amount. The one place a fraction is
// unavoidable is applying a coverage percentage (a REAL column, since a
// contract can say "80%" or "62.5%") to a minor-unit amount; applyPercentMinor
// is the single place that conversion happens, so it can be rounded correctly
// once instead of truncated inconsistently in several handlers.

// applyPercentMinor returns percent% of amountMinor, rounded to the nearest
// minor unit (round-half-up) rather than truncated. Truncation would silently
// and systematically under-count every split by up to one minor unit; over
// many claims that bias adds up in the clinic's favor or the insurer's
// depending on which side reads the truncated figure, which is not a decision
// this function should make on its own.
func applyPercentMinor(amountMinor int64, percent float64) int64 {
	if amountMinor <= 0 || percent <= 0 {
		return 0
	}
	if percent >= 100 {
		return amountMinor
	}
	return int64(math.Floor(float64(amountMinor)*percent/100 + 0.5))
}

// splitMinor divides totalMinor into a "first" share (percent% of the total,
// rounded) and a "remainder" share, defined as whatever is left over. Deriving
// the second share by subtraction — rather than computing it independently
// with its own rounding — guarantees first+remainder always equals total
// exactly, with no minor unit ever lost or invented to rounding.
func splitMinor(totalMinor int64, percent float64) (first, remainder int64) {
	first = applyPercentMinor(totalMinor, percent)
	if first > totalMinor {
		first = totalMinor
	}
	return first, totalMinor - first
}
