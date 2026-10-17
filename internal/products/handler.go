package products

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

const defaultPerPage = 25

type Handler struct {
	// All render functions are set by main.go.
	RenderList      func(w http.ResponseWriter, r *http.Request, list []ProductWithStock, q string, page Page)
	RenderSearch    func(w http.ResponseWriter, r *http.Request, list []ProductWithStock, q string, page Page)
	RenderNewModal  func(w http.ResponseWriter, r *http.Request, p Product, errMsg string)
	RenderEditModal func(w http.ResponseWriter, r *http.Request, p Product)
	RenderRow       func(w http.ResponseWriter, r *http.Request, p ProductWithStock)
	RenderRowEdit   func(w http.ResponseWriter, r *http.Request, p ProductWithStock, errMsg string)
	RenderOOBTable  func(w http.ResponseWriter, r *http.Request, list []ProductWithStock)
	RenderImport    func(w http.ResponseWriter, r *http.Request, list []ProductWithStock, created int, failed []string)
}

func parsePage(r *http.Request) (q string, page int) {
	q = strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ = strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if page < 1 {
		page = 1
	}
	return
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	q, pageNum := parsePage(r)
	list, total, err := List(r.Context(), q, pageNum, defaultPerPage)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	page := Page{Page: pageNum, PerPage: defaultPerPage, Total: total}
	h.RenderList(w, r, list, q, page)
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q, pageNum := parsePage(r)
	list, total, err := List(r.Context(), q, pageNum, defaultPerPage)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	page := Page{Page: pageNum, PerPage: defaultPerPage, Total: total}
	h.RenderSearch(w, r, list, q, page)
}

func (h *Handler) NewModal(w http.ResponseWriter, r *http.Request) {
	h.RenderNewModal(w, r, Product{}, "")
}

func (h *Handler) EditModal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	p, err := Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.RenderEditModal(w, r, p.Product)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	p, errMsg := parseForm(r)
	if errMsg != "" {
		h.RenderNewModal(w, r, *p, errMsg)
		return
	}
	if _, err := Create(r.Context(), p); err != nil {
		h.RenderNewModal(w, r, *p, err.Error())
		return
	}
	list, err := ListAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.RenderOOBTable(w, r, list)
}

func (h *Handler) EditInline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.RenderRowEdit(w, r, *p, "")
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, errMsg := parseForm(r)
	p.ID = id

	if errMsg != "" {
		h.RenderRowEdit(w, r, ProductWithStock{Product: *p}, errMsg)
		return
	}
	if err := Update(r.Context(), p); err != nil {
		h.RenderRowEdit(w, r, ProductWithStock{Product: *p}, err.Error())
		return
	}
	full, err := Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.RenderRow(w, r, *full)
}

func (h *Handler) CancelEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	h.RenderRow(w, r, *p)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := SoftDelete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "form too large or invalid", 400)
		return
	}
	file, _, err := r.FormFile("csv")
	if err != nil {
		http.Error(w, "missing csv field", 400)
		return
	}
	defer file.Close()

	rows, err := parseCSV(file)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	created, failed := 0, []string{}
	for i, p := range rows {
		if p.SKU == "" || p.Name == "" {
			failed = append(failed, "row "+strconv.Itoa(i+2)+": sku and name required")
			continue
		}
		if _, err := Create(r.Context(), p); err != nil {
			failed = append(failed, "row "+strconv.Itoa(i+2)+": "+err.Error())
			continue
		}
		created++
	}

	list, _ := ListAll(r.Context())
	h.RenderImport(w, r, list, created, failed)
}

func parseForm(r *http.Request) (*Product, string) {
	_ = r.ParseForm()

	p := &Product{
		SKU:          strings.TrimSpace(r.FormValue("sku")),
		Barcode:      strings.TrimSpace(r.FormValue("barcode")),
		Name:         strings.TrimSpace(r.FormValue("name")),
		Brand:        strings.TrimSpace(r.FormValue("brand")),
		Version:      strings.TrimSpace(r.FormValue("version")),
		Flavor:       strings.TrimSpace(r.FormValue("flavor")),
		Category:     strings.TrimSpace(r.FormValue("category")),
		CostPrice:    parseFloat(r.FormValue("cost_price")),
		SellingPrice: parseFloat(r.FormValue("selling_price")),
		MinimumStock: parseInt(r.FormValue("minimum_stock")),
		Active:       true,
	}

	if p.SKU == "" {
		return p, "SKU is required"
	}
	if p.Name == "" {
		return p, "Name is required"
	}
	if p.SellingPrice < 0 || p.CostPrice < 0 {
		return p, "Prices must be zero or greater"
	}
	return p, ""
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func parseInt(s string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(s))
	return i
}
