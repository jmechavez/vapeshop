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

const defaultPerPage = 25

type Handler struct {
	Repo *Repository

	RenderList           func(w http.ResponseWriter, r *http.Request, rows []InventoryRow, filter ListFilter, page Page)
	RenderListOOB        func(w http.ResponseWriter, r *http.Request, rows []InventoryRow)
	RenderRow            func(w http.ResponseWriter, r *http.Request, row InventoryRow)
	RenderReceiveModal   func(w http.ResponseWriter, r *http.Request, row InventoryRow, suppliers []Supplier)
	RenderAdjustModal    func(w http.ResponseWriter, r *http.Request, row InventoryRow)
	RenderMovements      func(w http.ResponseWriter, r *http.Request, movements []Movement, filter MovementFilter)
	RenderMovementsTable func(w http.ResponseWriter, r *http.Request, movements []Movement)
}

// parseListFilter reads ?q=, ?low=, ?page= into a ListFilter.
func parseListFilter(r *http.Request) ListFilter {
	q := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(q.Get("page")))
	if page < 1 {
		page = 1
	}
	isTruthy := func(v string) bool { return v == "1" || v == "true" || v == "on" }
	return ListFilter{
		Query:   strings.TrimSpace(q.Get("q")),
		LowOnly: isTruthy(q.Get("low")),
		Page:    page,
		PerPage: defaultPerPage,
	}
}

// List GET /inventory — full page.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f := parseListFilter(r)
	rows, total, err := h.Repo.List(r.Context(), f)
	if err != nil {
		http.Error(w, "failed to load inventory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	page := Page{Page: f.Page, PerPage: f.PerPage, Total: total}
	h.RenderList(w, r, rows, f, page)
}

// Search GET /inventory/search — htmx fragment used by live search, filter,
// and pagination.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	f := parseListFilter(r)
	rows, total, err := h.Repo.List(r.Context(), f)
	if err != nil {
		http.Error(w, "failed to search inventory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	page := Page{Page: f.Page, PerPage: f.PerPage, Total: total}
	h.RenderList(w, r, rows, f, page)
}

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
	rows, err := h.Repo.ListAll(r.Context())
	if err != nil {
		http.Error(w, "saved but failed to reload list: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderListOOB(w, r, rows)
}

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
	rows, err := h.Repo.ListAll(r.Context())
	if err != nil {
		http.Error(w, "saved but failed to reload list: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderListOOB(w, r, rows)
}

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
