package reports

import (
	"net/http"
)

// Handler holds render-function fields that are wired in cmd/server/main.go.
// internal/reports must NOT import views/*.
type Handler struct {
	RenderReports func(w http.ResponseWriter, r *http.Request, report Report)
	RenderSummary func(w http.ResponseWriter, r *http.Request, report Report)
	RenderDash    func(w http.ResponseWriter, r *http.Request, dash Dashboard)
}

// ---------------------------------------------------------------------------
// GET /reports
// ---------------------------------------------------------------------------

func (h *Handler) Reports(w http.ResponseWriter, r *http.Request) {
	rg := Range{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}
	report, err := LoadReport(r.Context(), rg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderReports(w, r, report)
}

// ---------------------------------------------------------------------------
// GET /reports/summary  (htmx fragment — date-range filter on /reports)
//
// Returns only the "content" of the reports page (summary cards + tables +
// chart) without the surrounding Shell, so the filter form can swap it in.
// ---------------------------------------------------------------------------

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	rg := Range{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}
	report, err := LoadReport(r.Context(), rg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderSummary(w, r, report)
}

// ---------------------------------------------------------------------------
// GET /dashboard
// ---------------------------------------------------------------------------

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	dash, err := LoadDashboard(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.RenderDash(w, r, dash)
}
