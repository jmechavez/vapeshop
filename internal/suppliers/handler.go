// internal/suppliers/handler.go
package suppliers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jmechavez/vapeshop/internal/auth"
)

// Handler holds render-function fields, injected from main.go.
type Handler struct {
	Repo *Repository

	RenderList      func(w http.ResponseWriter, r *http.Request, list []SupplierWithStats)
	RenderRow       func(w http.ResponseWriter, r *http.Request, s SupplierWithStats)
	RenderNewModal  func(w http.ResponseWriter, r *http.Request, in Input, errMsg string)
	RenderEditModal func(w http.ResponseWriter, r *http.Request, s Supplier, errMsg string)
	RenderDetail    func(w http.ResponseWriter, r *http.Request, s Supplier, purchases []PurchaseRow)
}

// Index GET /suppliers
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	list, err := h.Repo.List(r.Context())
	if err != nil {
		http.Error(w, "failed to load suppliers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderList(w, r, list)
}

// NewModal GET /suppliers/new
func (h *Handler) NewModal(w http.ResponseWriter, r *http.Request) {
	h.RenderNewModal(w, r, Input{}, "")
}

// Create POST /suppliers
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	in, errMsg := parseInput(r)
	if errMsg != "" {
		h.RenderNewModal(w, r, in, errMsg)
		return
	}

	if _, err := h.Repo.Create(r.Context(), in); err != nil {
		if errors.Is(err, ErrInvalid) {
			h.RenderNewModal(w, r, in, "Name is required.")
			return
		}
		http.Error(w, "failed to create supplier: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload full list and return it as an OOB response so the table refreshes.
	list, err := h.Repo.List(r.Context())
	if err != nil {
		http.Error(w, "created but failed to reload list: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderList(w, r, list)
}

// Detail GET /suppliers/{id}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad supplier id", http.StatusBadRequest)
		return
	}

	s, err := h.Repo.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	purchases, err := h.Repo.Purchases(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load purchases: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.RenderDetail(w, r, s, purchases)
}

// EditModal GET /suppliers/{id}/edit
func (h *Handler) EditModal(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad supplier id", http.StatusBadRequest)
		return
	}

	s, err := h.Repo.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	h.RenderEditModal(w, r, s, "")
}

// Update PUT /suppliers/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad supplier id", http.StatusBadRequest)
		return
	}

	in, errMsg := parseInput(r)
	if errMsg != "" {
		s, _ := h.Repo.Get(r.Context(), id)
		h.RenderEditModal(w, r, s, errMsg)
		return
	}

	updated, err := h.Repo.Update(r.Context(), id, in)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			s, _ := h.Repo.Get(r.Context(), id)
			h.RenderEditModal(w, r, s, "Name is required.")
			return
		}
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "supplier not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update supplier: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated row (with stats reloaded).
	list, err := h.Repo.List(r.Context())
	if err != nil {
		http.Error(w, "updated but failed to reload list: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Find the updated row and return just it.
	var stats SupplierWithStats
	for _, s := range list {
		if s.ID == updated.ID {
			stats = s
			break
		}
	}
	h.RenderRow(w, r, stats)
}

// Delete DELETE /suppliers/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "bad supplier id", http.StatusBadRequest)
		return
	}

	if err := h.Repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrInUse) {
			http.Error(w, "Cannot delete: supplier has stock movements.", http.StatusConflict)
			return
		}
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "supplier not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete supplier: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return empty response — htmx will remove the row via hx-target + swap.
	w.WriteHeader(http.StatusOK)
}

// --- helpers ---

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func parseInput(r *http.Request) (Input, string) {
	if err := r.ParseForm(); err != nil {
		return Input{}, "Bad form data."
	}
	in := Input{
		Name:          strings.TrimSpace(r.FormValue("name")),
		ContactPerson: strings.TrimSpace(r.FormValue("contact_person")),
		Phone:         strings.TrimSpace(r.FormValue("phone")),
		Email:         strings.TrimSpace(r.FormValue("email")),
		Address:       strings.TrimSpace(r.FormValue("address")),
	}
	if in.Name == "" {
		return in, "Name is required."
	}
	if len(in.Name) > 200 {
		return in, "Name is too long (max 200)."
	}
	return in, ""
}

// ensure auth import stays used even if you remove the require in this file
var _ = auth.FromContext
