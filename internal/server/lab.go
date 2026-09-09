package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

type labOrderPayload struct {
	PatientID      string         `json:"patientId"`
	PrescriptionID string         `json:"prescriptionId"`
	InvoiceID      string         `json:"invoiceId"`
	SupplierID     string         `json:"supplierId"`
	FrameItemID    string         `json:"frameItemId"`
	LensItemID     string         `json:"lensItemId"`
	LensType       string         `json:"lensType"`
	Material       string         `json:"material"`
	Coatings       []string       `json:"coatings"`
	Tint           string         `json:"tint"`
	Treatments     []string       `json:"treatments"`
	Measurements   map[string]any `json:"measurements"`
	Notes          string         `json:"notes"`
	ExpectedAt     string         `json:"expectedAt"`
	CostMinor      int64          `json:"costMinor"`
	SalePriceMinor int64          `json:"salePriceMinor"`
}

func (s *Server) handleLabOrdersList(w http.ResponseWriter, r *http.Request) {
	status, patientID := r.URL.Query().Get("status"), r.URL.Query().Get("patientId")
	where, args := "1=1", []any{}
	if status != "" {
		where += " AND l.status=?"
		args = append(args, status)
	}
	if patientID != "" {
		where += " AND l.patient_id=?"
		args = append(args, patientID)
	}
	paging := paginationFrom(r, 50, 200)
	orderCount, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM lab_orders l WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_LIST_FAILED", "Could not load optical lab orders.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT l.id,l.order_number,l.patient_id,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(l.prescription_id,''),COALESCE(l.invoice_id,''),COALESCE(l.supplier_id,''),COALESCE(s.company,''),COALESCE(l.frame_item_id,''),COALESCE(f.name,''),COALESCE(l.lens_item_id,''),COALESCE(le.name,''),COALESCE(l.lens_type,''),COALESCE(l.material,''),l.coatings_json,COALESCE(l.tint,''),l.treatments_json,l.measurements_json,COALESCE(l.notes,''),COALESCE(l.ordered_at,''),COALESCE(l.expected_at,''),l.cost_minor,l.sale_price_minor,l.status,COALESCE(l.delivered_at,''),l.version,l.created_at,l.updated_at FROM lab_orders l JOIN patients p ON p.id=l.patient_id LEFT JOIN suppliers s ON s.id=l.supplier_id LEFT JOIN inventory_items f ON f.id=l.frame_item_id LEFT JOIN inventory_items le ON le.id=l.lens_item_id WHERE `+where+` ORDER BY CASE l.status WHEN 'ready' THEN 0 WHEN 'quality_control' THEN 1 ELSE 2 END,l.expected_at,l.created_at DESC LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_LIST_FAILED", "Could not load optical lab orders.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, patientID, mrn, patientName, prescriptionID, invoiceID, supplierID, supplierName, frameID, frameName, lensID, lensName, lensType, material, coatings, tint, treatments, measurements, notes, orderedAt, expectedAt, status, deliveredAt, createdAt, updatedAt string
		var cost, price int64
		var version int
		if err := rows.Scan(&id, &number, &patientID, &mrn, &patientName, &prescriptionID, &invoiceID, &supplierID, &supplierName, &frameID, &frameName, &lensID, &lensName, &lensType, &material, &coatings, &tint, &treatments, &measurements, &notes, &orderedAt, &expectedAt, &cost, &price, &status, &deliveredAt, &version, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "LAB_ORDER_LIST_FAILED", "Could not load optical lab orders.")
			return
		}
		items = append(items, map[string]any{"id": id, "orderNumber": number, "patientId": patientID, "medicalRecordNumber": mrn, "patientName": patientName, "prescriptionId": prescriptionID, "invoiceId": invoiceID, "supplierId": supplierID, "supplierName": supplierName, "frameItemId": frameID, "frameName": frameName, "lensItemId": lensID, "lensName": lensName, "lensType": lensType, "material": material, "coatings": rawJSON(coatings), "tint": tint, "treatments": rawJSON(treatments), "measurements": rawJSON(measurements), "notes": notes, "orderedAt": orderedAt, "expectedAt": expectedAt, "costMinor": cost, "salePriceMinor": price, "status": status, "deliveredAt": deliveredAt, "version": version, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(orderCount)))
}

func (s *Server) handleLabOrderCreate(w http.ResponseWriter, r *http.Request) {
	var input labOrderPayload
	if err := decodeJSON(r, &input); err != nil || input.PatientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Patient is required for an optical lab order.")
		return
	}
	if input.CostMinor < 0 || input.SalePriceMinor < 0 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_LAB_ORDER_AMOUNT", "Lab cost and sale price cannot be negative.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "lab_order", "LAB", true)
		if err != nil {
			return err
		}
		if err := validatePatientLinks(r.Context(), tx, input.PatientID,
			patientLink{"SELECT patient_id FROM prescriptions WHERE id=? AND archived_at IS NULL", input.PrescriptionID},
			patientLink{"SELECT COALESCE(patient_id,'') FROM invoices WHERE id=? AND archived_at IS NULL", input.InvoiceID}); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO lab_orders(id,order_number,patient_id,prescription_id,invoice_id,supplier_id,frame_item_id,lens_item_id,lens_type,material,coatings_json,tint,treatments_json,measurements_json,notes,expected_at,cost_minor,sale_price_minor,status,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'draft',?,?,?,?)`, id, number, input.PatientID, nilIfEmpty(input.PrescriptionID), nilIfEmpty(input.InvoiceID), nilIfEmpty(input.SupplierID), nilIfEmpty(input.FrameItemID), nilIfEmpty(input.LensItemID), nilIfEmpty(input.LensType), nilIfEmpty(input.Material), marshalJSON(input.Coatings), nilIfEmpty(input.Tint), marshalJSON(input.Treatments), marshalJSON(input.Measurements), nilIfEmpty(input.Notes), nilIfEmpty(input.ExpectedAt), input.CostMinor, input.SalePriceMinor, now, now, user.ID, user.ID)
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, 422, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_CREATE_FAILED", "Could not create optical lab order.")
		return
	}
	s.audit(r.Context(), &user, "create", "lab_order", id, "Created optical lab order "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "lab_order.created", EntityType: "lab_order", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "orderNumber": number, "status": "draft", "version": 1})
}

type labStatusPayload struct {
	Status         string `json:"status"`
	Notes          string `json:"notes"`
	ReceivedByName string `json:"receivedByName"`
	Version        int    `json:"version"`
}

func (s *Server) handleLabOrderStatus(w http.ResponseWriter, r *http.Request) {
	var input labStatusPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Lab status and current version are required.")
		return
	}
	valid := map[string]bool{"draft": true, "ordered": true, "at_lab": true, "received": true, "edging_mounting": true, "quality_control": true, "ready": true, "delivered": true, "cancelled": true}
	if !valid[input.Status] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_LAB_STATUS", "Lab order status is invalid.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	var fromStatus string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(r.Context(), "SELECT status FROM lab_orders WHERE id=?", id).Scan(&fromStatus); err != nil {
			return err
		}
		if input.Status == "ready" || input.Status == "delivered" {
			var qc int
			if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM lab_quality_control WHERE lab_order_id=?", id).Scan(&qc); err != nil {
				return err
			}
			if qc == 0 {
				return &APIError{Code: "QUALITY_CONTROL_REQUIRED", Message: "Complete quality control before marking the order ready or delivered."}
			}
		}
		deliveredAt, deliveredBy, receivedBy := any(nil), any(nil), any(nil)
		if input.Status == "delivered" {
			deliveredAt, deliveredBy, receivedBy = now, user.ID, nilIfEmpty(input.ReceivedByName)
		}
		orderedAt := any(nil)
		if input.Status == "ordered" {
			orderedAt = now
		}
		result, err := tx.ExecContext(r.Context(), `UPDATE lab_orders SET status=?,ordered_at=COALESCE(ordered_at,?),delivered_at=?,delivered_by=?,received_by_name=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, input.Status, orderedAt, deliveredAt, deliveredBy, receivedBy, now, user.ID, id, input.Version)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errConcurrentModification
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO lab_status_history(id,lab_order_id,from_status,to_status,notes,changed_at,changed_by) VALUES(?,?,?,?,?,?,?)`, uuid.NewString(), id, fromStatus, input.Status, nilIfEmpty(input.Notes), now, user.ID)
		return err
	})
	if err != nil {
		if err == errConcurrentModification {
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The lab order changed since it was opened.")
			return
		}
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "LAB_STATUS_FAILED", "Could not update lab order status.")
		return
	}
	s.audit(r.Context(), &user, "status_change", "lab_order", id, "Lab order moved from "+fromStatus+" to "+input.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "lab_order.updated", EntityType: "lab_order", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status, "version": input.Version + 1})
}

type qualityControlPayload struct {
	PrescriptionVerified bool   `json:"prescriptionVerified"`
	PowerVerified        bool   `json:"powerVerified"`
	AxisVerified         bool   `json:"axisVerified"`
	FrameCondition       bool   `json:"frameCondition"`
	LensCondition        bool   `json:"lensCondition"`
	FittingVerified      bool   `json:"fittingVerified"`
	FinalCleaning        bool   `json:"finalCleaning"`
	Notes                string `json:"notes"`
}

func (s *Server) handleLabQualityControl(w http.ResponseWriter, r *http.Request) {
	var input qualityControlPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if !input.PrescriptionVerified || !input.PowerVerified || !input.FrameCondition || !input.LensCondition || !input.FittingVerified || !input.FinalCleaning {
		writeError(w, http.StatusUnprocessableEntity, "QC_INCOMPLETE", "All required quality checks must be completed.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO lab_quality_control(id,lab_order_id,prescription_verified,power_verified,axis_verified,frame_condition,lens_condition,fitting_verified,final_cleaning,notes,completed_at,completed_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(lab_order_id) DO UPDATE SET prescription_verified=excluded.prescription_verified,power_verified=excluded.power_verified,axis_verified=excluded.axis_verified,frame_condition=excluded.frame_condition,lens_condition=excluded.lens_condition,fitting_verified=excluded.fitting_verified,final_cleaning=excluded.final_cleaning,notes=excluded.notes,completed_at=excluded.completed_at,completed_by=excluded.completed_by`, uuid.NewString(), id, boolInt(input.PrescriptionVerified), boolInt(input.PowerVerified), boolInt(input.AxisVerified), boolInt(input.FrameCondition), boolInt(input.LensCondition), boolInt(input.FittingVerified), boolInt(input.FinalCleaning), nilIfEmpty(input.Notes), now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QC_SAVE_FAILED", "Could not save quality control checklist.")
		return
	}
	s.audit(r.Context(), &user, "quality_control", "lab_order", id, "Completed optical quality control", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"labOrderId": id, "completedAt": now})
}

type labBulkStatusPayload struct {
	OrderIDs []string `json:"orderIds"`
	Status   string   `json:"status"`
	Notes    string   `json:"notes"`
}

// handleLabOrdersBulkStatus sends a batch of orders to the lab in one action.
// A clinic sends a day's work together, and doing it one order at a time is
// where orders get missed. The whole batch commits or none of it does, so the
// operator never has to work out which half went.
func (s *Server) handleLabOrdersBulkStatus(w http.ResponseWriter, r *http.Request) {
	var input labBulkStatusPayload
	if err := decodeJSON(r, &input); err != nil || len(input.OrderIDs) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Choose at least one lab order and a status.")
		return
	}
	if len(input.OrderIDs) > 200 {
		writeError(w, http.StatusUnprocessableEntity, "BATCH_TOO_LARGE", "Send at most 200 lab orders at a time.")
		return
	}
	// Only the transitions a batch legitimately makes. Delivery and quality
	// control are per-order decisions and stay per-order.
	if !map[string]bool{"ordered": true, "at_lab": true, "received": true, "edging_mounting": true, "cancelled": true}[input.Status] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_BULK_LAB_STATUS", "A batch can be marked ordered, at the lab, received, in edging/mounting, or cancelled.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updated := []string{}
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		seen := map[string]bool{}
		for _, id := range input.OrderIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			var fromStatus string
			if err := tx.QueryRowContext(r.Context(), "SELECT status FROM lab_orders WHERE id=?", id).Scan(&fromStatus); err != nil {
				return err
			}
			if fromStatus == "delivered" || fromStatus == "cancelled" {
				return &APIError{Code: "LAB_ORDER_CLOSED", Message: "One of the selected orders is already delivered or cancelled. Remove it from the batch."}
			}
			orderedAt := any(nil)
			if input.Status == "ordered" {
				orderedAt = now
			}
			if _, err := tx.ExecContext(r.Context(), "UPDATE lab_orders SET status=?,ordered_at=COALESCE(ordered_at,?),version=version+1,updated_at=?,updated_by=? WHERE id=?", input.Status, orderedAt, now, user.ID, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO lab_status_history(id,lab_order_id,from_status,to_status,notes,changed_at,changed_by) VALUES(?,?,?,?,?,?,?)`,
				uuid.NewString(), id, fromStatus, input.Status, nilIfEmpty(strings.TrimSpace(input.Notes)), now, user.ID); err != nil {
				return err
			}
			updated = append(updated, id)
		}
		return nil
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		if err == sql.ErrNoRows {
			writeError(w, http.StatusUnprocessableEntity, "LAB_ORDER_NOT_FOUND", "One of the selected lab orders no longer exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "LAB_BULK_STATUS_FAILED", "No order was changed. The batch was rolled back.")
		return
	}
	s.audit(r.Context(), &user, "update", "lab_order", strings.Join(updated, ","), fmt.Sprintf("Moved %d lab orders to %s", len(updated), input.Status), "", "", r)
	s.broker.Publish(realtime.Event{Type: "lab.changed", EntityType: "lab_order", EntityID: ""})
	writeJSON(w, http.StatusOK, map[string]any{"updated": len(updated), "orderIds": updated, "status": input.Status})
}

// handleLabRequisitionBatch builds the printed requisition for a batch of
// orders: one document a courier can carry to the lab, rather than a stack
// printed one screen at a time.
func (s *Server) handleLabRequisitionBatch(w http.ResponseWriter, r *http.Request) {
	ids := []string{}
	for _, raw := range strings.Split(r.URL.Query().Get("orderIds"), ",") {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 || len(ids) > 200 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Choose between 1 and 200 lab orders to print.")
		return
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT l.id,l.order_number,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(p.phone,''),COALESCE(s.company,''),
		COALESCE(f.name,''),COALESCE(le.name,''),COALESCE(l.lens_type,''),COALESCE(l.material,''),l.coatings_json,COALESCE(l.tint,''),l.treatments_json,l.measurements_json,COALESCE(l.notes,''),COALESCE(l.expected_at,''),l.status
		FROM lab_orders l JOIN patients p ON p.id=l.patient_id
		LEFT JOIN suppliers s ON s.id=l.supplier_id
		LEFT JOIN inventory_items f ON f.id=l.frame_item_id
		LEFT JOIN inventory_items le ON le.id=l.lens_item_id
		WHERE l.id IN (`+placeholders+`) ORDER BY COALESCE(s.company,''), l.order_number`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_REQUISITION_FAILED", "Could not build the lab requisition.")
		return
	}
	defer rows.Close()
	orders := []map[string]any{}
	for rows.Next() {
		var id, number, mrn, patientName, phone, supplier, frame, lens, lensType, material, coatings, tint, treatments, measurements, notes, expectedAt, status string
		if err := rows.Scan(&id, &number, &mrn, &patientName, &phone, &supplier, &frame, &lens, &lensType, &material, &coatings, &tint, &treatments, &measurements, &notes, &expectedAt, &status); err != nil {
			writeError(w, http.StatusInternalServerError, "LAB_REQUISITION_FAILED", "Could not build the lab requisition.")
			return
		}
		orders = append(orders, map[string]any{"id": id, "orderNumber": number, "medicalRecordNumber": mrn, "patientName": patientName, "patientPhone": phone,
			"supplierName": supplier, "frameName": frame, "lensName": lens, "lensType": lensType, "material": material,
			"coatings": rawJSON(coatings), "tint": tint, "treatments": rawJSON(treatments), "measurements": rawJSON(measurements),
			"notes": notes, "expectedAt": expectedAt, "status": status})
	}
	if len(orders) == 0 {
		writeError(w, http.StatusNotFound, "LAB_ORDER_NOT_FOUND", "None of those lab orders were found.")
		return
	}
	var clinicName, clinicAddress, clinicPhone string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.name'),''),COALESCE(json_extract(value_json,'$.address'),''),COALESCE(json_extract(value_json,'$.phone'),'') FROM settings WHERE key='clinic'`).
		Scan(&clinicName, &clinicAddress, &clinicPhone)
	writeJSON(w, http.StatusOK, map[string]any{
		"clinic":      map[string]any{"name": clinicName, "address": clinicAddress, "phone": clinicPhone},
		"generatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"orders":      orders,
	})
}
