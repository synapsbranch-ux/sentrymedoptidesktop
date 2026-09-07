// Package clinicalvalues converts the string-heavy clinical JSON stored by the
// consultation forms into typed, comparable values. It deliberately returns nil for
// missing or invalid measurements so reporting code cannot mistake them for zero.
package clinicalvalues

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

type EyeMeasurement struct {
	OD      *float64 `json:"od"`
	OS      *float64 `json:"os"`
	ODLabel string   `json:"odLabel,omitempty"`
	OSLabel string   `json:"osLabel,omitempty"`
}

type EyeRefraction struct {
	Sphere              *float64 `json:"sphere"`
	Cylinder            *float64 `json:"cylinder"`
	Axis                *float64 `json:"axis"`
	SphericalEquivalent *float64 `json:"sphericalEquivalent"`
}

type Refraction struct {
	Source string        `json:"source"`
	OD     EyeRefraction `json:"od"`
	OS     EyeRefraction `json:"os"`
}

type KeratometryEye struct {
	K1     *float64 `json:"k1"`
	K1Axis *float64 `json:"k1Axis"`
	K2     *float64 `json:"k2"`
	K2Axis *float64 `json:"k2Axis"`
}

type Keratometry struct {
	OD KeratometryEye `json:"od"`
	OS KeratometryEye `json:"os"`
}

func Round2(value float64) float64 { return math.Round(value*100) / 100 }

func canonicalKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "")
	return replacer.Replace(value)
}

// DecodeObject accepts both string-valued form blobs and number-valued imported JSON.
func DecodeObject(raw string) map[string]any {
	decoded := map[string]any{}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &decoded) != nil {
		return map[string]any{}
	}
	normalized := make(map[string]any, len(decoded))
	for key, value := range decoded {
		normalized[canonicalKey(key)] = value
	}
	return normalized
}

func normalizeNumeric(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "−", "-")
	value = strings.ReplaceAll(value, ",", ".")
	value = strings.TrimPrefix(value, "+")
	for _, suffix := range []string{"mmhg", "microns", "micron", "µm", "um", "d", "°"} {
		value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
	}
	return value
}

func Number(value any) (float64, bool) {
	var normalized string
	switch typed := value.(type) {
	case string:
		normalized = normalizeNumeric(typed)
	case float64:
		return Round2(typed), true
	case json.Number:
		normalized = string(typed)
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
	if normalized == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(normalized, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return Round2(parsed), true
}

func numberFrom(values map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if parsed, ok := Number(values[canonicalKey(key)]); ok {
			value := Round2(parsed)
			return &value
		}
	}
	return nil
}

func SnellenToLogMAR(value string) (float64, bool) {
	raw := strings.ToLower(strings.TrimSpace(value))
	if raw == "" {
		return 0, false
	}
	switch {
	case strings.HasPrefix(raw, "nlp"), strings.Contains(raw, "no light"):
		return 3.0, true
	case strings.HasPrefix(raw, "lp"), strings.Contains(raw, "light perception"):
		return 2.7, true
	case strings.HasPrefix(raw, "hm"), strings.Contains(raw, "hand"):
		return 2.3, true
	case strings.HasPrefix(raw, "cf"), strings.Contains(raw, "finger"):
		return 1.9, true
	}
	if index := strings.IndexAny(raw, "+-"); index > 0 && strings.Contains(raw[:index], "/") {
		raw = strings.TrimSpace(raw[:index])
	}
	if numerator, denominator, found := strings.Cut(raw, "/"); found {
		top, okTop := Number(numerator)
		bottom, okBottom := Number(denominator)
		if !okTop || !okBottom || top <= 0 || bottom <= 0 {
			return 0, false
		}
		return Round2(math.Log10(bottom / top)), true
	}
	decimal, ok := Number(raw)
	if !ok || decimal <= 0 {
		return 0, false
	}
	return Round2(-math.Log10(decimal)), true
}

func SphericalEquivalent(sphere, cylinder float64, hasCylinder bool) float64 {
	if !hasCylinder {
		return Round2(sphere)
	}
	return Round2(sphere + cylinder/2)
}

func refractionEye(values map[string]any, eye string, separateEye bool) EyeRefraction {
	prefixes := []string{eye}
	if separateEye {
		prefixes = append(prefixes, "")
	}
	keys := func(field string) []string {
		result := make([]string, 0, len(prefixes))
		for _, prefix := range prefixes {
			result = append(result, prefix+field)
		}
		return result
	}
	sphere := numberFrom(values, keys("sphere")...)
	cylinder := numberFrom(values, keys("cylinder")...)
	axis := numberFrom(values, keys("axis")...)
	measurement := EyeRefraction{Sphere: sphere, Cylinder: cylinder, Axis: axis}
	if sphere != nil {
		cylinderValue := 0.0
		if cylinder != nil {
			cylinderValue = *cylinder
		}
		value := SphericalEquivalent(*sphere, cylinderValue, cylinder != nil)
		measurement.SphericalEquivalent = &value
	}
	return measurement
}

func hasRefraction(value EyeRefraction) bool {
	return value.Sphere != nil || value.Cylinder != nil || value.Axis != nil
}

func RefractionFromCombined(raw, source string) *Refraction {
	values := DecodeObject(raw)
	result := &Refraction{Source: source, OD: refractionEye(values, "od", false), OS: refractionEye(values, "os", false)}
	if !hasRefraction(result.OD) && !hasRefraction(result.OS) {
		return nil
	}
	return result
}

func RefractionFromEyes(odRaw, osRaw, source string) *Refraction {
	result := &Refraction{
		Source: source,
		OD:     refractionEye(DecodeObject(odRaw), "od", true),
		OS:     refractionEye(DecodeObject(osRaw), "os", true),
	}
	if !hasRefraction(result.OD) && !hasRefraction(result.OS) {
		return nil
	}
	return result
}

func bestAcuityFor(values map[string]any, eye string) (*float64, string) {
	candidates := []struct{ key, label string }{
		{eye + "bestcorrectedva", "best corrected"},
		{eye + "correcteddistance", "corrected distance"},
		{eye + "presentingdistance", "presenting distance"},
		{eye + "uncorrecteddistance", "uncorrected distance"},
		{eye + "distanceva", "distance"},
	}
	for _, candidate := range candidates {
		raw, ok := values[canonicalKey(candidate.key)].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		if logMAR, valid := SnellenToLogMAR(raw); valid {
			value := logMAR
			return &value, raw + " (" + candidate.label + ")"
		}
	}
	return nil, ""
}

func VisualAcuity(raw string) *EyeMeasurement {
	values := DecodeObject(raw)
	od, odLabel := bestAcuityFor(values, "od")
	os, osLabel := bestAcuityFor(values, "os")
	if od == nil && os == nil {
		return nil
	}
	return &EyeMeasurement{OD: od, OS: os, ODLabel: odLabel, OSLabel: osLabel}
}

func IOP(raw string) *EyeMeasurement {
	values := DecodeObject(raw)
	result := &EyeMeasurement{OD: numberFrom(values, "odiop", "odpressure"), OS: numberFrom(values, "osiop", "ospressure")}
	if result.OD == nil && result.OS == nil {
		return nil
	}
	return result
}

func Pachymetry(raw string) *EyeMeasurement {
	values := DecodeObject(raw)
	result := &EyeMeasurement{OD: numberFrom(values, "odthickness", "odcct"), OS: numberFrom(values, "osthickness", "oscct")}
	if result.OD == nil && result.OS == nil {
		return nil
	}
	return result
}

func KeratometryValues(raw string) *Keratometry {
	values := DecodeObject(raw)
	result := &Keratometry{
		OD: KeratometryEye{K1: numberFrom(values, "odk1"), K1Axis: numberFrom(values, "odk1axis"), K2: numberFrom(values, "odk2"), K2Axis: numberFrom(values, "odk2axis")},
		OS: KeratometryEye{K1: numberFrom(values, "osk1"), K1Axis: numberFrom(values, "osk1axis"), K2: numberFrom(values, "osk2"), K2Axis: numberFrom(values, "osk2axis")},
	}
	if result.OD.K1 == nil && result.OD.K2 == nil && result.OS.K1 == nil && result.OS.K2 == nil {
		return nil
	}
	return result
}
