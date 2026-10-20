package reports

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
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

func (h *Handler) SendZReport(w http.ResponseWriter, r *http.Request) {
	// 1. Load Dashboard data (which now has the Z-Report totals)
	dash, err := LoadDashboard(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Build the Telegram message
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if botToken == "" || chatID == "" {
		http.Error(w, "Telegram not configured", http.StatusInternalServerError)
		return
	}

	msg := fmt.Sprintf("🛑 *END OF DAY SUMMARY*\n\n")
	msg += fmt.Sprintf("📅 *Today's Sales:* %d\n", dash.TodaySalesCount)
	msg += fmt.Sprintf("• Cash: ₱%.2f\n", dash.TodayPayments.Cash)
	msg += fmt.Sprintf("• GCash: ₱%.2f\n", dash.TodayPayments.GCash)
	msg += fmt.Sprintf("• Maribank: ₱%.2f\n", dash.TodayPayments.Maribank)
	msg += fmt.Sprintf("• Bank Transfer: ₱%.2f\n", dash.TodayPayments.BankTransfer)
	msg += fmt.Sprintf("*TOTAL COLLECTED:* ₱%.2f\n\n", dash.TodayPayments.Total())

	if dash.TodayPartnerStockTotal > 0 {
		msg += fmt.Sprintf("🤝 *Partner Stock (To Pay):*\n")
		msg += fmt.Sprintf("• Items Sold: %d\n", dash.TodayPartnerStockCount)
		msg += fmt.Sprintf("• Amount: ₱%.2f\n\n", dash.TodayPartnerStockTotal)
	}

	msg += fmt.Sprintf("❌ *Voids:* %d\n", dash.TodayVoidsCount)

	// 3. Send to Telegram
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	data := url.Values{}
	data.Set("chat_id", chatID)
	data.Set("text", msg)
	data.Set("parse_mode", "Markdown")

	resp, err := http.PostForm(apiURL, data)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to send to Telegram", http.StatusInternalServerError)
		return
	}

	// 4. Render the same summary as an HTML modal
	h.RenderDash(w, r, dash) // (The modal will be built into the Dashboard templ)
}
