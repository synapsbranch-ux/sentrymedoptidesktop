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

// An order is optical — a frame and lenses sent to a glazing company — or
// medical, a list of exams sent to a laboratory. They travel the same road out
// of the clinic and back, so they share this payload; the fields the other kind
// does not use are simply left empty.
const (
	labKindOptical = "optical"
	labKindMedical = "medical"
)

const maxLabTestsPerOrder = 50

type labTestPayload struct {
	Label    string `json:"label"`
	Code     string `json:"code"`
	Specimen string `json:"specimen"`
	Notes    string `json:"notes"`
}

type labOrderPayload struct {
	Kind           string           `json:"kind"`
	Tests          []labTestPayload `json:"tests"`
	PatientID      string           `json:"patientId"`
	EncounterID    string           `json:"encounterId"`
	PrescriptionID string           `json:"prescriptionId"`
	InvoiceID      string           `json:"invoiceId"`
	SupplierID     string           `json:"supplierId"`
	FrameItemID    string           `json:"frameItemId"`
	LensItemID     string           `json:"lensItemId"`
	LensType       string           `json:"lensType"`
	Material       string           `json:"material"`
	Coatings       []string         `json:"coatings"`
	Tint           string           `json:"tint"`
	Treatments     []string         `json:"treatments"`
	Measurements   map[string]any   `json:"measurements"`
	Notes          string           `json:"notes"`
	ExpectedAt     string           `json:"expectedAt"`
	CostMinor      int64            `json:"costMinor"`
	SalePriceMinor int64            `json:"salePriceMinor"`
}

func (s *Server) handleLabOrdersList(w http.ResponseWriter, r *http.Request) {
	status, patientID, kind := r.URL.Query().Get("status"), r.URL.Query().Get("patientId"), r.URL.Query().Get("kind")
	where, args := "1=1", []any{}
	if status != "" {
		where += " AND l.status=?"
		args = append(args, status)
	}
	if kind == labKindOptical || kind == labKindMedical {
		where += " AND l.kind=?"
		args = append(args, kind)
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
	rows, err := s.db.QueryContext(r.Context(), `SELECT l.id,l.order_number,l.kind,l.patient_id,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(l.prescription_id,''),COALESCE(l.invoice_id,''),COALESCE(l.supplier_id,''),COALESCE(s.company,''),COALESCE(l.frame_item_id,''),COALESCE(f.name,''),COALESCE(l.lens_item_id,''),COALESCE(le.name,''),COALESCE(l.lens_type,''),COALESCE(l.material,''),l.coatings_json,COALESCE(l.tint,''),l.treatments_json,l.measurements_json,COALESCE(l.notes,''),COALESCE(l.ordered_at,''),COALESCE(l.expected_at,''),l.cost_minor,l.sale_price_minor,l.status,COALESCE(l.delivered_at,''),l.version,l.created_at,l.updated_at FROM lab_orders l JOIN patients p ON p.id=l.patient_id LEFT JOIN suppliers s ON s.id=l.supplier_id LEFT JOIN inventory_items f ON f.id=l.frame_item_id LEFT JOIN inventory_items le ON le.id=l.lens_item_id WHERE `+where+` ORDER BY CASE l.status WHEN 'ready' THEN 0 WHEN 'quality_control' THEN 1 ELSE 2 END,l.expected_at,l.created_at DESC LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_LIST_FAILED", "Could not load optical lab orders.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, kind, patientID, mrn, patientName, prescriptionID, invoiceID, supplierID, supplierName, frameID, frameName, lensID, lensName, lensType, material, coatings, tint, treatments, measurements, notes, orderedAt, expectedAt, status, deliveredAt, createdAt, updatedAt string
		var cost, price int64
		var version int
		if err := rows.Scan(&id, &number, &kind, &patientID, &mrn, &patientName, &prescriptionID, &invoiceID, &supplierID, &supplierName, &frameID, &frameName, &lensID, &lensName, &lensType, &material, &coatings, &tint, &treatments, &measurements, &notes, &orderedAt, &expectedAt, &cost, &price, &status, &deliveredAt, &version, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "LAB_ORDER_LIST_FAILED", "Could not load optical lab orders.")
			return
		}
		items = append(items, map[string]any{"id": id, "orderNumber": number, "kind": kind, "patientId": patientID, "medicalRecordNumber": mrn, "patientName": patientName, "prescriptionId": prescriptionID, "invoiceId": invoiceID, "supplierId": supplierID, "supplierName": supplierName, "frameItemId": frameID, "frameName": frameName, "lensItemId": lensID, "lensName": lensName, "lensType": lensType, "material": material, "coatings": rawJSON(coatings), "tint": tint, "treatments": rawJSON(treatments), "measurements": rawJSON(measurements), "notes": notes, "orderedAt": orderedAt, "expectedAt": expectedAt, "costMinor": cost, "salePriceMinor": price, "status": status, "deliveredAt": deliveredAt, "version": version, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(orderCount)))
}

func (s *Server) handleLabOrderCreate(w http.ResponseWriter, r *http.Request) {
	var input labOrderPayload
	if err := decodeJSON(r, &input); err != nil || input.PatientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Patient is required for a lab order.")
		return
	}
	// Orders written before the section handled exams carry no kind, and they
	// were all optical.
	if input.Kind == "" {
		input.Kind = labKindOptical
	}
	if input.Kind != labKindOptical && input.Kind != labKindMedical {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_LAB_ORDER_KIND", "A lab order is either an optical order or a laboratory exam request.")
		return
	}
	tests := []labTestPayload{}
	for _, test := range input.Tests {
		test.Label = strings.TrimSpace(test.Label)
		if test.Label != "" {
			tests = append(tests, test)
		}
	}
	// An exam request with nothing on it tells the laboratory nothing, and it is
	// the one field of a medical order the clinic cannot fill in later over the
	// phone without the request already having left.
	if input.Kind == labKindMedical && len(tests) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "LAB_TESTS_REQUIRED", "Choose at least one exam for the laboratory.")
		return
	}
	if len(tests) > maxLabTestsPerOrder {
		writeError(w, http.StatusUnprocessableEntity, "TOO_MANY_LAB_TESTS", fmt.Sprintf("A request carries at most %d exams.", maxLabTestsPerOrder))
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
			patientLink{"SELECT patient_id FROM encounters WHERE id=? AND archived_at IS NULL", input.EncounterID},
			patientLink{"SELECT COALESCE(patient_id,'') FROM invoices WHERE id=? AND archived_at IS NULL", input.InvoiceID}); err != nil {
			return err
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO lab_orders(id,order_number,kind,patient_id,encounter_id,prescription_id,invoice_id,supplier_id,frame_item_id,lens_item_id,lens_type,material,coatings_json,tint,treatments_json,measurements_json,notes,expected_at,cost_minor,sale_price_minor,status,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'draft',?,?,?,?)`, id, number, input.Kind, input.PatientID, nilIfEmpty(input.EncounterID), nilIfEmpty(input.PrescriptionID), nilIfEmpty(input.InvoiceID), nilIfEmpty(input.SupplierID), nilIfEmpty(input.FrameItemID), nilIfEmpty(input.LensItemID), nilIfEmpty(input.LensType), nilIfEmpty(input.Material), marshalJSON(input.Coatings), nilIfEmpty(input.Tint), marshalJSON(input.Treatments), marshalJSON(input.Measurements), nilIfEmpty(input.Notes), nilIfEmpty(input.ExpectedAt), input.CostMinor, input.SalePriceMinor, now, now, user.ID, user.ID); err != nil {
			return err
		}
		// The exams travel with the order: either the laboratory is asked for
		// all of them or the request was never raised.
		for index, test := range tests {
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO lab_order_tests(id,lab_order_id,label,code,specimen,notes,sort_order) VALUES(?,?,?,?,?,?,?)`,
				uuid.NewString(), id, test.Label, nilIfEmpty(strings.TrimSpace(test.Code)), nilIfEmpty(strings.TrimSpace(test.Specimen)), nilIfEmpty(strings.TrimSpace(test.Notes)), index); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, 422, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_CREATE_FAILED", "Could not create the lab order.")
		return
	}
	description := "Created optical lab order " + number
	if input.Kind == labKindMedical {
		description = "Created laboratory exam request " + number
	}
	s.audit(r.Context(), &user, "create", "lab_order", id, description, "", "", r)
	s.broker.Publish(realtime.Event{Type: "lab_order.created", EntityType: "lab_order", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "orderNumber": number, "kind": input.Kind, "status": "draft", "version": 1})
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
	var fromStatus, kind string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(r.Context(), "SELECT status,kind FROM lab_orders WHERE id=?", id).Scan(&fromStatus, &kind); err != nil {
			return err
		}
		// Quality control inspects a lens in a frame. A blood sample has neither,
		// so the gate belongs to optical orders only — applied to both it would
		// strand every exam request one step short of the patient.
		if kind == labKindOptical && (input.Status == "ready" || input.Status == "delivered") {
			var qc int
			if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM lab_quality_control WHERE lab_order_id=?", id).Scan(&qc); err != nil {
				return err
			}
			if qc == 0 {
				return &APIError{Code: "QUALITY_CONTROL_REQUIRED", Message: "Complete quality control before marking the order ready or delivered."}
			}
		}
		orderedAt := any(nil)
		if input.Status == "ordered" {
			orderedAt = now
		}
		// Who collected the glasses and when is a record of a handover that
		// happened. Correcting a status typed in error must not erase it, so the
		// delivery columns are written only when something is being delivered.
		var result sql.Result
		var err error
		if input.Status == "delivered" {
			result, err = tx.ExecContext(r.Context(), `UPDATE lab_orders SET status=?,ordered_at=COALESCE(ordered_at,?),delivered_at=?,delivered_by=?,received_by_name=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, input.Status, orderedAt, now, user.ID, nilIfEmpty(input.ReceivedByName), now, user.ID, id, input.Version)
		} else {
			result, err = tx.ExecContext(r.Context(), `UPDATE lab_orders SET status=?,ordered_at=COALESCE(ordered_at,?),version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`, input.Status, orderedAt, now, user.ID, id, input.Version)
		}
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
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "LAB_ORDER_NOT_FOUND", "That lab order was not found.")
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
	rows, err := s.db.QueryContext(r.Context(), `SELECT l.id,l.kind,l.order_number,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(p.phone,''),COALESCE(s.company,''),
		COALESCE(f.name,''),COALESCE(le.name,''),COALESCE(l.lens_type,''),COALESCE(l.material,''),l.coatings_json,COALESCE(l.tint,''),l.treatments_json,l.measurements_json,COALESCE(l.notes,''),COALESCE(l.expected_at,''),l.status,
		COALESCE(pr.prescription_number,''),COALESCE(pr.od_json,'{}'),COALESCE(pr.os_json,'{}'),COALESCE(pr.details_json,'{}')
		FROM lab_orders l JOIN patients p ON p.id=l.patient_id
		LEFT JOIN suppliers s ON s.id=l.supplier_id
		LEFT JOIN inventory_items f ON f.id=l.frame_item_id
		LEFT JOIN inventory_items le ON le.id=l.lens_item_id
		LEFT JOIN prescriptions pr ON pr.id=l.prescription_id
		WHERE l.id IN (`+placeholders+`) ORDER BY l.kind, COALESCE(s.company,''), l.order_number`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_REQUISITION_FAILED", "Could not build the lab requisition.")
		return
	}
	defer rows.Close()
	orders := []map[string]any{}
	for rows.Next() {
		var id, kind, number, mrn, patientName, phone, supplier, frame, lens, lensType, material, coatings, tint, treatments, measurements, notes, expectedAt, status string
		var rxNumber, od, os, rxDetails string
		if err := rows.Scan(&id, &kind, &number, &mrn, &patientName, &phone, &supplier, &frame, &lens, &lensType, &material, &coatings, &tint, &treatments, &measurements, &notes, &expectedAt, &status, &rxNumber, &od, &os, &rxDetails); err != nil {
			writeError(w, http.StatusInternalServerError, "LAB_REQUISITION_FAILED", "Could not build the lab requisition.")
			return
		}
		orders = append(orders, map[string]any{"id": id, "kind": kind, "orderNumber": number, "medicalRecordNumber": mrn, "patientName": patientName, "patientPhone": phone,
			"supplierName": supplier, "frameName": frame, "lensName": lens, "lensType": lensType, "material": material,
			"coatings": rawJSON(coatings), "tint": tint, "treatments": rawJSON(treatments), "measurements": rawJSON(measurements),
			"notes": notes, "expectedAt": expectedAt, "status": status,
			// A batch that leaves for the glazing company without the powers is a
			// batch the company has to telephone back about, one order at a time.
			"prescriptionNumber": rxNumber, "od": rawJSON(od), "os": rawJSON(os), "prescriptionDetails": rawJSON(rxDetails),
			"tests": []map[string]any{}})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_REQUISITION_FAILED", "Could not build the lab requisition.")
		return
	}
	// The exams for the whole batch in one pass, rather than a query per order
	// while the batch cursor is still open.
	if err := s.attachRequisitionTests(r, orders, ids, placeholders); err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_REQUISITION_FAILED", "Could not build the lab requisition.")
		return
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

// handleLabOrderDocument assembles everything one order's printed document
// needs, in one request.
//
// The list endpoint answers a board: who, what stage, when due. A workshop
// document answers a different question — what exactly to make — and needs the
// prescription powers, the sale, the consultation and the laboratory's contact
// details, none of which the board carries. Fetching those as five separate
// calls from the browser would make the print dialog open on a half-built page.
func (s *Server) handleLabOrderDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var kind, number, status, notes, orderedAt, expectedAt, deliveredAt, receivedBy, lensType, material, coatings, tint, treatments, measurements string
	var costMinor, salePriceMinor int64
	var version int
	var patientName, mrn, patientPhone, patientDOB string
	var prescriptionID, encounterID, invoiceID, supplierID, frameID, lensID string
	err := s.db.QueryRowContext(r.Context(), `SELECT l.kind,l.order_number,l.status,COALESCE(l.notes,''),COALESCE(l.ordered_at,''),COALESCE(l.expected_at,''),COALESCE(l.delivered_at,''),COALESCE(l.received_by_name,''),
		COALESCE(l.lens_type,''),COALESCE(l.material,''),l.coatings_json,COALESCE(l.tint,''),l.treatments_json,l.measurements_json,l.cost_minor,l.sale_price_minor,l.version,
		p.first_name||' '||p.last_name,p.medical_record_number,COALESCE(p.phone,''),COALESCE(p.date_of_birth,''),
		COALESCE(l.prescription_id,''),COALESCE(l.encounter_id,''),COALESCE(l.invoice_id,''),COALESCE(l.supplier_id,''),COALESCE(l.frame_item_id,''),COALESCE(l.lens_item_id,'')
		FROM lab_orders l JOIN patients p ON p.id=l.patient_id WHERE l.id=?`, id).
		Scan(&kind, &number, &status, &notes, &orderedAt, &expectedAt, &deliveredAt, &receivedBy, &lensType, &material, &coatings, &tint, &treatments, &measurements, &costMinor, &salePriceMinor, &version,
			&patientName, &mrn, &patientPhone, &patientDOB, &prescriptionID, &encounterID, &invoiceID, &supplierID, &frameID, &lensID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "LAB_ORDER_NOT_FOUND", "That lab order was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_LOAD_FAILED", "Could not load the lab order.")
		return
	}
	document := map[string]any{
		"order": map[string]any{"id": id, "kind": kind, "orderNumber": number, "status": status, "notes": notes, "orderedAt": orderedAt, "expectedAt": expectedAt,
			"deliveredAt": deliveredAt, "receivedByName": receivedBy, "lensType": lensType, "material": material, "coatings": rawJSON(coatings), "tint": tint,
			"treatments": rawJSON(treatments), "measurements": rawJSON(measurements), "costMinor": costMinor, "salePriceMinor": salePriceMinor, "version": version},
		"patient": map[string]any{"name": patientName, "medicalRecordNumber": mrn, "phone": patientPhone, "dateOfBirth": patientDOB},
	}

	// Every part is required or the sheet is wrong rather than incomplete: a
	// workshop cannot tell a prescription that failed to load from a repair that
	// never had one.
	assemble := func() error {
		if prescriptionID != "" {
			var rxNumber, rxType, od, osValues, details, rxNotes, issuedAt, expiresAt, doctor, rxEncounterID string
			err := s.db.QueryRowContext(r.Context(), `SELECT pr.prescription_number,pr.type,pr.od_json,pr.os_json,pr.details_json,COALESCE(pr.notes,''),pr.issued_at,COALESCE(pr.expires_at,''),COALESCE(u.display_name,''),COALESCE(pr.encounter_id,'')
				FROM prescriptions pr LEFT JOIN users u ON u.id=pr.doctor_id WHERE pr.id=?`, prescriptionID).
				Scan(&rxNumber, &rxType, &od, &osValues, &details, &rxNotes, &issuedAt, &expiresAt, &doctor, &rxEncounterID)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == nil {
				document["prescription"] = map[string]any{"id": prescriptionID, "prescriptionNumber": rxNumber, "type": rxType, "od": rawJSON(od), "os": rawJSON(osValues),
					"details": rawJSON(details), "notes": rxNotes, "issuedAt": issuedAt, "expiresAt": expiresAt, "doctor": doctor}
				// An optical order reaches the consultation through its
				// prescription; only an exam request carries the link itself.
				if encounterID == "" {
					encounterID = rxEncounterID
				}
			}
		}
		if encounterID != "" {
			var encounterNumber, visitReason, encounterStatus, createdAt, doctor string
			err := s.db.QueryRowContext(r.Context(), `SELECT e.encounter_number,COALESCE(e.visit_reason,''),e.status,e.created_at,COALESCE(u.display_name,'')
				FROM encounters e LEFT JOIN users u ON u.id=e.doctor_id WHERE e.id=?`, encounterID).Scan(&encounterNumber, &visitReason, &encounterStatus, &createdAt, &doctor)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == nil {
				document["encounter"] = map[string]any{"id": encounterID, "encounterNumber": encounterNumber, "visitReason": visitReason, "status": encounterStatus, "date": createdAt, "doctor": doctor}
			}
		}
		if supplierID != "" {
			var company, contact, phone, email, address string
			err := s.db.QueryRowContext(r.Context(), `SELECT company,COALESCE(contact_person,''),COALESCE(phone,''),COALESCE(email,''),COALESCE(address,'') FROM suppliers WHERE id=?`, supplierID).
				Scan(&company, &contact, &phone, &email, &address)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == nil {
				document["supplier"] = map[string]any{"id": supplierID, "company": company, "contactPerson": contact, "phone": phone, "email": email, "address": address}
			}
		}
		frame, err := s.labInventoryItem(r, frameID)
		if err != nil {
			return err
		}
		lens, err := s.labInventoryItem(r, lensID)
		if err != nil {
			return err
		}
		document["frame"], document["lens"] = frame, lens

		if invoiceID != "" {
			invoice, err := s.labOrderSale(r, invoiceID)
			if err != nil {
				return err
			}
			document["invoice"] = invoice
		}
		tests, err := s.labOrderTests(r, id)
		if err != nil {
			return err
		}
		document["tests"] = tests
		images, err := s.labOrderImageIDs(r, id)
		if err != nil {
			return err
		}
		document["imageIds"] = images
		history, err := s.labOrderHistory(r, id)
		if err != nil {
			return err
		}
		document["statusHistory"] = history
		return nil
	}
	if err := assemble(); err != nil {
		writeError(w, http.StatusInternalServerError, "LAB_ORDER_LOAD_FAILED", "Could not load the lab order.")
		return
	}
	for _, key := range []string{"prescription", "encounter", "supplier", "frame", "lens", "invoice"} {
		if _, present := document[key]; !present {
			document[key] = nil
		}
	}
	writeJSON(w, http.StatusOK, document)
}

func (s *Server) labInventoryItem(r *http.Request, itemID string) (map[string]any, error) {
	if itemID == "" {
		return nil, nil
	}
	var sku, name, brand, model string
	err := s.db.QueryRowContext(r.Context(), `SELECT sku,name,COALESCE(brand,''),COALESCE(model,'') FROM inventory_items WHERE id=?`, itemID).Scan(&sku, &name, &brand, &model)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": itemID, "sku": sku, "name": name, "brand": brand, "model": model}, nil
}

// labOrderSale is what the patient already paid for, so the workshop can see the
// frame and lenses it is being asked to make were actually sold.
func (s *Server) labOrderSale(r *http.Request, invoiceID string) (map[string]any, error) {
	var invoiceNumber, currency, invoiceStatus, createdAt string
	var total, paid int64
	err := s.db.QueryRowContext(r.Context(), `SELECT invoice_number,currency,status,created_at,total_minor,COALESCE((SELECT SUM(amount_minor) FROM payments WHERE invoice_id=invoices.id),0)
		FROM invoices WHERE id=? AND archived_at IS NULL`, invoiceID).Scan(&invoiceNumber, &currency, &invoiceStatus, &createdAt, &total, &paid)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT description,quantity,unit_price_minor,line_total_minor FROM invoice_items WHERE invoice_id=? ORDER BY rowid`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []map[string]any{}
	for rows.Next() {
		var description string
		var quantity int
		var unitPrice, lineTotal int64
		if err := rows.Scan(&description, &quantity, &unitPrice, &lineTotal); err != nil {
			return nil, err
		}
		lines = append(lines, map[string]any{"description": description, "quantity": quantity, "unitPriceMinor": unitPrice, "lineTotalMinor": lineTotal})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"id": invoiceID, "invoiceNumber": invoiceNumber, "currency": currency, "status": invoiceStatus, "date": createdAt,
		"totalMinor": total, "paidMinor": paid, "balanceMinor": total - paid, "lines": lines}, nil
}

func (s *Server) labOrderTests(r *http.Request, orderID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,label,COALESCE(code,''),COALESCE(specimen,''),COALESCE(notes,'') FROM lab_order_tests WHERE lab_order_id=? ORDER BY sort_order`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tests := []map[string]any{}
	for rows.Next() {
		var id, label, code, specimen, notes string
		if err := rows.Scan(&id, &label, &code, &specimen, &notes); err != nil {
			return nil, err
		}
		tests = append(tests, map[string]any{"id": id, "label": label, "code": code, "specimen": specimen, "notes": notes})
	}
	return tests, rows.Err()
}

// The picture of the frame is fetched by the client through the authenticated
// image endpoint, so the document only needs to know which images exist.
func (s *Server) labOrderImageIDs(r *http.Request, orderID string) ([]string, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id FROM entity_images WHERE entity_type='lab_order' AND entity_id=? ORDER BY sort_order, created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Written on every move since the section was built and never read back until
// now; who moved an order and when is what a disputed delivery turns on.
func (s *Server) labOrderHistory(r *http.Request, orderID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT COALESCE(h.from_status,''),h.to_status,COALESCE(h.notes,''),h.changed_at,COALESCE(u.display_name,'')
		FROM lab_status_history h LEFT JOIN users u ON u.id=h.changed_by WHERE h.lab_order_id=? ORDER BY h.changed_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []map[string]any{}
	for rows.Next() {
		var from, to, notes, changedAt, by string
		if err := rows.Scan(&from, &to, &notes, &changedAt, &by); err != nil {
			return nil, err
		}
		history = append(history, map[string]any{"fromStatus": from, "toStatus": to, "notes": notes, "changedAt": changedAt, "changedBy": by})
	}
	return history, rows.Err()
}

func (s *Server) attachRequisitionTests(r *http.Request, orders []map[string]any, ids []string, placeholders string) error {
	if len(orders) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT lab_order_id,label,COALESCE(code,''),COALESCE(specimen,''),COALESCE(notes,'') FROM lab_order_tests WHERE lab_order_id IN (`+placeholders+`) ORDER BY lab_order_id, sort_order`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	byOrder := map[string][]map[string]any{}
	for rows.Next() {
		var orderID, label, code, specimen, notes string
		if err := rows.Scan(&orderID, &label, &code, &specimen, &notes); err != nil {
			return err
		}
		byOrder[orderID] = append(byOrder[orderID], map[string]any{"label": label, "code": code, "specimen": specimen, "notes": notes})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, order := range orders {
		if tests, ok := byOrder[order["id"].(string)]; ok {
			order["tests"] = tests
		}
	}
	return nil
}
