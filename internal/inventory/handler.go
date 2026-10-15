// internal/inventory/handler.go
package inventory

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jmechavez/vapeshop/internal/auth"
)

type Handler struct {
	Repo *Repository

	RenderList           func(w http.ResponseWriter, r *http.Request, rows []InventoryRow)
	RenderRow            func(w http.ResponseWriter, r *http.Request, row InventoryRow)
	RenderReceiveModal   func(w http.ResponseWriter, r *http.Request, row InventoryRow, suppliers []Supplier)
	RenderAdjustModal    func(w http.ResponseWriter, r *http.Request, row InventoryRow)
	RenderMovements      func(w http.ResponseWriter, r *http.Request, movements []Movement, filter MovementFilter)
	RenderMovementsTable func(w http.ResponseWriter, r *http.Request, movements []Movement) // NEW
}

// List GET /inventory
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Repo.List(r.Context())
	if err != nil {
		http.Error(w, "failed to load inventory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderList(w, r, rows)
}

// ReceiveModal GET /inventory/receive/{productID}
func (h *Handler) ReceiveModal(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad product id", http.StatusBadRequest)
		return
	}
	row, err := h.Repo.GetRow(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	suppliers, err := h.Repo.ListSuppliers(r.Context())
	if err != nil {
		http.Error(w, "failed to load suppliers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderReceiveModal(w, r, row, suppliers)
}

// Receive POST /inventory/receive/{productID}
func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad product id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	qty, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
	if err != nil || qty <= 0 {
		http.Error(w, "quantity must be a positive number", http.StatusBadRequest)
		return
	}
	cost := 0.0
	if v := strings.TrimSpace(r.FormValue("unit_cost")); v != "" {
		c, err := strconv.ParseFloat(v, 64)
		if err != nil || c < 0 {
			http.Error(w, "unit_cost must be a number", http.StatusBadRequest)
			return
		}
		cost = c
	}
	var supplierID *int64
	if v := strings.TrimSpace(r.FormValue("supplier_id")); v != "" {
		sid, err := strconv.ParseInt(v, 10, 64)
		if err == nil && sid > 0 {
			supplierID = &sid
		}
	}
	in := ReceiveInput{
		Quantity:   qty,
		UnitCost:   cost,
		SupplierID: supplierID,
		Reference:  strings.TrimSpace(r.FormValue("reference")),
		Notes:      strings.TrimSpace(r.FormValue("notes")),
	}
	var userID int64
	if sess := auth.FromContext(r.Context()); sess != nil {
		userID = sess.UserID
	}
	if err := h.Repo.Receive(r.Context(), id, userID, in); err != nil {
		http.Error(w, "failed to receive stock: "+err.Error(), http.StatusInternalServerError)
		return
	}
	row, err := h.Repo.GetRow(r.Context(), id)
	if err != nil {
		http.Error(w, "saved but failed to reload row: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderRow(w, r, row)
}

// AdjustModal GET /inventory/adjust/{productID}
func (h *Handler) AdjustModal(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad product id", http.StatusBadRequest)
		return
	}
	row, err := h.Repo.GetRow(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	h.RenderAdjustModal(w, r, row)
}

// Adjust POST /inventory/adjust/{productID}
func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad product id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	newQty, err := strconv.Atoi(strings.TrimSpace(r.FormValue("new_quantity")))
	if err != nil || newQty < 0 {
		http.Error(w, "new quantity must be zero or more", http.StatusBadRequest)
		return
	}
	in := AdjustInput{
		NewQuantity: newQty,
		Notes:       strings.TrimSpace(r.FormValue("notes")),
	}
	var userID int64
	if sess := auth.FromContext(r.Context()); sess != nil {
		userID = sess.UserID
	}
	if err := h.Repo.Adjust(r.Context(), id, userID, in); err != nil {
		http.Error(w, "failed to adjust stock: "+err.Error(), http.StatusInternalServerError)
		return
	}
	row, err := h.Repo.GetRow(r.Context(), id)
	if err != nil {
		http.Error(w, "saved but failed to reload row: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderRow(w, r, row)
}

// Movements GET /inventory/movements
//
// If the request is an htmx request, returns only the table (partial).
// Otherwise returns the full page.
func (h *Handler) Movements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := MovementFilter{
		Query:    strings.TrimSpace(q.Get("q")),
		Type:     strings.TrimSpace(q.Get("type")),
		DateFrom: strings.TrimSpace(q.Get("date_from")),
		DateTo:   strings.TrimSpace(q.Get("date_to")),
	}

	movements, err := h.Repo.Movements(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load movements: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		h.RenderMovementsTable(w, r, movements)
		return
	}
	h.RenderMovements(w, r, movements, filter)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "productID"), 10, 64)
}
