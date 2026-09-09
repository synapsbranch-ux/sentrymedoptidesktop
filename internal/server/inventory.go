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

type inventoryPayload struct {
	SKU            string         `json:"sku"`
	Barcode        string         `json:"barcode"`
	Category       string         `json:"category"`
	Name           string         `json:"name"`
	Brand          string         `json:"brand"`
	Model          string         `json:"model"`
	Attributes     map[string]any `json:"attributes"`
	SupplierID     string         `json:"supplierId"`
	CostMinor      int64          `json:"costMinor"`
	SalePriceMinor int64          `json:"salePriceMinor"`
	Currency       string         `json:"currency"`
	Quantity       int            `json:"quantity"`
	ReorderLevel   int            `json:"reorderLevel"`
	TrackStock     bool           `json:"trackStock"`
	ProcedureCode  string         `json:"procedureCode"`
	// A service the clinic sells can also be booked: how long it takes and
	// whether it appears as an appointment type live with the price rather than
	// being retyped on the scheduling screen.
	DurationMinutes int  `json:"durationMinutes"`
	Bookable        bool `json:"bookable"`
}

func (s *Server) handleInventoryList(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	lowStock := r.URL.Query().Get("lowStock") == "true"
	where, args := "archived_at IS NULL", []any{}
	if search != "" {
		like := "%" + search + "%"
		where += " AND (sku LIKE ? OR barcode LIKE ? OR name LIKE ? OR brand LIKE ?)"
		args = append(args, like, like, like, like)
	}
	if category != "" {
		where += " AND category=?"
		args = append(args, category)
	}
	if lowStock {
		where += " AND track_stock=1 AND quantity<=reorder_level"
	}
	paging := paginationFrom(r, 50, 200)
	itemCount, err := s.countRows(r.Context(), "SELECT COUNT(*) FROM inventory_items WHERE "+where, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INVENTORY_LIST_FAILED", "Could not load inventory.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,sku,COALESCE(barcode,''),category,name,COALESCE(brand,''),COALESCE(model,''),attributes_json,COALESCE(supplier_id,''),cost_minor,sale_price_minor,currency,quantity,reorder_level,track_stock,COALESCE(procedure_code,''),duration_minutes,bookable,version,updated_at FROM inventory_items WHERE `+where+` ORDER BY name LIMIT ? OFFSET ?`, paging.Args(args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INVENTORY_LIST_FAILED", "Could not load inventory.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sku, barcode, category, name, brand, model, attributes, supplierID, currency, procedureCode, updatedAt string
		var cost, price int64
		var quantity, reorder, version, duration int
		var tracked, bookable bool
		if err := rows.Scan(&id, &sku, &barcode, &category, &name, &brand, &model, &attributes, &supplierID, &cost, &price, &currency, &quantity, &reorder, &tracked, &procedureCode, &duration, &bookable, &version, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "INVENTORY_LIST_FAILED", "Could not load inventory.")
			return
		}
		items = append(items, map[string]any{"id": id, "sku": sku, "barcode": barcode, "category": category, "name": name, "brand": brand, "model": model, "attributes": rawJSON(attributes), "supplierId": supplierID, "costMinor": cost, "salePriceMinor": price, "currency": currency, "quantity": quantity, "reorderLevel": reorder, "trackStock": tracked, "procedureCode": procedureCode, "durationMinutes": duration, "bookable": bookable, "lowStock": tracked && quantity <= reorder, "version": version, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, withItems(items, paging.Meta(itemCount)))
}

func (s *Server) handleInventoryCreate(w http.ResponseWriter, r *http.Request) {
	var input inventoryPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	validCategories := map[string]bool{"frame": true, "ophthalmic_lens": true, "contact_lens": true, "accessory": true, "service": true}
	input.SKU, input.Name = strings.TrimSpace(input.SKU), strings.TrimSpace(input.Name)
	if input.SKU == "" || input.Name == "" || !validCategories[input.Category] || input.CostMinor < 0 || input.SalePriceMinor < 0 || input.Quantity < 0 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_INVENTORY_ITEM", "SKU, name, valid category, and non-negative amounts are required.")
		return
	}
	if input.DurationMinutes < 0 || input.DurationMinutes > 24*60 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SERVICE_DURATION", "A service duration must be between 0 minutes and 24 hours.")
		return
	}
	if input.Bookable && input.Category != "service" {
		writeError(w, http.StatusUnprocessableEntity, "ONLY_SERVICES_ARE_BOOKABLE", "Only a service can be offered as an appointment type.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(r.Context(), `INSERT INTO inventory_items(id,sku,barcode,category,name,brand,model,attributes_json,supplier_id,cost_minor,sale_price_minor,currency,quantity,reorder_level,track_stock,procedure_code,duration_minutes,bookable,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, input.SKU, nilIfEmpty(input.Barcode), input.Category, input.Name, nilIfEmpty(input.Brand), nilIfEmpty(input.Model), marshalJSON(input.Attributes), nilIfEmpty(input.SupplierID), input.CostMinor, input.SalePriceMinor, strings.ToUpper(input.Currency), input.Quantity, input.ReorderLevel, boolInt(input.TrackStock), nilIfEmpty(input.ProcedureCode), input.DurationMinutes, boolInt(input.Bookable), now, now, user.ID)
		if err != nil {
			return err
		}
		if input.TrackStock && input.Quantity != 0 {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO stock_movements(id,item_id,movement_type,previous_quantity,quantity_change,resulting_quantity,reason,created_at,created_by) VALUES(?,?,'adjustment',0,?,?,'Opening stock',?,?)`, uuid.NewString(), id, input.Quantity, input.Quantity, now, user.ID)
		}
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SKU_OR_BARCODE_IN_USE", "The SKU or barcode already exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "INVENTORY_CREATE_FAILED", "Could not create the inventory item.")
		return
	}
	s.audit(r.Context(), &user, "create", "inventory_item", id, "Created inventory item "+input.SKU, "", "", r)
	s.broker.Publish(realtime.Event{Type: "inventory.changed", EntityType: "inventory_item", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "sku": input.SKU, "version": 1})
}

type movementPayload struct {
	Type     string `json:"type"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
	Version  int    `json:"version"`
}

func (s *Server) handleStockMovement(w http.ResponseWriter, r *http.Request) {
	var input movementPayload
	if err := decodeJSON(r, &input); err != nil || input.Quantity == 0 || input.Version < 1 || strings.TrimSpace(input.Reason) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Movement type, non-zero quantity, reason, and item version are required.")
		return
	}
	valid := map[string]bool{"purchase": true, "sale": true, "return": true, "adjustment": true, "damage": true, "loss": true, "transfer": true, "correction": true}
	if !valid[input.Type] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_MOVEMENT_TYPE", "Stock movement type is invalid.")
		return
	}
	user, _ := userFromContext(r.Context())
	itemID, movementID, now := chi.URLParam(r, "id"), uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var resulting int
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var previous, version int
		var tracked bool
		if err := tx.QueryRowContext(r.Context(), "SELECT quantity,version,track_stock FROM inventory_items WHERE id=? AND archived_at IS NULL", itemID).Scan(&previous, &version, &tracked); err != nil {
			return err
		}
		if version != input.Version {
			return errConcurrentModification
		}
		if !tracked {
			return &APIError{Code: "STOCK_NOT_TRACKED", Message: "This item does not track stock."}
		}
		resulting = previous + input.Quantity
		if resulting < 0 {
			return &APIError{Code: "INSUFFICIENT_STOCK", Message: "Stock cannot become negative."}
		}
		result, err := tx.ExecContext(r.Context(), "UPDATE inventory_items SET quantity=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?", resulting, now, user.ID, itemID, input.Version)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errConcurrentModification
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO stock_movements(id,item_id,movement_type,previous_quantity,quantity_change,resulting_quantity,reason,created_at,created_by) VALUES(?,?,?,?,?,?,?,?,?)`, movementID, itemID, input.Type, previous, input.Quantity, resulting, strings.TrimSpace(input.Reason), now, user.ID)
		return err
	})
	if err != nil {
		if err == errConcurrentModification {
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Inventory changed since it was opened. Reload before adjusting stock.")
			return
		}
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "INVENTORY_ITEM_NOT_FOUND", "Inventory item was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "STOCK_MOVEMENT_FAILED", "Could not record stock movement.")
		return
	}
	s.audit(r.Context(), &user, "stock_movement", "inventory_item", itemID, "Recorded "+input.Type+" stock movement", "", "", r)
	s.broker.Publish(realtime.Event{Type: "inventory.changed", EntityType: "inventory_item", EntityID: itemID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": movementID, "resultingQuantity": resulting, "version": input.Version + 1})
}

func (s *Server) handleStockMovementsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT m.id,m.movement_type,m.previous_quantity,m.quantity_change,m.resulting_quantity,m.reason,COALESCE(m.reference_type,''),COALESCE(m.reference_id,''),u.display_name,m.created_at FROM stock_movements m JOIN users u ON u.id=m.created_by WHERE m.item_id=? ORDER BY m.created_at DESC LIMIT 500`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MOVEMENT_LIST_FAILED", "Could not load stock movements.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, kind, reason, referenceType, referenceID, user, createdAt string
		var previous, change, resulting int
		if err := rows.Scan(&id, &kind, &previous, &change, &resulting, &reason, &referenceType, &referenceID, &user, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "MOVEMENT_LIST_FAILED", "Could not load stock movements.")
			return
		}
		items = append(items, map[string]any{"id": id, "type": kind, "previousQuantity": previous, "quantityChange": change, "resultingQuantity": resulting, "reason": reason, "referenceType": referenceType, "referenceId": referenceID, "user": user, "createdAt": createdAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
