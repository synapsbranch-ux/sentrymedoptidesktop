package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/clinicalvalues"
)

// Clinical values are typed by staff into free-form JSON blobs (pretests.*_json,
// encounter_sections.data_json) using the "od"/"os" + field-name key convention built by
// the EyeGrid component. Nothing recomputes them, so acuity, pressure and refraction are
// captured for years without ever being comparable across visits. The helpers below turn
// those strings into numbers so trends, deltas and safety flags become possible without
// asking clinicians to enter anything new.

const (
	logMarLine = 0.10 // one acuity line
)

type clinicalAnalyticsSettings struct {
	RapidMyopicShiftDPerYear float64 `json:"rapidMyopicShiftDPerYear"`
	SignificantAcuityLoss    float64 `json:"significantAcuityLossLines"`
	ElevatedIOP              float64 `json:"elevatedIOPMmHg"`
	IOPAsymmetry             float64 `json:"iopAsymmetryMmHg"`
	ThinCornea               float64 `json:"thinCorneaMicrons"`
	ThickCornea              float64 `json:"thickCorneaMicrons"`
	CCTCorrectionEnabled     bool    `json:"cctCorrectionEnabled"`
	CCTReference             float64 `json:"cctReferenceMicrons"`
	CCTMicronsPerMmHg        float64 `json:"cctMicronsPerMmHg"`
}

func defaultClinicalAnalyticsSettings() clinicalAnalyticsSettings {
	return clinicalAnalyticsSettings{
		RapidMyopicShiftDPerYear: 0.75,
		SignificantAcuityLoss:    2,
		ElevatedIOP:              21,
		IOPAsymmetry:             4,
		ThinCornea:               510,
		ThickCornea:              570,
		CCTReference:             545,
	}
}

func (s *Server) clinicalAnalyticsSettings(ctx context.Context) clinicalAnalyticsSettings {
	settings := defaultClinicalAnalyticsSettings()
	var raw string
	if s.db.QueryRowContext(ctx, "SELECT COALESCE(json_extract(value_json,'$.analytics'),'{}') FROM settings WHERE key='clinical'").Scan(&raw) != nil {
		return settings
	}
	var configured clinicalAnalyticsSettings
	if json.Unmarshal([]byte(raw), &configured) != nil {
		return settings
	}
	if configured.RapidMyopicShiftDPerYear > 0 {
		settings.RapidMyopicShiftDPerYear = configured.RapidMyopicShiftDPerYear
	}
	if configured.SignificantAcuityLoss > 0 {
		settings.SignificantAcuityLoss = configured.SignificantAcuityLoss
	}
	if configured.ElevatedIOP > 0 {
		settings.ElevatedIOP = configured.ElevatedIOP
	}
	if configured.IOPAsymmetry > 0 {
		settings.IOPAsymmetry = configured.IOPAsymmetry
	}
	if configured.ThinCornea > 0 {
		settings.ThinCornea = configured.ThinCornea
	}
	if configured.ThickCornea > 0 {
		settings.ThickCornea = configured.ThickCornea
	}
	if configured.CCTReference > 0 {
		settings.CCTReference = configured.CCTReference
	}
	if configured.CCTMicronsPerMmHg > 0 {
		settings.CCTMicronsPerMmHg = configured.CCTMicronsPerMmHg
	}
	settings.CCTCorrectionEnabled = configured.CCTCorrectionEnabled && settings.CCTMicronsPerMmHg > 0
	return settings
}

func parseClinicalNumber(value string) (float64, bool) {
	return clinicalvalues.Number(value)
}

// snellenToLogMAR converts the acuity notations clinics actually type -- "20/40" and
// "6/12" (foot/metre Snellen), "5/10" (decimal fraction), "0.8" (decimal acuity), and the
// low-vision abbreviations CF/HM/LP/NLP -- into logMAR, where one line equals 0.10 and a
// higher number means worse vision. Fractions are the same formula in every convention,
// which is why 20/20, 6/6 and 10/10 all resolve to 0.00.
func snellenToLogMAR(value string) (float64, bool) {
	return clinicalvalues.SnellenToLogMAR(value)
}

func round2(value float64) float64 { return clinicalvalues.Round2(value) }

// sphericalEquivalent is the standard single number used to follow refractive change.
func sphericalEquivalent(sphere, cylinder float64, hasCylinder bool) float64 {
	return clinicalvalues.SphericalEquivalent(sphere, cylinder, hasCylinder)
}

type eyeMeasurement = clinicalvalues.EyeMeasurement

type refractionPoint struct {
	Source   string   `json:"source"`
	ODSphere *float64 `json:"odSphere"`
	OSSphere *float64 `json:"osSphere"`
	ODSE     *float64 `json:"odSphericalEquivalent"`
	OSSE     *float64 `json:"osSphericalEquivalent"`
	ODCyl    *float64 `json:"odCylinder"`
	OSCyl    *float64 `json:"osCylinder"`
	ODAxis   *float64 `json:"odAxis"`
	OSAxis   *float64 `json:"osAxis"`
}

type trendPoint struct {
	EncounterID     string                      `json:"encounterId"`
	EncounterNumber string                      `json:"encounterNumber"`
	Date            string                      `json:"date"`
	Refraction      *refractionPoint            `json:"refraction,omitempty"`
	Refractions     map[string]*refractionPoint `json:"refractions,omitempty"`
	VisualAcuity    *eyeMeasurement             `json:"visualAcuity,omitempty"`
	IOP             *eyeMeasurement             `json:"iop,omitempty"`
	Pachymetry      *eyeMeasurement             `json:"pachymetry,omitempty"`
	Keratometry     *clinicalvalues.Keratometry `json:"keratometry,omitempty"`
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
	raw, _ := json.Marshal(values)
	measurement := clinicalvalues.VisualAcuity(string(raw))
	if measurement == nil {
		return nil, ""
	}
	if eye == "od" {
		return measurement.OD, measurement.ODLabel
	}
	return measurement.OS, measurement.OSLabel
}

func refractionPointFrom(value *clinicalvalues.Refraction) *refractionPoint {
	if value == nil {
		return nil
	}
	return &refractionPoint{
		Source:   value.Source,
		ODSphere: value.OD.Sphere, OSSphere: value.OS.Sphere,
		ODSE: value.OD.SphericalEquivalent, OSSE: value.OS.SphericalEquivalent,
		ODCyl: value.OD.Cylinder, OSCyl: value.OS.Cylinder,
		ODAxis: value.OD.Axis, OSAxis: value.OS.Axis,
	}
}

func refractionFromMap(values map[string]string, source string) *refractionPoint {
	raw, _ := json.Marshal(values)
	return refractionPointFrom(clinicalvalues.RefractionFromCombined(string(raw), source))
}

func (s *Server) buildTrendPoints(r *http.Request, patientID string) ([]trendPoint, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT e.id,e.encounter_number,e.created_at,
			COALESCE(p.visual_acuity_json,'{}'),COALESCE(p.autorefraction_json,'{}'),COALESCE(p.iop_json,'{}'),COALESCE(p.pachymetry_json,'{}'),COALESCE(p.keratometry_json,'{}'),
			COALESCE(s.data_json,'{}'),COALESCE(rx.od_json,'{}'),COALESCE(rx.os_json,'{}')
		FROM encounters e
		LEFT JOIN pretests p ON p.encounter_id=e.id
		LEFT JOIN encounter_sections s ON s.encounter_id=e.id AND s.section_type='subjective_refraction'
		LEFT JOIN prescriptions rx ON rx.id=(
			SELECT candidate.id FROM prescriptions candidate
			WHERE candidate.encounter_id=e.id AND candidate.type='spectacle' AND candidate.status='final' AND candidate.archived_at IS NULL
			ORDER BY candidate.issued_at DESC LIMIT 1
		)
		WHERE e.patient_id=? AND e.archived_at IS NULL ORDER BY e.created_at`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []trendPoint{}
	for rows.Next() {
		var id, number, createdAt, acuityJSON, autorefractionJSON, iopJSON, pachymetryJSON, keratometryJSON, subjectiveJSON, prescriptionODJSON, prescriptionOSJSON string
		if rows.Scan(&id, &number, &createdAt, &acuityJSON, &autorefractionJSON, &iopJSON, &pachymetryJSON, &keratometryJSON, &subjectiveJSON, &prescriptionODJSON, &prescriptionOSJSON) != nil {
			continue
		}
		point := trendPoint{EncounterID: id, EncounterNumber: number, Date: createdAt, Refractions: map[string]*refractionPoint{}}

		if refraction := refractionPointFrom(clinicalvalues.RefractionFromCombined(subjectiveJSON, "subjective")); refraction != nil {
			point.Refractions["subjective"] = refraction
		}
		if refraction := refractionPointFrom(clinicalvalues.RefractionFromEyes(prescriptionODJSON, prescriptionOSJSON, "prescription")); refraction != nil {
			point.Refractions["prescription"] = refraction
		}
		if refraction := refractionPointFrom(clinicalvalues.RefractionFromCombined(autorefractionJSON, "autorefraction")); refraction != nil {
			point.Refractions["autorefraction"] = refraction
		}
		for _, source := range []string{"subjective", "prescription", "autorefraction"} {
			if point.Refractions[source] != nil {
				point.Refraction = point.Refractions[source]
				break
			}
		}
		if len(point.Refractions) == 0 {
			point.Refractions = nil
		}

		point.VisualAcuity = clinicalvalues.VisualAcuity(acuityJSON)
		point.IOP = clinicalvalues.IOP(iopJSON)
		point.Pachymetry = clinicalvalues.Pachymetry(pachymetryJSON)
		point.Keratometry = clinicalvalues.KeratometryValues(keratometryJSON)
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

func refractionSourceSelector(eye, source string, pick func(*refractionPoint) *float64) func(trendPoint) *float64 {
	return func(point trendPoint) *float64 {
		refraction := point.Refractions[source]
		if refraction == nil {
			return nil
		}
		if pick != nil {
			return pick(refraction)
		}
		if eye == "OD" {
			return refraction.ODSE
		}
		return refraction.OSSE
	}
}

func progressionSource(points []trendPoint, eye string) (string, func(trendPoint) *float64) {
	for _, source := range []string{"subjective", "prescription", "autorefraction"} {
		selector := refractionSourceSelector(eye, source, nil)
		count := 0
		for _, point := range points {
			if selector(point) != nil {
				count++
			}
		}
		if count >= 2 {
			return source, selector
		}
	}
	return "", refractionSelector(eye)
}

type correctedIOP struct {
	Measured       *float64 `json:"measured,omitempty"`
	Pachymetry     *float64 `json:"pachymetry,omitempty"`
	Estimated      *float64 `json:"estimated,omitempty"`
	Interpretation string   `json:"interpretation"`
}

func iopContext(measured, pachymetry *float64, settings clinicalAnalyticsSettings) *correctedIOP {
	if measured == nil {
		return nil
	}
	result := &correctedIOP{Measured: measured, Pachymetry: pachymetry, Interpretation: "No pachymetry recorded; measured IOP is unchanged."}
	if pachymetry == nil {
		return result
	}
	switch {
	case *pachymetry < settings.ThinCornea:
		result.Interpretation = "Thin cornea: the measured IOP may underestimate pressure."
	case *pachymetry > settings.ThickCornea:
		result.Interpretation = "Thick cornea: the measured IOP may overestimate pressure."
	default:
		result.Interpretation = "Pachymetry is within the configured reference band."
	}
	if settings.CCTCorrectionEnabled && settings.CCTMicronsPerMmHg > 0 {
		estimate := round2(*measured + (settings.CCTReference-*pachymetry)/settings.CCTMicronsPerMmHg)
		result.Estimated = &estimate
		result.Interpretation += " The displayed estimate uses the clinic-configured coefficient and is not a replacement measurement."
	}
	return result
}

func buildClinicalAlerts(points []trendPoint) []clinicalAlert {
	return buildClinicalAlertsWithSettings(points, defaultClinicalAnalyticsSettings())
}

func buildClinicalAlertsWithSettings(points []trendPoint, settings clinicalAnalyticsSettings) []clinicalAlert {
	alerts := []clinicalAlert{}
	for _, eye := range []string{"OD", "OS"} {
		source, refraction := progressionSource(points, eye)
		acuity := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.VisualAcuity })
		pressure := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.IOP })
		cornea := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.Pachymetry })

		if rate, years, ok := progressionRate(points, refraction); ok && rate <= -settings.RapidMyopicShiftDPerYear {
			alerts = append(alerts, clinicalAlert{
				Severity: "warning", Eye: eye, Title: "Rapid myopic shift",
				Detail: fmt.Sprintf("%s spherical equivalent moved %s per year over %.1f year(s). Confirm clinically.", source, formatDioptre(rate), years),
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
			if lines >= settings.SignificantAcuityLoss {
				alerts = append(alerts, clinicalAlert{
					Severity: "danger", Eye: eye, Title: "Visual acuity dropped",
					Detail: fmt.Sprintf("%.0f line(s) lost since the previous recorded visit.", lines),
				})
			}
		}

		if value, _, ok := lastNonNil(points, pressure); ok && value > settings.ElevatedIOP {
			alerts = append(alerts, clinicalAlert{
				Severity: "warning", Eye: eye, Title: "Elevated intraocular pressure",
				Detail: fmt.Sprintf("Latest reading %.0f mmHg (above %.0f mmHg).", value, settings.ElevatedIOP),
			})
		}
		if value, when, ok := peakValue(points, pressure); ok && value > settings.ElevatedIOP {
			alerts = append(alerts, clinicalAlert{
				Severity: "info", Eye: eye, Title: "Peak pressure on record",
				Detail: fmt.Sprintf("Highest recorded %.0f mmHg on %s.", value, strings.SplitN(when, "T", 2)[0]),
			})
		}

		// Corneal thickness changes how a tonometer reading should be read, so the bias is
		// surfaced as a direction rather than silently rewriting the measured pressure.
		if value, _, ok := lastNonNil(points, cornea); ok {
			switch {
			case value < settings.ThinCornea:
				alerts = append(alerts, clinicalAlert{
					Severity: "warning", Eye: eye, Title: "Thin cornea",
					Detail: fmt.Sprintf("%.0f µm (below %.0f µm): measured IOP may underestimate pressure.", value, settings.ThinCornea),
				})
			case value > settings.ThickCornea:
				alerts = append(alerts, clinicalAlert{
					Severity: "info", Eye: eye, Title: "Thick cornea",
					Detail: fmt.Sprintf("%.0f µm (above %.0f µm): measured IOP may overestimate pressure.", value, settings.ThickCornea),
				})
			}
		}
	}

	if len(points) > 0 {
		latest := points[len(points)-1]
		if latest.IOP != nil && latest.IOP.OD != nil && latest.IOP.OS != nil {
			difference := math.Abs(*latest.IOP.OD - *latest.IOP.OS)
			if difference >= settings.IOPAsymmetry {
				alerts = append(alerts, clinicalAlert{
					Severity: "warning", Title: "Asymmetric intraocular pressure",
					Detail: fmt.Sprintf("%.0f mmHg difference between eyes at the latest visit.", difference),
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
	settings := s.clinicalAnalyticsSettings(r.Context())
	summary := map[string]any{}
	for _, eye := range []string{"OD", "OS"} {
		source, selector := progressionSource(points, eye)
		if rate, years, ok := progressionRate(points, selector); ok {
			summary["refractionRate"+eye] = rate
			summary["refractionYears"+eye] = round2(years)
			summary["refractionSource"+eye] = source
		}
		if value, when, ok := peakValue(points, eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.IOP })); ok {
			summary["iopPeak"+eye] = value
			summary["iopPeakDate"+eye] = when
		}
	}
	if len(points) > 0 {
		latest := points[len(points)-1]
		var odIOP, osIOP, odPachymetry, osPachymetry *float64
		if latest.IOP != nil {
			odIOP, osIOP = latest.IOP.OD, latest.IOP.OS
		}
		if latest.Pachymetry != nil {
			odPachymetry, osPachymetry = latest.Pachymetry.OD, latest.Pachymetry.OS
		}
		summary["iopContext"] = map[string]any{
			"od":                iopContext(odIOP, odPachymetry, settings),
			"os":                iopContext(osIOP, osPachymetry, settings),
			"correctionEnabled": settings.CCTCorrectionEnabled,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"patientId": patientID, "points": points, "alerts": buildClinicalAlertsWithSettings(points, settings), "summary": summary, "thresholds": settings})
}

type visitChange struct {
	Category   string `json:"category"`
	ChangeType string `json:"changeType"`
	Eye        string `json:"eye,omitempty"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Delta      string `json:"delta"`
	Severity   string `json:"severity"`
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

func (s *Server) medicationsFor(r *http.Request, encounterID string) []string {
	labels := []string{}
	rows, err := s.db.QueryContext(r.Context(), `SELECT details_json FROM prescriptions
		WHERE encounter_id=? AND type='medication' AND status='final' AND archived_at IS NULL ORDER BY issued_at`, encounterID)
	if err != nil {
		return labels
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var details map[string]any
		if json.Unmarshal([]byte(raw), &details) != nil {
			continue
		}
		parts := []string{}
		for _, key := range []string{"medication", "strength", "dosage", "frequency", "route", "duration"} {
			if value := strings.TrimSpace(fmt.Sprint(details[key])); value != "" && value != "<nil>" {
				parts = append(parts, value)
			}
		}
		if len(parts) > 0 {
			labels = append(labels, strings.Join(parts, " · "))
		}
	}
	return labels
}

func (s *Server) treatmentPlanFor(r *http.Request, encounterID string) string {
	var value string
	_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(treatment_plan,'') FROM encounters WHERE id=?", encounterID).Scan(&value)
	return strings.TrimSpace(value)
}

func appendSetChanges(changes []visitChange, category string, before, after []string) []visitChange {
	beforeSet, afterSet := map[string]bool{}, map[string]bool{}
	for _, value := range before {
		beforeSet[value] = true
	}
	for _, value := range after {
		afterSet[value] = true
	}
	for _, value := range after {
		if !beforeSet[value] {
			changes = append(changes, visitChange{Category: category, ChangeType: "added", Before: "—", After: value, Delta: "added", Severity: "info"})
		}
	}
	for _, value := range before {
		if !afterSet[value] {
			changes = append(changes, visitChange{Category: category, ChangeType: "removed", Before: value, After: "—", Delta: "removed", Severity: "warning"})
		}
	}
	return changes
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func commonRefractionSource(current, previous trendPoint, eye string) string {
	for _, source := range []string{"subjective", "prescription", "autorefraction"} {
		currentValue, previousValue := current.Refractions[source], previous.Refractions[source]
		if currentValue == nil || previousValue == nil {
			continue
		}
		if eye == "OD" && currentValue.ODSE != nil && previousValue.ODSE != nil {
			return source
		}
		if eye == "OS" && currentValue.OSSE != nil && previousValue.OSSE != nil {
			return source
		}
	}
	return ""
}

func refractionComponent(point trendPoint, source, eye, component string) *float64 {
	value := point.Refractions[source]
	if value == nil {
		return nil
	}
	if eye == "OD" {
		switch component {
		case "Sphere":
			return value.ODSphere
		case "Cylinder":
			return value.ODCyl
		case "Axis":
			return value.ODAxis
		default:
			return value.ODSE
		}
	}
	switch component {
	case "Sphere":
		return value.OSSphere
	case "Cylinder":
		return value.OSCyl
	case "Axis":
		return value.OSAxis
	default:
		return value.OSSE
	}
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
	settings := s.clinicalAnalyticsSettings(r.Context())

	for _, eye := range []string{"OD", "OS"} {
		acuity := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.VisualAcuity })
		before, after := acuity(previous), acuity(current)
		if before != nil && after != nil && *before != *after {
			lines := (*after - *before) / logMarLine
			severity := "info"
			if lines >= settings.SignificantAcuityLoss {
				severity = "danger"
			}
			changes = append(changes, visitChange{
				Category: "Visual acuity", ChangeType: "modified", Eye: eye,
				Before: fmt.Sprintf("%.2f logMAR", *before), After: fmt.Sprintf("%.2f logMAR", *after),
				Delta: fmt.Sprintf("%+.0f line(s)", -lines), Severity: severity,
			})
		}

		pressure := eyeSelector(eye, func(point trendPoint) *eyeMeasurement { return point.IOP })
		beforeIOP, afterIOP := pressure(previous), pressure(current)
		if beforeIOP != nil && afterIOP != nil && *beforeIOP != *afterIOP {
			severity := "info"
			if *afterIOP > settings.ElevatedIOP {
				severity = "warning"
			}
			changes = append(changes, visitChange{
				Category: "Intraocular pressure", ChangeType: "modified", Eye: eye,
				Before: fmt.Sprintf("%.0f mmHg", *beforeIOP), After: fmt.Sprintf("%.0f mmHg", *afterIOP),
				Delta: fmt.Sprintf("%+.0f mmHg", *afterIOP-*beforeIOP), Severity: severity,
			})
		}

		source := commonRefractionSource(current, previous, eye)
		if source != "" {
			for _, component := range []string{"Sphere", "Cylinder", "Axis", "Spherical equivalent"} {
				beforeValue, afterValue := refractionComponent(previous, source, eye, component), refractionComponent(current, source, eye, component)
				if beforeValue == nil || afterValue == nil || *beforeValue == *afterValue {
					continue
				}
				unit := "D"
				if component == "Axis" {
					unit = "°"
				}
				category := "Refraction · " + component + " (" + source + ")"
				if component == "Spherical equivalent" {
					category = "Refraction (spherical equivalent)"
				}
				changes = append(changes, visitChange{
					Category: category, ChangeType: "modified", Eye: eye,
					Before: fmt.Sprintf("%+.2f %s", *beforeValue, unit), After: fmt.Sprintf("%+.2f %s", *afterValue, unit),
					Delta: fmt.Sprintf("%+.2f %s", *afterValue-*beforeValue, unit), Severity: "info",
				})
			}
		}
	}

	changes = appendSetChanges(changes, "Diagnosis", s.diagnosesFor(r, previous.EncounterID), s.diagnosesFor(r, current.EncounterID))
	changes = appendSetChanges(changes, "Medication", s.medicationsFor(r, previous.EncounterID), s.medicationsFor(r, current.EncounterID))
	previousPlan, currentPlan := s.treatmentPlanFor(r, previous.EncounterID), s.treatmentPlanFor(r, current.EncounterID)
	if previousPlan != currentPlan {
		changeType := "modified"
		if previousPlan == "" {
			changeType = "added"
		}
		if currentPlan == "" {
			changeType = "removed"
		}
		changes = append(changes, visitChange{Category: "Treatment plan", ChangeType: changeType, Before: valueOrDash(previousPlan), After: valueOrDash(currentPlan), Delta: changeType, Severity: "info"})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hasPrevious": true,
		"previous":    map[string]any{"encounterId": previous.EncounterID, "encounterNumber": previous.EncounterNumber, "date": previous.Date},
		"current":     map[string]any{"encounterId": current.EncounterID, "encounterNumber": current.EncounterNumber, "date": current.Date},
		"changes":     changes,
	})
}
