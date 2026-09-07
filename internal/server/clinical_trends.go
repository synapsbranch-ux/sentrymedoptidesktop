package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Clinical values are typed by staff into free-form JSON blobs (pretests.*_json,
// encounter_sections.data_json) using the "od"/"os" + field-name key convention built by
// the EyeGrid component. Nothing recomputes them, so acuity, pressure and refraction are
// captured for years without ever being comparable across visits. The helpers below turn
// those strings into numbers so trends, deltas and safety flags become possible without
// asking clinicians to enter anything new.

const (
	logMarLine          = 0.10 // one acuity line
	normalCorneaMicrons = 545.0
	thinCorneaMicrons   = 510.0
	thickCorneaMicrons  = 570.0
	elevatedIOP         = 21.0
	iopAsymmetryFlag    = 4.0
	fastProgressionDpt  = 0.75 // dioptres per year of myopic shift worth surfacing
	significantVALines  = 2.0
)

func normalizeNumeric(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "−", "-") // unicode minus sign
	value = strings.ReplaceAll(value, ",", ".")      // French decimal comma
	value = strings.TrimPrefix(value, "+")
	return value
}

// parseClinicalNumber reads a dioptre, millimetre-of-mercury or micron value.
func parseClinicalNumber(value string) (float64, bool) {
	normalized := normalizeNumeric(value)
	if normalized == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// snellenToLogMAR converts the acuity notations clinics actually type -- "20/40" and
// "6/12" (foot/metre Snellen), "5/10" (decimal fraction), "0.8" (decimal acuity), and the
// low-vision abbreviations CF/HM/LP/NLP -- into logMAR, where one line equals 0.10 and a
// higher number means worse vision. Fractions are the same formula in every convention,
// which is why 20/20, 6/6 and 10/10 all resolve to 0.00.
func snellenToLogMAR(value string) (float64, bool) {
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
	// Letter-count suffixes such as "20/40-2" describe partial lines; the base fraction is
	// what stays comparable across visits, so the suffix is dropped rather than guessed at.
	if index := strings.IndexAny(raw, "+-"); index > 0 && strings.Contains(raw[:index], "/") {
		raw = strings.TrimSpace(raw[:index])
	}
	if numerator, denominator, found := strings.Cut(raw, "/"); found {
		top, okTop := parseClinicalNumber(numerator)
		bottom, okBottom := parseClinicalNumber(denominator)
		if !okTop || !okBottom || top <= 0 || bottom <= 0 {
			return 0, false
		}
		return round2(math.Log10(bottom / top)), true
	}
	decimal, ok := parseClinicalNumber(raw)
	if !ok || decimal <= 0 {
		return 0, false
	}
	return round2(-math.Log10(decimal)), true
}

func round2(value float64) float64 { return math.Round(value*100) / 100 }

// sphericalEquivalent is the standard single number used to follow refractive change.
func sphericalEquivalent(sphere, cylinder float64, hasCylinder bool) float64 {
	if !hasCylinder {
		return round2(sphere)
	}
	return round2(sphere + cylinder/2)
}

type eyeMeasurement struct {
	OD      *float64 `json:"od"`
	OS      *float64 `json:"os"`
	ODLabel string   `json:"odLabel,omitempty"`
	OSLabel string   `json:"osLabel,omitempty"`
}

type refractionPoint struct {
	Source string   `json:"source"`
	ODSE   *float64 `json:"odSphericalEquivalent"`
	OSSE   *float64 `json:"osSphericalEquivalent"`
	ODCyl  *float64 `json:"odCylinder"`
	OSCyl  *float64 `json:"osCylinder"`
}

type trendPoint struct {
	EncounterID     string           `json:"encounterId"`
	EncounterNumber string           `json:"encounterNumber"`
	Date            string           `json:"date"`
	Refraction      *refractionPoint `json:"refraction,omitempty"`
	VisualAcuity    *eyeMeasurement  `json:"visualAcuity,omitempty"`
	IOP             *eyeMeasurement  `json:"iop,omitempty"`
	Pachymetry      *eyeMeasurement  `json:"pachymetry,omitempty"`
}

type clinicalAlert struct {
	Severity string `json:"severity"` // info | warning | danger
	Eye      string `json:"eye,omitempty"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

func decodeEyeMap(raw string) map[string]string {
	values := map[string]string{}
	if raw == "" {
		return values
	}
	_ = json.Unmarshal([]byte(raw), &values)
	return values
}

func numberFromMap(values map[string]string, keys ...string) *float64 {
	for _, key := range keys {
		if parsed, ok := parseClinicalNumber(values[key]); ok {
			rounded := round2(parsed)
			return &rounded
		}
	}
	return nil
}

// bestAcuity walks the acuity fields in clinical priority order: a best-corrected value
// describes the eye's potential, and only when it is missing do presenting and uncorrected
// values stand in. The label records which one was used so the chart never silently
// compares a corrected value against an uncorrected one.
func bestAcuity(values map[string]string, eye string) (*float64, string) {
	candidates := []struct{ key, label string }{
		{eye + "bestcorrectedva", "best corrected"},
		{eye + "correcteddistance", "corrected distance"},
		{eye + "presentingdistance", "presenting distance"},
		{eye + "uncorrecteddistance", "uncorrected distance"},
	}
	for _, candidate := range candidates {
		raw := strings.TrimSpace(values[candidate.key])
		if raw == "" {
			continue
		}
		if logMar, ok := snellenToLogMAR(raw); ok {
			return &logMar, raw + " (" + candidate.label + ")"
		}
	}
	return nil, ""
}

func refractionFromMap(values map[string]string, source string) *refractionPoint {
	odSphere, odHasSphere := parseClinicalNumber(values["odsphere"])
	osSphere, osHasSphere := parseClinicalNumber(values["ossphere"])
	if !odHasSphere && !osHasSphere {
		return nil
	}
	point := &refractionPoint{Source: source}
	odCyl, odHasCyl := parseClinicalNumber(values["odcylinder"])
	osCyl, osHasCyl := parseClinicalNumber(values["oscylinder"])
	if odHasSphere {
		value := sphericalEquivalent(odSphere, odCyl, odHasCyl)
		point.ODSE = &value
	}
	if osHasSphere {
		value := sphericalEquivalent(osSphere, osCyl, osHasCyl)
		point.OSSE = &value
	}
	if odHasCyl {
		rounded := round2(odCyl)
		point.ODCyl = &rounded
	}
	if osHasCyl {
		rounded := round2(osCyl)
		point.OSCyl = &rounded
	}
	return point
}

func (s *Server) buildTrendPoints(r *http.Request, patientID string) ([]trendPoint, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT e.id,e.encounter_number,e.created_at,
			COALESCE(p.visual_acuity_json,'{}'),COALESCE(p.autorefraction_json,'{}'),COALESCE(p.iop_json,'{}'),COALESCE(p.pachymetry_json,'{}'),
			COALESCE(s.data_json,'{}')
		FROM encounters e
		LEFT JOIN pretests p ON p.encounter_id=e.id
		LEFT JOIN encounter_sections s ON s.encounter_id=e.id AND s.section_type='subjective_refraction'
		WHERE e.patient_id=? AND e.archived_at IS NULL ORDER BY e.created_at`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []trendPoint{}
	for rows.Next() {
		var id, number, createdAt, acuityJSON, autorefractionJSON, iopJSON, pachymetryJSON, subjectiveJSON string
		if rows.Scan(&id, &number, &createdAt, &acuityJSON, &autorefractionJSON, &iopJSON, &pachymetryJSON, &subjectiveJSON) != nil {
			continue
		}
		point := trendPoint{EncounterID: id, EncounterNumber: number, Date: createdAt}

		// The doctor's subjective refraction is the clinical reference; autorefraction is
		// only a machine reading and stands in when the subjective one was never filled.
		if refraction := refractionFromMap(decodeEyeMap(subjectiveJSON), "subjective"); refraction != nil {
			point.Refraction = refraction
		} else if refraction := refractionFromMap(decodeEyeMap(autorefractionJSON), "autorefraction"); refraction != nil {
			point.Refraction = refraction
		}

		acuity := decodeEyeMap(acuityJSON)
		odVA, odLabel := bestAcuity(acuity, "od")
		osVA, osLabel := bestAcuity(acuity, "os")
		if odVA != nil || osVA != nil {
			point.VisualAcuity = &eyeMeasurement{OD: odVA, OS: osVA, ODLabel: odLabel, OSLabel: osLabel}
		}

		iop := decodeEyeMap(iopJSON)
		odIOP, osIOP := numberFromMap(iop, "odiop"), numberFromMap(iop, "osiop")
		if odIOP != nil || osIOP != nil {
			point.IOP = &eyeMeasurement{OD: odIOP, OS: osIOP}
		}

		pachymetry := decodeEyeMap(pachymetryJSON)
		odCCT, osCCT := numberFromMap(pachymetry, "odthickness"), numberFromMap(pachymetry, "osthickness")
		if odCCT != nil || osCCT != nil {
			point.Pachymetry = &eyeMeasurement{OD: odCCT, OS: osCCT}
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func parseVisitDate(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func formatDioptre(value float64) string { return fmt.Sprintf("%+.2f D", value) }

// progressionRate returns the yearly change of a series' first and last usable value.
func progressionRate(points []trendPoint, pick func(trendPoint) *float64) (rate float64, span float64, ok bool) {
	type sample struct {
		at    time.Time
		value float64
	}
	samples := []sample{}
	for _, point := range points {
		value := pick(point)
		if value == nil {
			continue
		}
		at, parsed := parseVisitDate(point.Date)
		if !parsed {
			continue
		}
		samples = append(samples, sample{at: at, value: *value})
	}
	if len(samples) < 2 {
		return 0, 0, false
	}
	first, last := samples[0], samples[len(samples)-1]
	years := last.at.Sub(first.at).Hours() / (24 * 365.25)
	if years < 0.25 { // under three months a rate would be extrapolated noise
		return 0, years, false
	}
	return round2((last.value - first.value) / years), years, true
}

func lastNonNil(points []trendPoint, pick func(trendPoint) *float64) (float64, string, bool) {
	for index := len(points) - 1; index >= 0; index-- {
		if value := pick(points[index]); value != nil {
			return *value, points[index].Date, true
		}
	}
	return 0, "", false
}

func peakValue(points []trendPoint, pick func(trendPoint) *float64) (float64, string, bool) {
	best, when, found := 0.0, "", false
	for _, point := range points {
		value := pick(point)
		if value == nil {
			continue
		}
		if !found || *value > best {
			best, when, found = *value, point.Date, true
		}
	}
	return best, when, found
}

func eyeSelector(eye string, pick func(trendPoint) *eyeMeasurement) func(trendPoint) *float64 {
	return func(point trendPoint) *float64 {
		measurement := pick(point)
		if measurement == nil {
			return nil
		}
		if eye == "OD" {
			return measurement.OD
		}
		return measurement.OS
	}
}

func refractionSelector(eye string) func(trendPoint) *float64 {
	return func(point trendPoint) *float64 {
		if point.Refraction == nil {
			return nil
		}
		if eye == "OD" {
			return point.Refraction.ODSE
		}
		return point.Refraction.OSSE
	}
}

func buildClinicalAlerts(points []trendPoint) []clinicalAlert {
	alerts := []clinicalAlert{}
	for _, eye := range []string{"OD", "OS"} {
		refraction := refractionSelector(eye)
		acuity := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.VisualAcuity })
		pressure := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.IOP })
		cornea := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.Pachymetry })

		if rate, years, ok := progressionRate(points, refraction); ok && rate <= -fastProgressionDpt {
			alerts = append(alerts, clinicalAlert{
				Severity: "warning", Eye: eye, Title: "Rapid myopic shift",
				Detail:   fmt.Sprintf("Spherical equivalent moved %s per year over %.1f year(s). Confirm clinically.", formatDioptre(rate), years),
			})
		}

		// Acuity is compared between the two most recent visits that actually recorded it.
		recent := []float64{}
		for index := len(points) - 1; index >= 0 && len(recent) < 2; index-- {
			if value := acuity(points[index]); value != nil {
				recent = append(recent, *value)
			}
		}
		if len(recent) == 2 {
			lines := (recent[0] - recent[1]) / logMarLine
			if lines >= significantVALines {
				alerts = append(alerts, clinicalAlert{
					Severity: "danger", Eye: eye, Title: "Visual acuity dropped",
					Detail:   fmt.Sprintf("%.0f line(s) lost since the previous recorded visit.", lines),
				})
			}
		}

		if value, _, ok := lastNonNil(points, pressure); ok && value > elevatedIOP {
			alerts = append(alerts, clinicalAlert{
				Severity: "warning", Eye: eye, Title: "Elevated intraocular pressure",
				Detail:   fmt.Sprintf("Latest reading %.0f mmHg (above %.0f mmHg).", value, elevatedIOP),
			})
		}
		if value, when, ok := peakValue(points, pressure); ok && value > elevatedIOP {
			alerts = append(alerts, clinicalAlert{
				Severity: "info", Eye: eye, Title: "Peak pressure on record",
				Detail:   fmt.Sprintf("Highest recorded %.0f mmHg on %s.", value, strings.SplitN(when, "T", 2)[0]),
			})
		}

		// Corneal thickness changes how a tonometer reading should be read, so the bias is
		// surfaced as a direction rather than silently rewriting the measured pressure.
		if value, _, ok := lastNonNil(points, cornea); ok {
			switch {
			case value < thinCorneaMicrons:
				alerts = append(alerts, clinicalAlert{
					Severity: "warning", Eye: eye, Title: "Thin cornea",
					Detail:   fmt.Sprintf("%.0f µm (below %.0f µm): measured IOP likely underestimates true pressure.", value, thinCorneaMicrons),
				})
			case value > thickCorneaMicrons:
				alerts = append(alerts, clinicalAlert{
					Severity: "info", Eye: eye, Title: "Thick cornea",
					Detail:   fmt.Sprintf("%.0f µm (above %.0f µm): measured IOP likely overestimates true pressure.", value, thickCorneaMicrons),
				})
			}
		}
	}

	if len(points) > 0 {
		latest := points[len(points)-1]
		if latest.IOP != nil && latest.IOP.OD != nil && latest.IOP.OS != nil {
			difference := math.Abs(*latest.IOP.OD - *latest.IOP.OS)
			if difference >= iopAsymmetryFlag {
				alerts = append(alerts, clinicalAlert{
					Severity: "warning", Title: "Asymmetric intraocular pressure",
					Detail:   fmt.Sprintf("%.0f mmHg difference between eyes at the latest visit.", difference),
				})
			}
		}
	}
	return alerts
}

func (s *Server) handleClinicalTrends(w http.ResponseWriter, r *http.Request) {
	patientID := chi.URLParam(r, "id")
	var exists int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM patients WHERE id=? AND archived_at IS NULL", patientID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, "PATIENT_NOT_FOUND", "Patient was not found.")
		return
	}
	points, err := s.buildTrendPoints(r, patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TRENDS_FAILED", "Could not build clinical trends.")
		return
	}
	summary := map[string]any{}
	for _, eye := range []string{"OD", "OS"} {
		if rate, years, ok := progressionRate(points, refractionSelector(eye)); ok {
			summary["refractionRate"+eye] = rate
			summary["refractionYears"+eye] = round2(years)
		}
		if value, when, ok := peakValue(points, eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.IOP })); ok {
			summary["iopPeak"+eye] = value
			summary["iopPeakDate"+eye] = when
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"patientId": patientID, "points": points, "alerts": buildClinicalAlerts(points), "summary": summary})
}

type visitChange struct {
	Category string `json:"category"`
	Eye      string `json:"eye,omitempty"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Delta    string `json:"delta"`
	Severity string `json:"severity"`
}

func (s *Server) diagnosesFor(r *http.Request, encounterID string) []string {
	labels := []string{}
	rows, err := s.db.QueryContext(r.Context(), "SELECT diagnosis,COALESCE(code,'') FROM diagnoses WHERE encounter_id=? ORDER BY is_primary DESC,created_at", encounterID)
	if err != nil {
		return labels
	}
	defer rows.Close()
	for rows.Next() {
		var diagnosis, code string
		if rows.Scan(&diagnosis, &code) == nil {
			if code != "" {
				diagnosis += " (" + code + ")"
			}
			labels = append(labels, diagnosis)
		}
	}
	return labels
}

// handleEncounterDelta answers the question a clinician asks first at every follow-up:
// what actually changed since the previous visit.
func (s *Server) handleEncounterDelta(w http.ResponseWriter, r *http.Request) {
	encounterID := chi.URLParam(r, "id")
	var patientID string
	if err := s.db.QueryRowContext(r.Context(), "SELECT patient_id FROM encounters WHERE id=? AND archived_at IS NULL", encounterID).Scan(&patientID); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	points, err := s.buildTrendPoints(r, patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DELTA_FAILED", "Could not compare consultations.")
		return
	}
	index := -1
	for position, point := range points {
		if point.EncounterID == encounterID {
			index = position
			break
		}
	}
	if index <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"hasPrevious": false, "changes": []visitChange{}})
		return
	}
	current, previous := points[index], points[index-1]
	changes := []visitChange{}

	for _, eye := range []string{"OD", "OS"} {
		acuity := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.VisualAcuity })
		before, after := acuity(previous), acuity(current)
		if before != nil && after != nil && *before != *after {
			lines := (*after - *before) / logMarLine
			severity := "info"
			if lines >= significantVALines {
				severity = "danger"
			}
			changes = append(changes, visitChange{
				Category: "Visual acuity", Eye: eye,
				Before:   fmt.Sprintf("%.2f logMAR", *before), After: fmt.Sprintf("%.2f logMAR", *after),
				Delta:    fmt.Sprintf("%+.0f line(s)", -lines), Severity: severity,
			})
		}

		pressure := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.IOP })
		beforeIOP, afterIOP := pressure(previous), pressure(current)
		if beforeIOP != nil && afterIOP != nil && *beforeIOP != *afterIOP {
			severity := "info"
			if *afterIOP > elevatedIOP {
				severity = "warning"
			}
			changes = append(changes, visitChange{
				Category: "Intraocular pressure", Eye: eye,
				Before:   fmt.Sprintf("%.0f mmHg", *beforeIOP), After: fmt.Sprintf("%.0f mmHg", *afterIOP),
				Delta:    fmt.Sprintf("%+.0f mmHg", *afterIOP-*beforeIOP), Severity: severity,
			})
		}

		refraction := refractionSelector(eye)
		beforeSE, afterSE := refraction(previous), refraction(current)
		if beforeSE != nil && afterSE != nil && *beforeSE != *afterSE {
			changes = append(changes, visitChange{
				Category: "Refraction (spherical equivalent)", Eye: eye,
				Before:   formatDioptre(*beforeSE), After: formatDioptre(*afterSE),
				Delta:    formatDioptre(*afterSE - *beforeSE), Severity: "info",
			})
		}
	}

	previousDiagnoses := map[string]bool{}
	for _, label := range s.diagnosesFor(r, previous.EncounterID) {
		previousDiagnoses[label] = true
	}
	for _, label := range s.diagnosesFor(r, current.EncounterID) {
		if !previousDiagnoses[label] {
			changes = append(changes, visitChange{Category: "Diagnosis", Before: "—", After: label, Delta: "new", Severity: "info"})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hasPrevious": true,
		"previous":    map[string]any{"encounterId": previous.EncounterID, "encounterNumber": previous.EncounterNumber, "date": previous.Date},
		"current":     map[string]any{"encounterId": current.EncounterID, "encounterNumber": current.EncounterNumber, "date": current.Date},
		"changes":     changes,
	})
}
