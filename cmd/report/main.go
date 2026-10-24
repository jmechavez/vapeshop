package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentStats struct {
	Cash         float64
	GCash        float64
	Maribank     float64
	BankTransfer float64
}

func (p PaymentStats) Total() float64 {
	return p.Cash + p.GCash + p.Maribank + p.BankTransfer
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if dbURL == "" || botToken == "" || chatID == "" {
		log.Fatal("Missing environment variables.")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Use Philippine Time
	loc, _ := time.LoadLocation("Asia/Manila")
	now := time.Now().In(loc)

	// GitHub Actions cron is unreliable and often runs late.
	// We run the action every hour, but ONLY execute if it is exactly 7 AM or 10 PM PH time.
	isMorning := false

	if now.Hour() == 7 {
		isMorning = true
	} else if now.Hour() != 22 { // 10 PM
		// If it's not 7 AM AND not 10 PM, skip.
		log.Printf("Skipping report. Current PH hour is %d (only runs at 7 AM or 10 PM).", now.Hour())
		return
	}

	var reportDate time.Time
	if isMorning {
		reportDate = now.AddDate(0, 0, -1) // Morning reports for Yesterday
	} else {
		reportDate = now // Evening reports for Today
	}

	startOfDay := time.Date(reportDate.Year(), reportDate.Month(), reportDate.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	startOfMonth := time.Date(reportDate.Year(), reportDate.Month(), 1, 0, 0, 0, 0, loc)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// --- 1. Fetch Today's Stats ---
	todayPayments := getPaymentStats(ctx, pool, startOfDay, endOfDay)
	var todaySalesCount int
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sales 
		WHERE status = 'completed' 
		  AND (sale_date AT TIME ZONE 'Asia/Manila') >= $1 
		  AND (sale_date AT TIME ZONE 'Asia/Manila') < $2
	`, startOfDay, endOfDay).Scan(&todaySalesCount)

	// --- 2. Fetch Month's Stats ---
	monthPayments := getPaymentStats(ctx, pool, startOfMonth, endOfMonth)
	var monthSalesCount int
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sales 
		WHERE status = 'completed' 
		  AND (sale_date AT TIME ZONE 'Asia/Manila') >= $1 
		  AND (sale_date AT TIME ZONE 'Asia/Manila') < $2
	`, startOfMonth, endOfMonth).Scan(&monthSalesCount)

	// --- 3. Fetch Partner Stock Sales (Today) ---
	var partnerTotal float64
	var partnerCount int
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(si.subtotal), 0), COUNT(DISTINCT s.id)
		FROM sale_items si
		JOIN sales s ON s.id = si.sale_id
		JOIN products p ON p.id = si.product_id
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') >= $1 
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') < $2
		  AND p.is_partner_stock = TRUE
	`, startOfDay, endOfDay).Scan(&partnerTotal, &partnerCount)

	// --- 4. Build the Message ---
	var sb strings.Builder
	if isMorning {
		sb.WriteString(fmt.Sprintf("🌅 *Morning Reminder (%s)*\n\n", reportDate.Format("Jan 02, 2006")))
	} else {
		sb.WriteString(fmt.Sprintf("🌙 *Evening Report (%s)*\n\n", reportDate.Format("Jan 02, 2006")))
	}

	// Today
	sb.WriteString(fmt.Sprintf("📅 *MGA HALIN %s*\n", reportDate.Format("Jan 02, 2006")))
	sb.WriteString(fmt.Sprintf("• Sales Count: %d\n", todaySalesCount))
	sb.WriteString(fmt.Sprintf("• Cash: ₱%.2f\n", todayPayments.Cash))
	sb.WriteString(fmt.Sprintf("• GCash: ₱%.2f\n", todayPayments.GCash))
	sb.WriteString(fmt.Sprintf("• Maribank: ₱%.2f\n", todayPayments.Maribank))
	sb.WriteString(fmt.Sprintf("• Bank Transfer: ₱%.2f\n", todayPayments.BankTransfer))
	sb.WriteString(fmt.Sprintf("*TOTAL:* ₱%.2f\n\n", todayPayments.Total()))

	// Partner Stock (only shows if there are sales)
	if partnerTotal > 0 {
		sb.WriteString(fmt.Sprintf("🤝 *MGA HALIN SA STOCK NI JOAN(ATE) %s*\n", reportDate.Format("Jan 02, 2006")))
		sb.WriteString(fmt.Sprintf("• Items Sold: %d\n", partnerCount))
		sb.WriteString(fmt.Sprintf("• Total: ₱%.2f\n", partnerTotal))
		sb.WriteString("*This amount belongs to JOAN(ATE).*\n\n")
	}

	// This Month
	sb.WriteString("📊 *MGA HALIN KARONG BULANA*\n")
	sb.WriteString(fmt.Sprintf("• Sales Count: %d\n", monthSalesCount))
	sb.WriteString(fmt.Sprintf("• Cash: ₱%.2f\n", monthPayments.Cash))
	sb.WriteString(fmt.Sprintf("• GCash: ₱%.2f\n", monthPayments.GCash))
	sb.WriteString(fmt.Sprintf("• Maribank: ₱%.2f\n", monthPayments.Maribank))
	sb.WriteString(fmt.Sprintf("• Bank Transfer: ₱%.2f\n", monthPayments.BankTransfer))
	sb.WriteString(fmt.Sprintf("*TOTAL:* ₱%.2f\n\n", monthPayments.Total()))

	// 5. Send to Telegram
	sendTelegramMessage(botToken, chatID, sb.String())
}

func getPaymentStats(ctx context.Context, pool *pgxpool.Pool, start, end time.Time) PaymentStats {
	var p PaymentStats
	rows, err := pool.Query(ctx, `
		SELECT sp.method, SUM(sp.amount)::float8
		FROM sale_payments sp
		JOIN sales s ON s.id = sp.sale_id
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') >= $1 
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') < $2
		GROUP BY sp.method
	`, start, end)
	if err != nil {
		return p
	}
	defer rows.Close()

	for rows.Next() {
		var method string
		var amount float64
		if err := rows.Scan(&method, &amount); err == nil {
			switch method {
			case "Cash":
				p.Cash = amount
			case "GCash":
				p.GCash = amount
			case "Maribank":
				p.Maribank = amount
			case "Bank Transfer":
				p.BankTransfer = amount
			}
		}
	}
	return p
}

func sendTelegramMessage(token, chatID, text string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	data := url.Values{}
	data.Set("chat_id", chatID)
	data.Set("text", text)
	data.Set("parse_mode", "Markdown")

	resp, err := http.PostForm(apiURL, data)
	if err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}
	defer resp.Body.Close()
	log.Println("Report sent successfully!")
}
