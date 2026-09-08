package server

import "strings"

// D1: the two optional registration fields. Both are validated against a closed
// list so that a typo cannot silently create a new category, and both may be
// left blank — "not recorded" stays distinct from "prefer not to say".

var civilStatuses = []string{"single", "married", "common_law", "divorced", "separated", "widowed"}

// The starting list reflects the clinic's own patient population and is pending
// confirmation with clinic staff; "other" carries free text and
// "prefer_not_to_say" is a recorded refusal, not a blank.
var religions = []string{
	"catholic", "protestant", "baptist", "adventist", "pentecostal", "methodist",
	"jehovahs_witness", "vodou", "muslim", "jewish", "none", "other", "prefer_not_to_say",
}

func allowedValue(value string, allowed []string) bool {
	if value == "" {
		return true
	}
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

// normaliseDemographics validates the two fields and drops free text that no
// longer applies, so a record cannot keep an orphaned "other" description after
// the religion is changed to a listed one.
func normaliseDemographics(input *patientPayload) string {
	input.CivilStatus = strings.TrimSpace(input.CivilStatus)
	input.Religion = strings.TrimSpace(input.Religion)
	input.ReligionOther = strings.TrimSpace(input.ReligionOther)
	if !allowedValue(input.CivilStatus, civilStatuses) {
		return "Civil status must be one of: " + strings.Join(civilStatuses, ", ") + "."
	}
	if !allowedValue(input.Religion, religions) {
		return "Religion must be one of the listed options."
	}
	if input.Religion != "other" {
		input.ReligionOther = ""
	}
	if len(input.ReligionOther) > 120 {
		return "The religion description must be 120 characters or fewer."
	}
	return ""
}

func (s *Server) demographicOptions() map[string]any {
	return map[string]any{"civilStatuses": civilStatuses, "religions": religions}
}
