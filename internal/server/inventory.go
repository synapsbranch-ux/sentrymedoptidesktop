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
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,sku,COALESCE(barcode,''),category,name,COALESCE(brand,''),COALESCE(model,''),attributes_json,COALESCE(supplier_id,''),cost_minor,sale_price_minor,currency,quantity,reorder_level,track_stock,version,updated_at FROM inventory_items WHERE `+where+` ORDER BY name LIMIT 1000`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INVENTORY_LIST_FAILED", "Could not load inventory.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sku, barcode, category, name, brand, model, attributes, supplierID, currency, updatedAt string
		var cost, price int64
		var quantity, reorder, version int
		var tracked bool
		if rows.Scan(&id, &sku, &barcode, &category, &name, &brand, &model, &attributes, &supplierID, &cost, &price, &currency, &quantity, &reorder, &tracked, &version, &updatedAt) == nil {
			items = append(items, map[string]any{"id": id, "sku": sku, "barcode": barcode, "category": category, "name": name, "brand": brand, "model": model, "attributes": rawJSON(attributes), "supplierId": supplierID, "costMinor": cost, "salePriceMinor": price, "currency": currency, "quantity": quantity, "reorderLevel": reorder, "trackStock": tracked, "lowStock": tracked && quantity <= reorder, "version": version, "updatedAt": updatedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
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
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(r.Context(), `INSERT INTO inventory_items(id,sku,barcode,category,name,brand,model,attributes_json,supplier_id,cost_minor,sale_price_minor,currency,quantity,reorder_level,track_stock,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, input.SKU, nilIfEmpty(input.Barcode), input.Category, input.Name, nilIfEmpty(input.Brand), nilIfEmpty(input.Model), marshalJSON(input.Attributes), nilIfEmpty(input.SupplierID), input.CostMinor, input.SalePriceMinor, strings.ToUpper(input.Currency), input.Quantity, input.ReorderLevel, boolInt(input.TrackStock), now, now, user.ID)
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
		if rows.Scan(&id, &kind, &previous, &change, &resulting, &reason, &referenceType, &referenceID, &user, &createdAt) == nil {
			items = append(items, map[string]any{"id": id, "type": kind, "previousQuantity": previous, "quantityChange": change, "resultingQuantity": resulting, "reason": reason, "referenceType": referenceType, "referenceId": referenceID, "user": user, "createdAt": createdAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type supplierPayload struct {
	Company       string `json:"company"`
	ContactPerson string `json:"contactPerson"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Address       string `json:"address"`
	Notes         string `json:"notes"`
}

func (s *Server) handleSuppliersList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,company,COALESCE(contact_person,''),COALESCE(phone,''),COALESCE(email,''),COALESCE(address,''),COALESCE(notes,''),version,updated_at FROM suppliers WHERE archived_at IS NULL ORDER BY company`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPPLIER_LIST_FAILED", "Could not load suppliers.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, company, contact, phone, email, address, notes, updatedAt string
		var version int
		if rows.Scan(&id, &company, &contact, &phone, &email, &address, &notes, &version, &updatedAt) == nil {
			items = append(items, map[string]any{"id": id, "company": company, "contactPerson": contact, "phone": phone, "email": email, "address": address, "notes": notes, "version": version, "updatedAt": updatedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSupplierCreate(w http.ResponseWriter, r *http.Request) {
	var input supplierPayload
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.Company) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Supplier company name is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO suppliers(id,company,contact_person,phone,email,address,notes,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, strings.TrimSpace(input.Company), nilIfEmpty(input.ContactPerson), nilIfEmpty(input.Phone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.Notes), now, now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPPLIER_CREATE_FAILED", "Could not create supplier.")
		return
	}
	s.audit(r.Context(), &user, "create", "supplier", id, "Created supplier", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "version": 1})
}

type purchaseOrderPayload struct {
	SupplierID string `json:"supplierId"`
	Currency   string `json:"currency"`
	ExpectedAt string `json:"expectedAt"`
	Notes      string `json:"notes"`
	Items      []struct {
		InventoryItemID string `json:"inventoryItemId"`
		Quantity        int    `json:"quantity"`
		UnitCostMinor   int64  `json:"unitCostMinor"`
	} `json:"items"`
}

func (s *Server) handlePurchaseOrderCreate(w http.ResponseWriter, r *http.Request) {
	var input purchaseOrderPayload
	if err := decodeJSON(r, &input); err != nil || input.SupplierID == "" || len(input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Supplier and at least one purchase item are required.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "purchase_order", "PO", true)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO purchase_orders(id,order_number,supplier_id,status,currency,expected_at,notes,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,'draft',?,?,?,?,?,?,?)`, id, number, input.SupplierID, strings.ToUpper(input.Currency), nilIfEmpty(input.ExpectedAt), nilIfEmpty(input.Notes), now, now, user.ID, user.ID); err != nil {
			return err
		}
		for _, item := range input.Items {
			if item.InventoryItemID == "" || item.Quantity <= 0 || item.UnitCostMinor < 0 {
				return &APIError{Code: "INVALID_PURCHASE_ITEM", Message: "Purchase quantities must be positive."}
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO purchase_order_items(id,purchase_order_id,inventory_item_id,quantity_ordered,unit_cost_minor) VALUES(?,?,?,?,?)`, uuid.NewString(), id, item.InventoryItemID, item.Quantity, item.UnitCostMinor); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "PURCHASE_ORDER_FAILED", err.Error())
		return
	}
	s.audit(r.Context(), &user, "create", "purchase_order", id, "Created purchase order "+number, "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "orderNumber": number, "status": "draft", "version": 1})
}

type receivePayload struct {
	Items []struct {
		ItemID   string `json:"itemId"`
		Quantity int    `json:"quantity"`
	} `json:"items"`
}

func (s *Server) handlePurchaseOrderReceive(w http.ResponseWriter, r *http.Request) {
	var input receivePayload
	if err := decodeJSON(r, &input); err != nil || len(input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "At least one received item is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	orderID, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		for _, received := range input.Items {
			if received.Quantity <= 0 {
				return &APIError{Code: "INVALID_RECEIVED_QUANTITY", Message: "Received quantity must be positive."}
			}
			var inventoryID string
			var ordered, alreadyReceived int
			if err := tx.QueryRowContext(r.Context(), "SELECT inventory_item_id,quantity_ordered,quantity_received FROM purchase_order_items WHERE id=? AND purchase_order_id=?", received.ItemID, orderID).Scan(&inventoryID, &ordered, &alreadyReceived); err != nil {
				return err
			}
			if alreadyReceived+received.Quantity > ordered {
				return &APIError{Code: "RECEIPT_EXCEEDS_ORDER", Message: "Received quantity exceeds ordered quantity."}
			}
			var previous int
			if err := tx.QueryRowContext(r.Context(), "SELECT quantity FROM inventory_items WHERE id=?", inventoryID).Scan(&previous); err != nil {
				return err
			}
			resulting := previous + received.Quantity
			if _, err := tx.ExecContext(r.Context(), "UPDATE purchase_order_items SET quantity_received=quantity_received+? WHERE id=?", received.Quantity, received.ItemID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), "UPDATE inventory_items SET quantity=?,version=version+1,updated_at=?,updated_by=? WHERE id=?", resulting, now, user.ID, inventoryID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO stock_movements(id,item_id,movement_type,previous_quantity,quantity_change,resulting_quantity,reason,reference_type,reference_id,created_at,created_by) VALUES(?,?,'purchase',?,?,?,'Purchase order receipt','purchase_order',?,?,?)`, uuid.NewString(), inventoryID, previous, received.Quantity, resulting, orderID, now, user.ID); err != nil {
				return err
			}
		}
		var remaining int
		if err := tx.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(quantity_ordered-quantity_received),0) FROM purchase_order_items WHERE purchase_order_id=?", orderID).Scan(&remaining); err != nil {
			return err
		}
		status := "partial"
		if remaining == 0 {
			status = "received"
		}
		_, err := tx.ExecContext(r.Context(), "UPDATE purchase_orders SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND status NOT IN ('cancelled','received')", status, now, user.ID, orderID)
		return err
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "PURCHASE_RECEIPT_FAILED", "Could not receive purchase order stock.")
		return
	}
	s.audit(r.Context(), &user, "receive", "purchase_order", orderID, "Received purchase order stock", "", "", r)
	s.broker.Publish(realtime.Event{Type: "inventory.changed", EntityType: "purchase_order", EntityID: orderID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}
