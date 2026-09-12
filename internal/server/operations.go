package server

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) registerOperationsRoutes(r chi.Router) {
	r.Get("/inventory", s.handleInventoryList)
	r.Post("/inventory", s.handleInventoryCreate)
	r.Post("/inventory/{id}/movements", s.handleStockMovement)
	r.Get("/inventory/{id}/movements", s.handleStockMovementsList)
	r.Get("/suppliers", s.handleSuppliersList)
	r.With(s.requireDoctor).Post("/suppliers", s.handleSupplierCreate)
	r.With(s.requireDoctor).Put("/suppliers/{id}", s.handleSupplierUpdate)
	r.With(s.requireDoctor).Delete("/suppliers/{id}", s.handleSupplierArchive)
	r.With(s.requireDoctor).Get("/purchase-orders", s.handlePurchaseOrdersList)
	r.With(s.requireDoctor).Get("/purchase-orders/{id}", s.handlePurchaseOrderGet)
	r.With(s.requireDoctor).Post("/purchase-orders", s.handlePurchaseOrderCreate)
	r.With(s.requireDoctor).Patch("/purchase-orders/{id}/status", s.handlePurchaseOrderStatus)
	r.With(s.requireDoctor).Post("/purchase-orders/{id}/receive", s.handlePurchaseOrderReceive)
	r.Get("/stock-takes", s.handleStockTakesList)
	r.Get("/stock-takes/{id}", s.handleStockTakeGet)
	r.With(s.requireDoctor).Post("/stock-takes", s.handleStockTakeCreate)
	r.Put("/stock-takes/{id}/items/{itemID}", s.handleStockTakeCount)
	r.With(s.requireDoctor).Post("/stock-takes/{id}/finalize", s.handleStockTakeFinalize)
	r.With(s.requireDoctor).Post("/stock-takes/{id}/cancel", s.handleStockTakeCancel)
	r.Get("/invoices", s.handleInvoicesList)
	r.Get("/invoices/{id}", s.handleInvoiceGet)
	r.Post("/invoices", s.handleInvoiceCreate)
	r.Post("/invoices/{id}/payments", s.handlePaymentCreate)
	r.With(s.requireDoctor).Post("/payments/{id}/refunds", s.handleRefundCreate)
	r.Post("/pos/checkout", s.handlePOSCheckout)
	s.registerPOSRoutes(r)
	s.registerIncomeRoutes(r)
	s.registerImageRoutes(r)
	s.registerHumanResourcesRoutes(r)
	r.Get("/payment-methods", s.handlePaymentMethodsList)
	r.Get("/cash-register", s.handleCashRegisterCurrent)
	r.Post("/cash-register/open", s.handleCashRegisterOpen)
	r.Post("/cash-register/{id}/close", s.handleCashRegisterClose)
	r.Get("/lab-orders", s.handleLabOrdersList)
	r.Post("/lab-orders", s.handleLabOrderCreate)
	r.Patch("/lab-orders/{id}/status", s.handleLabOrderStatus)
	r.Post("/lab-orders/bulk-status", s.handleLabOrdersBulkStatus)
	r.Get("/lab-orders/requisition", s.handleLabRequisitionBatch)
	// Registered after the literal path above; chi matches a static segment
	// before a parameter, so "requisition" is never read as an order id.
	r.Get("/lab-orders/{id}", s.handleLabOrderDocument)
	r.Post("/lab-orders/{id}/quality-control", s.handleLabQualityControl)
	r.With(s.requireDoctor).Get("/expenses", s.handleExpensesList)
	r.With(s.requireDoctor).Post("/expenses", s.handleExpenseCreate)
	r.With(s.requireDoctor).Get("/finance/summary", s.handleFinanceSummary)
	r.With(s.requireDoctor).Get("/reports/{report}", s.handleReport)
	r.Get("/insurance/payers", s.handlePayersList)
	r.Get("/insurance/payers/{id}", s.handlePayerGet)
	r.With(s.requireDoctor).Post("/insurance/payers", s.handlePayerCreate)
	r.With(s.requireDoctor).Put("/insurance/payers/{id}", s.handlePayerUpdate)
	r.Get("/insurance/claims", s.handleClaimsList)
	r.Get("/insurance/claims/summary", s.handleClaimsSummary)
	r.Get("/insurance/claims/proposal", s.handleClaimProposal)
	r.Get("/insurance/claims/{id}", s.handleClaimGet)
	r.Get("/insurance/policies", s.handlePatientPoliciesList)
	r.With(s.requireDoctor).Post("/patients/{patientId}/insurance", s.handlePatientPolicySave)
	r.With(s.requireDoctor).Put("/patients/{patientId}/insurance/{id}", s.handlePatientPolicyUpdate)
	r.With(s.requireDoctor).Post("/patients/{patientId}/insurance/{id}/set-primary", s.handlePatientPolicySetPrimary)
	r.With(s.requireDoctor).Post("/patients/{patientId}/insurance/{id}/deactivate", s.handlePatientPolicyDeactivate)
	r.With(s.requireDoctor).Post("/patients/{patientId}/insurance/{id}/reactivate", s.handlePatientPolicyReactivate)
	r.With(s.requireDoctor).Post("/patients/{patientId}/insurance/{id}/verify", s.handlePatientPolicyVerify)
	r.With(s.requireDoctor).Delete("/patients/{patientId}/insurance/{id}", s.handlePatientPolicyDelete)
	s.registerInsuranceDocumentRoutes(r)
	r.Post("/insurance/claims", s.handleClaimCreate)
	r.With(s.requireDoctor).Put("/insurance/claims/{id}", s.handleClaimUpdate)
	r.With(s.requireDoctor).Patch("/insurance/claims/{id}/status", s.handleClaimStatus)
	r.With(s.requireDoctor).Post("/insurance/claims/{id}/payments", s.handleClaimPayment)
	r.Get("/operations/analytics", s.handleOperationsAnalytics)
}

type queueStageAverage struct {
	Minutes float64
	Samples int
}

type stageMetric struct {
	Stage          string  `json:"stage"`
	Samples        int     `json:"samples"`
	AverageMinutes float64 `json:"averageMinutes"`
	CurrentCount   int     `json:"currentCount"`
}

type heatmapCell struct {
	Weekday             int     `json:"weekday"`
	Hour                int     `json:"hour"`
	Arrivals            int     `json:"arrivals"`
	AverageCycleMinutes float64 `json:"averageCycleMinutes"`
}

type durationSummary struct {
	MatchedAppointments    int     `json:"matchedAppointments"`
	PlannedAverageMinutes  float64 `json:"plannedAverageMinutes"`
	ActualAverageMinutes   float64 `json:"actualAverageMinutes"`
	VarianceAverageMinutes float64 `json:"varianceAverageMinutes"`
	WithinTolerancePercent float64 `json:"withinTolerancePercent"`
}

type noShowSlot struct {
	Weekday int     `json:"weekday"`
	Hour    int     `json:"hour"`
	Total   int     `json:"total"`
	NoShows int     `json:"noShows"`
	Rate    float64 `json:"rate"`
}

type noShowPatient struct {
	PatientID           string  `json:"patientId"`
	MedicalRecordNumber string  `json:"medicalRecordNumber"`
	PatientName         string  `json:"patientName"`
	Total               int     `json:"total"`
	NoShows             int     `json:"noShows"`
	Rate                float64 `json:"rate"`
}

type operationsAnalytics struct {
	PeriodDays     int             `json:"periodDays"`
	Stages         []stageMetric   `json:"stages"`
	Heatmap        []heatmapCell   `json:"heatmap"`
	Duration       durationSummary `json:"duration"`
	NoShowSlots    []noShowSlot    `json:"noShowSlots"`
	NoShowPatients []noShowPatient `json:"noShowPatients"`
}

func roundMetric(value float64) float64 {
	return math.Round(value*10) / 10
}

func (s *Server) queueStageAverages(ctx context.Context) (map[string]queueStageAverage, error) {
	cutoff := time.Now().UTC().AddDate(0, -3, 0).Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `SELECT stage,COUNT(*),AVG((julianday(exited_at)-julianday(entered_at))*1440.0)
		FROM queue_stage_events WHERE entered_at>=? AND exited_at IS NOT NULL AND stage<>'completed'
		GROUP BY stage`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	averages := map[string]queueStageAverage{}
	for rows.Next() {
		var stage string
		var samples int
		var minutes sql.NullFloat64
		if err := rows.Scan(&stage, &samples, &minutes); err != nil {
			return nil, err
		}
		if minutes.Valid && samples >= 3 {
			averages[stage] = queueStageAverage{Minutes: minutes.Float64, Samples: samples}
		}
	}
	return averages, rows.Err()
}

func estimatedQueueWait(stage, enteredAt string, averages map[string]queueStageAverage) (*int, int) {
	lookup := stage
	if lookup == "checked_in" {
		lookup = "waiting_nurse"
	}
	if lookup != "waiting_nurse" && lookup != "waiting_doctor" {
		return nil, 0
	}
	average, ok := averages[lookup]
	if !ok {
		return nil, 0
	}
	entered, err := time.Parse(time.RFC3339Nano, enteredAt)
	if err != nil {
		return nil, 0
	}
	remaining := int(math.Ceil(average.Minutes - time.Since(entered).Minutes()))
	if remaining < 0 {
		remaining = 0
	}
	return &remaining, average.Samples
}

func (s *Server) handleOperationsAnalytics(w http.ResponseWriter, r *http.Request) {
	days := 28
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 7 || parsed > 365 {
			writeError(w, http.StatusBadRequest, "INVALID_PERIOD", "Analytics period must be between 7 and 365 days.")
			return
		}
		days = parsed
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339Nano)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := operationsAnalytics{PeriodDays: days, Stages: []stageMetric{}, Heatmap: []heatmapCell{}, NoShowSlots: []noShowSlot{}, NoShowPatients: []noShowPatient{}}
	_, clinic, configurationErr := s.publicDisplayConfiguration(r.Context())
	location, locationErr := time.LoadLocation(clinic.Timezone)
	if configurationErr != nil || locationErr != nil {
		location = time.UTC
	}

	stageRows, err := s.db.QueryContext(r.Context(), `SELECT h.stage,h.samples,h.average_minutes,COALESCE(c.current_count,0)
		FROM (SELECT stage,COUNT(*) samples,AVG((julianday(exited_at)-julianday(entered_at))*1440.0) average_minutes
			FROM queue_stage_events WHERE entered_at>=? AND exited_at IS NOT NULL AND stage<>'completed' GROUP BY stage) h
		LEFT JOIN (SELECT stage,COUNT(*) current_count FROM queue_entries WHERE completed_at IS NULL GROUP BY stage) c ON c.stage=h.stage
		UNION ALL
		SELECT c.stage,0,0,c.current_count FROM (SELECT stage,COUNT(*) current_count FROM queue_entries WHERE completed_at IS NULL GROUP BY stage) c
		WHERE NOT EXISTS (SELECT 1 FROM queue_stage_events h WHERE h.stage=c.stage AND h.entered_at>=? AND h.exited_at IS NOT NULL)
		ORDER BY average_minutes DESC`, cutoff, cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not load queue stage metrics.")
		return
	}
	for stageRows.Next() {
		var item stageMetric
		if err := stageRows.Scan(&item.Stage, &item.Samples, &item.AverageMinutes, &item.CurrentCount); err != nil {
			stageRows.Close()
			writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not read queue stage metrics.")
			return
		}
		item.AverageMinutes = roundMetric(item.AverageMinutes)
		result.Stages = append(result.Stages, item)
	}
	stageRows.Close()

	heatRows, err := s.db.QueryContext(r.Context(), `SELECT arrived_at,completed_at FROM queue_entries WHERE arrived_at>=? AND completed_at IS NOT NULL`, cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not load arrival heatmap.")
		return
	}
	type heatAggregate struct {
		arrivals int
		minutes  float64
	}
	heatAggregates := map[[2]int]heatAggregate{}
	for heatRows.Next() {
		var arrivedRaw, completedRaw string
		if err := heatRows.Scan(&arrivedRaw, &completedRaw); err != nil {
			heatRows.Close()
			writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not read arrival heatmap.")
			return
		}
		arrived, arrivedErr := time.Parse(time.RFC3339Nano, arrivedRaw)
		completed, completedErr := time.Parse(time.RFC3339Nano, completedRaw)
		if arrivedErr != nil || completedErr != nil || completed.Before(arrived) {
			continue
		}
		local := arrived.In(location)
		key := [2]int{int(local.Weekday()), local.Hour()}
		aggregate := heatAggregates[key]
		aggregate.arrivals++
		aggregate.minutes += completed.Sub(arrived).Minutes()
		heatAggregates[key] = aggregate
	}
	heatRows.Close()
	for key, aggregate := range heatAggregates {
		result.Heatmap = append(result.Heatmap, heatmapCell{Weekday: key[0], Hour: key[1], Arrivals: aggregate.arrivals, AverageCycleMinutes: roundMetric(aggregate.minutes / float64(aggregate.arrivals))})
	}
	sort.Slice(result.Heatmap, func(i, j int) bool {
		if result.Heatmap[i].Weekday == result.Heatmap[j].Weekday {
			return result.Heatmap[i].Hour < result.Heatmap[j].Hour
		}
		return result.Heatmap[i].Weekday < result.Heatmap[j].Weekday
	})

	durationRows, err := s.db.QueryContext(r.Context(), `SELECT a.duration_minutes,SUM((julianday(e.exited_at)-julianday(e.entered_at))*1440.0)
		FROM appointments a JOIN queue_entries q ON q.appointment_id=a.id JOIN queue_stage_events e ON e.queue_entry_id=q.id
		WHERE a.starts_at>=? AND e.stage='in_consultation' AND e.exited_at IS NOT NULL AND a.archived_at IS NULL
		GROUP BY a.id,a.duration_minutes`, cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not load appointment duration metrics.")
		return
	}
	var plannedTotal, actualTotal float64
	var withinTolerance int
	for durationRows.Next() {
		var planned int
		var actual float64
		if err := durationRows.Scan(&planned, &actual); err != nil {
			durationRows.Close()
			writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not read appointment duration metrics.")
			return
		}
		result.Duration.MatchedAppointments++
		plannedTotal += float64(planned)
		actualTotal += actual
		if math.Abs(actual-float64(planned)) <= 10 {
			withinTolerance++
		}
	}
	durationRows.Close()
	if result.Duration.MatchedAppointments > 0 {
		count := float64(result.Duration.MatchedAppointments)
		result.Duration.PlannedAverageMinutes = roundMetric(plannedTotal / count)
		result.Duration.ActualAverageMinutes = roundMetric(actualTotal / count)
		result.Duration.VarianceAverageMinutes = roundMetric((actualTotal - plannedTotal) / count)
		result.Duration.WithinTolerancePercent = roundMetric(float64(withinTolerance) * 100 / count)
	}

	slotRows, err := s.db.QueryContext(r.Context(), `SELECT starts_at,status FROM appointments WHERE starts_at>=? AND starts_at<? AND archived_at IS NULL AND status IN ('completed','no_show')`, cutoff, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not load no-show slot metrics.")
		return
	}
	slotAggregates := map[[2]int]noShowSlot{}
	for slotRows.Next() {
		var startsRaw, status string
		if err := slotRows.Scan(&startsRaw, &status); err != nil {
			slotRows.Close()
			writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not read no-show slot metrics.")
			return
		}
		starts, err := time.Parse(time.RFC3339Nano, startsRaw)
		if err != nil {
			continue
		}
		local := starts.In(location)
		key := [2]int{int(local.Weekday()), local.Hour()}
		item := slotAggregates[key]
		item.Weekday, item.Hour = key[0], key[1]
		item.Total++
		if status == "no_show" {
			item.NoShows++
		}
		slotAggregates[key] = item
	}
	slotRows.Close()
	for _, item := range slotAggregates {
		if item.NoShows == 0 {
			continue
		}
		item.Rate = roundMetric(float64(item.NoShows) * 100 / float64(item.Total))
		result.NoShowSlots = append(result.NoShowSlots, item)
	}
	sort.Slice(result.NoShowSlots, func(i, j int) bool {
		if result.NoShowSlots[i].NoShows == result.NoShowSlots[j].NoShows {
			return result.NoShowSlots[i].Total > result.NoShowSlots[j].Total
		}
		return result.NoShowSlots[i].NoShows > result.NoShowSlots[j].NoShows
	})

	patientRows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.medical_record_number,p.first_name||' '||p.last_name,COUNT(*),SUM(CASE WHEN a.status='no_show' THEN 1 ELSE 0 END)
		FROM appointments a JOIN patients p ON p.id=a.patient_id
		WHERE a.starts_at>=? AND a.starts_at<? AND a.archived_at IS NULL AND a.status IN ('completed','no_show')
		GROUP BY p.id,p.medical_record_number,p.first_name,p.last_name HAVING SUM(CASE WHEN a.status='no_show' THEN 1 ELSE 0 END)>0
		ORDER BY 5 DESC,4 DESC LIMIT 20`, cutoff, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not load patient no-show metrics.")
		return
	}
	for patientRows.Next() {
		var item noShowPatient
		if err := patientRows.Scan(&item.PatientID, &item.MedicalRecordNumber, &item.PatientName, &item.Total, &item.NoShows); err != nil {
			patientRows.Close()
			writeError(w, http.StatusInternalServerError, "OPERATIONS_ANALYTICS_FAILED", "Could not read patient no-show metrics.")
			return
		}
		item.Rate = roundMetric(float64(item.NoShows) * 100 / float64(item.Total))
		result.NoShowPatients = append(result.NoShowPatients, item)
	}
	patientRows.Close()

	writeJSON(w, http.StatusOK, result)
}
