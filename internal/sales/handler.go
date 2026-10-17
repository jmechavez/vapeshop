package sales

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jmechavez/vapeshop/internal/auth"
)

const defaultPerPage = 25

type Handler struct {
	RenderList        func(w http.ResponseWriter, r *http.Request, list []Sale, filter Filter, page Page)
	RenderPOS         func(w http.ResponseWriter, r *http.Request, sale Sale)
	RenderCart        func(w http.ResponseWriter, r *http.Request, sale Sale)
	RenderSuggestions func(w http.ResponseWriter, r *http.Request, saleID int64, products []ProductSuggestion)
	RenderDetail      func(w http.ResponseWriter, r *http.Request, sale Sale)
	RenderReceipt     func(w http.ResponseWriter, r *http.Request, sale Sale)
}

func currentSession(r *http.Request) (*auth.Session, bool) {
	sess := auth.FromContext(r.Context())
	if sess == nil {
		return nil, false
	}
	return sess, true
}

func writeHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Retarget", "#flash")
		w.Header().Set("HX-Reswap", "innerHTML")
	}
	http.Error(w, msg, status)
}

func parseID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}

func (h *Handler) handleRepoError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		h.fail(w, r, http.StatusNotFound, "not found")
	case errors.Is(err, ErrInvalidStatus):
		h.fail(w, r, http.StatusConflict, "sale is not pending")
	case errors.Is(err, ErrInsufficientStock):
		h.fail(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, ErrEmptyCart):
		h.fail(w, r, http.StatusConflict, "add at least one item first")
	case errors.Is(err, ErrVoidReasonMissing):
		h.fail(w, r, http.StatusBadRequest, "void reason is required")
	default:
		h.fail(w, r, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) renderCart(w http.ResponseWriter, r *http.Request, saleID int64) {
	sale, err := GetByID(r.Context(), saleID)
	if err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	writeHTML(w)
	h.RenderCart(w, r, sale)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{
		Status:    q.Get("status"),
		DateFrom:  q.Get("from"),
		DateTo:    q.Get("to"),
		Reference: q.Get("ref"),
	}
	pageNum, _ := strconv.Atoi(q.Get("page"))
	if pageNum < 1 {
		pageNum = 1
	}

	list, total, err := List(r.Context(), f, pageNum, defaultPerPage)
	if err != nil {
		h.fail(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	page := Page{Page: pageNum, PerPage: defaultPerPage, Total: total}
	writeHTML(w)
	h.RenderList(w, r, list, f, page)
}

func (h *Handler) NewSale(w http.ResponseWriter, r *http.Request) {
	sess, ok := currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if raw := r.URL.Query().Get("sale"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if sale, err := GetByID(r.Context(), id); err == nil && sale.Status == StatusPending {
				writeHTML(w)
				h.RenderPOS(w, r, sale)
				return
			}
		}
	}

	id, found, err := FindLatestPendingForUser(r.Context(), sess.UserID)
	if err != nil {
		h.fail(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		id, err = CreatePending(r.Context(), sess.UserID, PaymentCash)
		if err != nil {
			h.fail(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	sale, err := GetByID(r.Context(), id)
	if err != nil {
		h.fail(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeHTML(w)
	h.RenderPOS(w, r, sale)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	sess, ok := currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	method := strings.TrimSpace(r.FormValue("payment_method"))
	if method == "" {
		method = PaymentCash
	}
	id, err := CreatePending(r.Context(), sess.UserID, method)
	if err != nil {
		h.fail(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/sales/new?sale=%d", id), http.StatusSeeOther)
}

func (h *Handler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	saleID, _ := strconv.ParseInt(r.URL.Query().Get("sale"), 10, 64)
	q := r.URL.Query().Get("q")
	products, err := SearchProducts(r.Context(), q)
	if err != nil {
		h.fail(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeHTML(w)
	h.RenderSuggestions(w, r, saleID, products)
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid id")
		return
	}
	sale, err := GetByID(r.Context(), id)
	if err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	writeHTML(w)
	h.RenderDetail(w, r, sale)
}

func (h *Handler) Receipt(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid id")
		return
	}
	sale, err := GetByID(r.Context(), id)
	if err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	writeHTML(w)
	h.RenderReceipt(w, r, sale)
}

func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid sale id")
		return
	}
	pid, err := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if err != nil || pid <= 0 {
		h.fail(w, r, http.StatusBadRequest, "invalid product")
		return
	}
	qty := 1
	if v := r.FormValue("quantity"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			qty = n
		}
	}
	if err := AddItem(r.Context(), id, pid, qty); err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		h.renderCart(w, r, id)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/sales/new?sale=%d", id), http.StatusSeeOther)
}

func (h *Handler) UpdateItemQty(w http.ResponseWriter, r *http.Request) {
	saleID, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid sale id")
		return
	}
	itemID, err := parseID(r, "itemID")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid item id")
		return
	}
	qty, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil || qty < 0 {
		h.fail(w, r, http.StatusBadRequest, "invalid quantity")
		return
	}
	if err := UpdateItemQty(r.Context(), saleID, itemID, qty); err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	h.renderCart(w, r, saleID)
}

func (h *Handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	saleID, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid sale id")
		return
	}
	itemID, err := parseID(r, "itemID")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid item id")
		return
	}
	if err := RemoveItem(r.Context(), saleID, itemID); err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	h.renderCart(w, r, saleID)
}

func (h *Handler) UpdateDiscount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid id")
		return
	}
	disc, err := strconv.ParseFloat(r.FormValue("discount"), 64)
	if err != nil || disc < 0 {
		h.fail(w, r, http.StatusBadRequest, "invalid discount")
		return
	}
	if err := UpdateDiscount(r.Context(), id, disc); err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	h.renderCart(w, r, id)
}

func (h *Handler) UpdatePayment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid id")
		return
	}
	method := strings.TrimSpace(r.FormValue("payment_method"))
	if err := UpdatePayment(r.Context(), id, method); err != nil {
		if strings.HasPrefix(err.Error(), "invalid payment method") {
			h.fail(w, r, http.StatusBadRequest, err.Error())
			return
		}
		h.handleRepoError(w, r, err)
		return
	}
	h.renderCart(w, r, id)
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	sess, ok := currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid id")
		return
	}
	if err := Complete(r.Context(), id, sess.UserID); err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	h.redirectAfterAction(w, r, fmt.Sprintf("/sales/%d", id))
}

func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	sess, ok := currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		h.fail(w, r, http.StatusBadRequest, "invalid id")
		return
	}
	in := VoidInput{
		Reason: strings.TrimSpace(r.FormValue("reason")),
		Note:   strings.TrimSpace(r.FormValue("note")),
	}
	if err := Void(r.Context(), id, sess.UserID, in); err != nil {
		h.handleRepoError(w, r, err)
		return
	}
	h.redirectAfterAction(w, r, fmt.Sprintf("/sales/%d", id))
}

func (h *Handler) redirectAfterAction(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}
