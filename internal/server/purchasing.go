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

type supplierPayload struct {
	Company       string `json:"company"`
	ContactPerson string `json:"contactPerson"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Address       string `json:"address"`
	Notes         string `json:"notes"`
	Version       int    `json:"version"`
}

func (s *Server) handleSuppliersList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT s.id,s.company,COALESCE(s.contact_person,''),COALESCE(s.phone,''),COALESCE(s.email,''),COALESCE(s.address,''),COALESCE(s.notes,''),s.version,s.updated_at,COUNT(i.id)
		FROM suppliers s LEFT JOIN inventory_items i ON i.supplier_id=s.id AND i.archived_at IS NULL
		WHERE s.archived_at IS NULL GROUP BY s.id ORDER BY s.company COLLATE NOCASE`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPPLIER_LIST_FAILED", "Could not load suppliers.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, company, contact, phone, email, address, notes, updatedAt string
		var version, productCount int
		if rows.Scan(&id, &company, &contact, &phone, &email, &address, &notes, &version, &updatedAt, &productCount) == nil {
			items = append(items, map[string]any{"id": id, "company": company, "contactPerson": contact, "phone": phone, "email": email, "address": address, "notes": notes, "productCount": productCount, "version": version, "updatedAt": updatedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func validateSupplier(input *supplierPayload) error {
	input.Company = strings.TrimSpace(input.Company)
	input.Email = strings.TrimSpace(input.Email)
	if input.Company == "" {
		return &APIError{Code: "VALIDATION_ERROR", Message: "Supplier company name is required."}
	}
	if input.Email != "" && !strings.Contains(input.Email, "@") {
		return &APIError{Code: "VALIDATION_ERROR", Message: "Supplier email address is invalid."}
	}
	return nil
}

func (s *Server) handleSupplierCreate(w http.ResponseWriter, r *http.Request) {
	var input supplierPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := validateSupplier(&input); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, err)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO suppliers(id,company,contact_person,phone,email,address,notes,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, input.Company, nilIfEmpty(input.ContactPerson), nilIfEmpty(input.Phone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.Notes), now, now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPPLIER_CREATE_FAILED", "Could not create supplier.")
		return
	}
	s.audit(r.Context(), &user, "create", "supplier", id, "Created supplier "+input.Company, "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "supplier.changed", EntityType: "supplier", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "version": 1})
}

func (s *Server) handleSupplierUpdate(w http.ResponseWriter, r *http.Request) {
	var input supplierPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Supplier version is required.")
		return
	}
	if err := validateSupplier(&input); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, err)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE suppliers SET company=?,contact_person=?,phone=?,email=?,address=?,notes=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, input.Company, nilIfEmpty(input.ContactPerson), nilIfEmpty(input.Phone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.Notes), now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPPLIER_UPDATE_FAILED", "Could not update supplier.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Supplier changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "supplier", id, "Updated supplier "+input.Company, "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "supplier.changed", EntityType: "supplier", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

func (s *Server) handleSupplierArchive(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if version == "" {
		writeError(w, http.StatusBadRequest, "VERSION_REQUIRED", "Supplier version is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE suppliers SET archived_at=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, now, now, user.ID, id, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUPPLIER_ARCHIVE_FAILED", "Could not archive supplier.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Supplier changed since it was opened. Reload before archiving.")
		return
	}
	s.audit(r.Context(), &user, "archive", "supplier", id, "Archived supplier", "", "", r)
	s.broker.Publish(realtime.Event{Type: "supplier.changed", EntityType: "supplier", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
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

func (s *Server) handlePurchaseOrdersList(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	where, args := "1=1", []any{}
	if status != "" {
		where += " AND po.status=?"
		args = append(args, status)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT po.id,po.order_number,po.supplier_id,s.company,po.status,po.currency,COALESCE(po.expected_at,''),COALESCE(po.notes,''),po.version,po.created_at,po.updated_at,COUNT(poi.id),COALESCE(SUM(poi.quantity_ordered),0),COALESCE(SUM(poi.quantity_received),0),COALESCE(SUM(poi.quantity_ordered*poi.unit_cost_minor),0)
		FROM purchase_orders po JOIN suppliers s ON s.id=po.supplier_id LEFT JOIN purchase_order_items poi ON poi.purchase_order_id=po.id
		WHERE `+where+` GROUP BY po.id ORDER BY po.created_at DESC LIMIT 500`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PURCHASE_ORDER_LIST_FAILED", "Could not load purchase orders.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, supplierID, supplierName, orderStatus, currency, expectedAt, notes, createdAt, updatedAt string
		var version, lineCount, ordered, received int
		var total int64
		if rows.Scan(&id, &number, &supplierID, &supplierName, &orderStatus, &currency, &expectedAt, &notes, &version, &createdAt, &updatedAt, &lineCount, &ordered, &received, &total) == nil {
			items = append(items, map[string]any{"id": id, "orderNumber": number, "supplierId": supplierID, "supplierName": supplierName, "status": orderStatus, "currency": currency, "expectedAt": expectedAt, "notes": notes, "version": version, "createdAt": createdAt, "updatedAt": updatedAt, "lineCount": lineCount, "quantityOrdered": ordered, "quantityReceived": received, "totalMinor": total})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePurchaseOrderGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var number, supplierID, supplierName, status, currency, expectedAt, notes, createdAt, updatedAt string
	var version int
	err := s.db.QueryRowContext(r.Context(), `SELECT po.order_number,po.supplier_id,s.company,po.status,po.currency,COALESCE(po.expected_at,''),COALESCE(po.notes,''),po.version,po.created_at,po.updated_at FROM purchase_orders po JOIN suppliers s ON s.id=po.supplier_id WHERE po.id=?`, id).Scan(&number, &supplierID, &supplierName, &status, &currency, &expectedAt, &notes, &version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PURCHASE_ORDER_NOT_FOUND", "Purchase order was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PURCHASE_ORDER_LOAD_FAILED", "Could not load purchase order.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT poi.id,poi.inventory_item_id,i.sku,i.name,poi.quantity_ordered,poi.quantity_received,poi.unit_cost_minor FROM purchase_order_items poi JOIN inventory_items i ON i.id=poi.inventory_item_id WHERE poi.purchase_order_id=? ORDER BY i.name`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PURCHASE_ORDER_LOAD_FAILED", "Could not load purchase order lines.")
		return
	}
	defer rows.Close()
	items, total := []map[string]any{}, int64(0)
	for rows.Next() {
		var lineID, inventoryID, sku, name string
		var ordered, received int
		var unitCost int64
		if rows.Scan(&lineID, &inventoryID, &sku, &name, &ordered, &received, &unitCost) == nil {
			total += int64(ordered) * unitCost
			items = append(items, map[string]any{"id": lineID, "inventoryItemId": inventoryID, "sku": sku, "name": name, "quantityOrdered": ordered, "quantityReceived": received, "remainingQuantity": ordered - received, "unitCostMinor": unitCost})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "orderNumber": number, "supplierId": supplierID, "supplierName": supplierName, "status": status, "currency": currency, "expectedAt": expectedAt, "notes": notes, "version": version, "createdAt": createdAt, "updatedAt": updatedAt, "totalMinor": total, "items": items})
}

func (s *Server) handlePurchaseOrderCreate(w http.ResponseWriter, r *http.Request) {
	var input purchaseOrderPayload
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.SupplierID) == "" || len(input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Supplier and at least one purchase item are required.")
		return
	}
	if input.Currency == "" {
		input.Currency = "HTG"
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.Currency != "HTG" && input.Currency != "USD" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CURRENCY", "Purchase order currency must be HTG or USD.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var supplierExists int
		if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM suppliers WHERE id=? AND archived_at IS NULL", input.SupplierID).Scan(&supplierExists); err != nil || supplierExists != 1 {
			return &APIError{Code: "SUPPLIER_NOT_FOUND", Message: "Supplier was not found or is archived."}
		}
		seen := map[string]bool{}
		for _, item := range input.Items {
			if item.InventoryItemID == "" || item.Quantity <= 0 || item.UnitCostMinor < 0 || seen[item.InventoryItemID] {
				return &APIError{Code: "INVALID_PURCHASE_ITEM", Message: "Each inventory item must appear once with a positive quantity and non-negative cost."}
			}
			seen[item.InventoryItemID] = true
		}
		var err error
		number, err = s.nextNumber(r.Context(), tx, "purchase_order", "PO", true)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO purchase_orders(id,order_number,supplier_id,status,currency,expected_at,notes,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,'draft',?,?,?,?,?,?,?)`, id, number, input.SupplierID, input.Currency, nilIfEmpty(input.ExpectedAt), nilIfEmpty(input.Notes), now, now, user.ID, user.ID); err != nil {
			return err
		}
		for _, item := range input.Items {
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO purchase_order_items(id,purchase_order_id,inventory_item_id,quantity_ordered,unit_cost_minor) SELECT ?,?,?,?,? WHERE EXISTS (SELECT 1 FROM inventory_items WHERE id=? AND archived_at IS NULL AND track_stock=1)`, uuid.NewString(), id, item.InventoryItemID, item.Quantity, item.UnitCostMinor, item.InventoryItemID); err != nil {
				return err
			}
			var found int
			if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM purchase_order_items WHERE purchase_order_id=? AND inventory_item_id=?", id, item.InventoryItemID).Scan(&found); err != nil || found != 1 {
				return &APIError{Code: "INVENTORY_ITEM_NOT_FOUND", Message: "A purchase item was not found or does not track stock."}
			}
		}
		return nil
	})
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		writeError(w, http.StatusInternalServerError, "PURCHASE_ORDER_FAILED", "Could not create purchase order.")
		return
	}
	s.audit(r.Context(), &user, "create", "purchase_order", id, "Created purchase order "+number, "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "purchase_order.changed", EntityType: "purchase_order", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "orderNumber": number, "status": "draft", "version": 1})
}

type purchaseOrderStatusPayload struct {
	Status  string `json:"status"`
	Version int    `json:"version"`
}

func (s *Server) handlePurchaseOrderStatus(w http.ResponseWriter, r *http.Request) {
	var input purchaseOrderStatusPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 || (input.Status != "sent" && input.Status != "cancelled") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid status and current purchase-order version are required.")
		return
	}
	orderID := chi.URLParam(r, "id")
	var current string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM purchase_orders WHERE id=?", orderID).Scan(&current); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PURCHASE_ORDER_NOT_FOUND", "Purchase order was not found.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "PURCHASE_ORDER_STATUS_FAILED", "Could not update purchase order.")
		return
	}
	valid := (current == "draft" && (input.Status == "sent" || input.Status == "cancelled")) || (current == "sent" && input.Status == "cancelled")
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_STATUS_TRANSITION", "This purchase order cannot move from "+current+" to "+input.Status+".")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), "UPDATE purchase_orders SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status=?", input.Status, now, user.ID, orderID, input.Version, current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PURCHASE_ORDER_STATUS_FAILED", "Could not update purchase order.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Purchase order changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "status_change", "purchase_order", orderID, "Changed purchase order status to "+input.Status, current, input.Status, r)
	s.broker.Publish(realtime.Event{Type: "purchase_order.changed", EntityType: "purchase_order", EntityID: orderID})
	writeJSON(w, http.StatusOK, map[string]any{"id": orderID, "status": input.Status, "version": input.Version + 1})
}

type receivePayload struct {
	Version int    `json:"version"`
	Notes   string `json:"notes"`
	Items   []struct {
		ItemID   string `json:"itemId"`
		Quantity int    `json:"quantity"`
	} `json:"items"`
}

func (s *Server) handlePurchaseOrderReceive(w http.ResponseWriter, r *http.Request) {
	var input receivePayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 || len(input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Current purchase-order version and at least one received item are required.")
		return
	}
	user, _ := userFromContext(r.Context())
	orderID, receiptID, now := chi.URLParam(r, "id"), uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	finalStatus := "partial"
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var status string
		var version int
		if err := tx.QueryRowContext(r.Context(), "SELECT status,version FROM purchase_orders WHERE id=?", orderID).Scan(&status, &version); err != nil {
			return err
		}
		if version != input.Version {
			return errConcurrentModification
		}
		if status != "sent" && status != "partial" {
			return &APIError{Code: "PURCHASE_ORDER_NOT_RECEIVABLE", Message: "Only sent or partially received orders can receive stock."}
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO purchase_order_receipts(id,purchase_order_id,notes,received_at,received_by) VALUES(?,?,?,?,?)`, receiptID, orderID, nilIfEmpty(input.Notes), now, user.ID); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, received := range input.Items {
			if received.Quantity <= 0 || seen[received.ItemID] {
				return &APIError{Code: "INVALID_RECEIVED_QUANTITY", Message: "Each received line must appear once with a positive quantity."}
			}
			seen[received.ItemID] = true
			var inventoryID string
			var ordered, alreadyReceived int
			if err := tx.QueryRowContext(r.Context(), "SELECT inventory_item_id,quantity_ordered,quantity_received FROM purchase_order_items WHERE id=? AND purchase_order_id=?", received.ItemID, orderID).Scan(&inventoryID, &ordered, &alreadyReceived); err != nil {
				return err
			}
			if alreadyReceived+received.Quantity > ordered {
				return &APIError{Code: "RECEIPT_EXCEEDS_ORDER", Message: "Received quantity exceeds ordered quantity."}
			}
			var previous, itemVersion int
			if err := tx.QueryRowContext(r.Context(), "SELECT quantity,version FROM inventory_items WHERE id=? AND archived_at IS NULL AND track_stock=1", inventoryID).Scan(&previous, &itemVersion); err != nil {
				return err
			}
			resulting := previous + received.Quantity
			if _, err := tx.ExecContext(r.Context(), "UPDATE purchase_order_items SET quantity_received=quantity_received+? WHERE id=?", received.Quantity, received.ItemID); err != nil {
				return err
			}
			result, err := tx.ExecContext(r.Context(), "UPDATE inventory_items SET quantity=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?", resulting, now, user.ID, inventoryID, itemVersion)
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected == 0 {
				return errConcurrentModification
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO purchase_order_receipt_items(id,receipt_id,purchase_order_item_id,quantity_received) VALUES(?,?,?,?)`, uuid.NewString(), receiptID, received.ItemID, received.Quantity); err != nil {
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
		if remaining == 0 {
			finalStatus = "received"
		}
		result, err := tx.ExecContext(r.Context(), "UPDATE purchase_orders SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status=?", finalStatus, now, user.ID, orderID, input.Version, status)
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
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Purchase order or stock changed since it was opened. Reload before receiving.")
			return
		}
		if apiErr, ok := err.(*APIError); ok {
			writeJSON(w, http.StatusUnprocessableEntity, apiErr)
			return
		}
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "PURCHASE_ORDER_ITEM_NOT_FOUND", "Purchase order or a receipt line was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "PURCHASE_RECEIPT_FAILED", "Could not receive purchase order stock.")
		return
	}
	s.audit(r.Context(), &user, "receive", "purchase_order", orderID, "Received purchase order stock", "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "inventory.changed", EntityType: "purchase_order", EntityID: orderID})
	s.broker.Publish(realtime.Event{Type: "purchase_order.changed", EntityType: "purchase_order", EntityID: orderID})
	writeJSON(w, http.StatusOK, map[string]any{"id": orderID, "receiptId": receiptID, "status": finalStatus, "version": input.Version + 1})
}
