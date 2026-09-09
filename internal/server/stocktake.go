package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

type stockTakeCreatePayload struct {
	Category string `json:"category"`
	Notes    string `json:"notes"`
}

func (s *Server) handleStockTakesList(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	where, args := "1=1", []any{}
	if status != "" {
		where += " AND st.status=?"
		args = append(args, status)
	}
	paging := paginationFrom(r, 50, 200)
	sessionCount, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM stock_take_sessions st WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_LIST_FAILED", "Could not load stock takes.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT st.id,st.stock_take_number,st.status,COALESCE(st.category,''),COALESCE(st.notes,''),st.version,st.started_at,COALESCE(st.completed_at,''),u.display_name,COUNT(si.id),COALESCE(SUM(CASE WHEN si.counted_quantity IS NOT NULL THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN si.counted_quantity IS NOT NULL AND si.counted_quantity<>si.expected_quantity THEN 1 ELSE 0 END),0)
		FROM stock_take_sessions st JOIN users u ON u.id=st.created_by LEFT JOIN stock_take_items si ON si.stock_take_id=st.id
		WHERE `+where+` GROUP BY st.id ORDER BY st.started_at DESC LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_LIST_FAILED", "Could not load stock takes.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, takeStatus, category, notes, startedAt, completedAt, createdBy string
		var version, itemCount, countedCount, differenceCount int
		if err := rows.Scan(&id, &number, &takeStatus, &category, &notes, &version, &startedAt, &completedAt, &createdBy, &itemCount, &countedCount, &differenceCount); err != nil {
			writeError(w, http.StatusInternalServerError, "STOCK_TAKE_LIST_FAILED", "Could not load stock takes.")
			return
		}
		items = append(items, map[string]any{"id": id, "stockTakeNumber": number, "status": takeStatus, "category": category, "notes": notes, "version": version, "startedAt": startedAt, "completedAt": completedAt, "createdBy": createdBy, "itemCount": itemCount, "countedCount": countedCount, "differenceCount": differenceCount})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(sessionCount)))
}

func (s *Server) handleStockTakeGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var number, status, category, notes, startedAt, completedAt, createdBy string
	var version int
	err := s.db.QueryRowContext(r.Context(), `SELECT st.stock_take_number,st.status,COALESCE(st.category,''),COALESCE(st.notes,''),st.version,st.started_at,COALESCE(st.completed_at,''),u.display_name FROM stock_take_sessions st JOIN users u ON u.id=st.created_by WHERE st.id=?`, id).Scan(&number, &status, &category, &notes, &version, &startedAt, &completedAt, &createdBy)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "STOCK_TAKE_NOT_FOUND", "Stock take was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_LOAD_FAILED", "Could not load stock take.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT si.id,si.inventory_item_id,i.sku,i.name,i.category,si.expected_quantity,si.counted_quantity,COALESCE(si.reason,''),si.version,COALESCE(si.counted_at,''),COALESCE(u.display_name,'') FROM stock_take_items si JOIN inventory_items i ON i.id=si.inventory_item_id LEFT JOIN users u ON u.id=si.counted_by WHERE si.stock_take_id=? ORDER BY i.name COLLATE NOCASE`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_LOAD_FAILED", "Could not load stock-take items.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var lineID, inventoryID, sku, name, itemCategory, reason, countedAt, countedBy string
		var expected, lineVersion int
		var counted sql.NullInt64
		if err := rows.Scan(&lineID, &inventoryID, &sku, &name, &itemCategory, &expected, &counted, &reason, &lineVersion, &countedAt, &countedBy); err != nil {
			writeError(w, http.StatusInternalServerError, "STOCK_TAKE_LOAD_FAILED", "Could not load stock-take items.")
			return
		}
		var countedValue any
		var difference any
		if counted.Valid {
			countedValue = int(counted.Int64)
			difference = int(counted.Int64) - expected
		}
		items = append(items, map[string]any{"id": lineID, "inventoryItemId": inventoryID, "sku": sku, "name": name, "category": itemCategory, "expectedQuantity": expected, "countedQuantity": countedValue, "difference": difference, "reason": reason, "version": lineVersion, "countedAt": countedAt, "countedBy": countedBy})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "stockTakeNumber": number, "status": status, "category": category, "notes": notes, "version": version, "startedAt": startedAt, "completedAt": completedAt, "createdBy": createdBy, "items": items})
}

func (s *Server) handleStockTakeCreate(w http.ResponseWriter, r *http.Request) {
	var input stockTakeCreatePayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	validCategories := map[string]bool{"": true, "frame": true, "ophthalmic_lens": true, "contact_lens": true, "accessory": true}
	input.Category = strings.TrimSpace(input.Category)
	if !validCategories[input.Category] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CATEGORY", "Stock-take category is invalid.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	var itemCount int64
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "stock_take", "ST", true)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO stock_take_sessions(id,stock_take_number,status,category,notes,started_at,created_at,updated_at,created_by,updated_by) VALUES(?,?,'in_progress',?,?,?,?,?,?,?)`, id, number, nilIfEmpty(input.Category), nilIfEmpty(input.Notes), now, now, now, user.ID, user.ID); err != nil {
			return err
		}
		query := `INSERT INTO stock_take_items(id,stock_take_id,inventory_item_id,expected_quantity) SELECT lower(hex(randomblob(16))),?,id,quantity FROM inventory_items WHERE archived_at IS NULL AND track_stock=1 AND category<>'service'`
		args := []any{id}
		if input.Category != "" {
			query += " AND category=?"
			args = append(args, input.Category)
		}
		result, err := tx.ExecContext(r.Context(), query, args...)
		if err != nil {
			return err
		}
		itemCount, _ = result.RowsAffected()
		if itemCount == 0 {
			return &APIError{Code: "NO_STOCK_ITEMS", Message: "No tracked inventory items match this stock take."}
		}
		return nil
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_CREATE_FAILED", "Could not start stock take.")
		return
	}
	s.audit(r.Context(), &user, "create", "stock_take", id, "Started stock take "+number, "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "stock_take.changed", EntityType: "stock_take", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "stockTakeNumber": number, "status": "in_progress", "version": 1, "itemCount": itemCount})
}

type stockTakeCountPayload struct {
	CountedQuantity int    `json:"countedQuantity"`
	Reason          string `json:"reason"`
	Version         int    `json:"version"`
}

func (s *Server) handleStockTakeCount(w http.ResponseWriter, r *http.Request) {
	var input stockTakeCountPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 || input.CountedQuantity < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A non-negative count and current line version are required.")
		return
	}
	stockTakeID, itemID := chi.URLParam(r, "id"), chi.URLParam(r, "itemID")
	var expected int
	var status string
	err := s.db.QueryRowContext(r.Context(), `SELECT si.expected_quantity,st.status FROM stock_take_items si JOIN stock_take_sessions st ON st.id=si.stock_take_id WHERE si.id=? AND si.stock_take_id=?`, itemID, stockTakeID).Scan(&expected, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "STOCK_TAKE_ITEM_NOT_FOUND", "Stock-take line was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_COUNT_FAILED", "Could not record stock count.")
		return
	}
	if status != "in_progress" {
		writeError(w, http.StatusLocked, "STOCK_TAKE_LOCKED", "Completed or cancelled stock takes cannot be edited.")
		return
	}
	if expected != input.CountedQuantity && strings.TrimSpace(input.Reason) == "" {
		writeError(w, http.StatusUnprocessableEntity, "DIFFERENCE_REASON_REQUIRED", "A reason is required when the counted quantity differs from expected stock.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE stock_take_items SET counted_quantity=?,reason=?,version=version+1,counted_at=?,counted_by=? WHERE id=? AND stock_take_id=? AND version=?`, input.CountedQuantity, nilIfEmpty(input.Reason), now, user.ID, itemID, stockTakeID, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_COUNT_FAILED", "Could not record stock count.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This count changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "count", "stock_take_item", itemID, "Recorded physical stock count", "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "stock_take.changed", EntityType: "stock_take", EntityID: stockTakeID})
	writeJSON(w, http.StatusOK, map[string]any{"id": itemID, "countedQuantity": input.CountedQuantity, "difference": input.CountedQuantity - expected, "version": input.Version + 1})
}

type stockTakeFinalizePayload struct {
	Version int `json:"version"`
}

func (s *Server) handleStockTakeFinalize(w http.ResponseWriter, r *http.Request) {
	var input stockTakeFinalizePayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Current stock-take version is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	stockTakeID, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	adjustmentCount := 0
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var status string
		var version int
		if err := tx.QueryRowContext(r.Context(), "SELECT status,version FROM stock_take_sessions WHERE id=?", stockTakeID).Scan(&status, &version); err != nil {
			return err
		}
		if version != input.Version {
			return errConcurrentModification
		}
		if status != "in_progress" {
			return &APIError{Code: "STOCK_TAKE_LOCKED", Message: "Only an in-progress stock take can be finalized."}
		}
		var uncounted int
		if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM stock_take_items WHERE stock_take_id=? AND counted_quantity IS NULL", stockTakeID).Scan(&uncounted); err != nil {
			return err
		}
		if uncounted > 0 {
			return &APIError{Code: "STOCK_TAKE_INCOMPLETE", Message: "Every item must be counted before finalization.", Details: map[string]any{"uncounted": uncounted}}
		}
		rows, err := tx.QueryContext(r.Context(), `SELECT inventory_item_id,expected_quantity,counted_quantity,COALESCE(reason,'') FROM stock_take_items WHERE stock_take_id=?`, stockTakeID)
		if err != nil {
			return err
		}
		type adjustment struct {
			inventoryID, reason string
			expected, counted   int
		}
		adjustments := []adjustment{}
		for rows.Next() {
			var row adjustment
			if err := rows.Scan(&row.inventoryID, &row.expected, &row.counted, &row.reason); err != nil {
				rows.Close()
				return err
			}
			adjustments = append(adjustments, row)
		}
		rows.Close()
		for _, row := range adjustments {
			var current, itemVersion int
			if err := tx.QueryRowContext(r.Context(), "SELECT quantity,version FROM inventory_items WHERE id=? AND archived_at IS NULL", row.inventoryID).Scan(&current, &itemVersion); err != nil {
				return err
			}
			if current != row.expected {
				return &APIError{Code: "STOCK_CHANGED_DURING_COUNT", Message: "Inventory changed after this stock take started. Cancel it and start a new count.", Details: map[string]any{"inventoryItemId": row.inventoryID, "expected": row.expected, "current": current}}
			}
			change := row.counted - row.expected
			if change == 0 {
				continue
			}
			result, err := tx.ExecContext(r.Context(), "UPDATE inventory_items SET quantity=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?", row.counted, now, user.ID, row.inventoryID, itemVersion)
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected == 0 {
				return errConcurrentModification
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO stock_movements(id,item_id,movement_type,previous_quantity,quantity_change,resulting_quantity,reason,reference_type,reference_id,created_at,created_by) VALUES(?,?,'correction',?,?,?,?, 'stock_take',?,?,?)`, uuid.NewString(), row.inventoryID, row.expected, change, row.counted, row.reason, stockTakeID, now, user.ID); err != nil {
				return err
			}
			adjustmentCount++
		}
		result, err := tx.ExecContext(r.Context(), `UPDATE stock_take_sessions SET status='completed',version=version+1,completed_at=?,updated_at=?,updated_by=?,completed_by=? WHERE id=? AND version=? AND status='in_progress'`, now, now, user.ID, user.ID, stockTakeID, input.Version)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errConcurrentModification
		}
		return nil
	})
	if err != nil {
		if err == errConcurrentModification {
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Stock take or inventory changed. Reload before finalizing.")
			return
		}
		if apiErr, ok := err.(*APIError); ok {
			status := http.StatusUnprocessableEntity
			if apiErr.Code == "STOCK_CHANGED_DURING_COUNT" {
				status = http.StatusConflict
			}
			writeJSON(w, status, apiErr)
			return
		}
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "STOCK_TAKE_NOT_FOUND", "Stock take was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_FINALIZE_FAILED", "Could not finalize stock take.")
		return
	}
	s.audit(r.Context(), &user, "finalize", "stock_take", stockTakeID, "Finalized stock take", "", marshalJSON(map[string]any{"adjustments": adjustmentCount}), r)
	s.broker.Publish(realtime.Event{Type: "inventory.changed", EntityType: "stock_take", EntityID: stockTakeID})
	s.broker.Publish(realtime.Event{Type: "stock_take.changed", EntityType: "stock_take", EntityID: stockTakeID})
	writeJSON(w, http.StatusOK, map[string]any{"id": stockTakeID, "status": "completed", "version": input.Version + 1, "adjustmentCount": adjustmentCount})
}

func (s *Server) handleStockTakeCancel(w http.ResponseWriter, r *http.Request) {
	var input stockTakeFinalizePayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Current stock-take version is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE stock_take_sessions SET status='cancelled',version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status='in_progress'`, now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STOCK_TAKE_CANCEL_FAILED", "Could not cancel stock take.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Stock take changed since it was opened. Reload before cancelling.")
		return
	}
	s.audit(r.Context(), &user, "cancel", "stock_take", id, "Cancelled stock take", "", "", r)
	s.broker.Publish(realtime.Event{Type: "stock_take.changed", EntityType: "stock_take", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "cancelled", "version": input.Version + 1})
}
