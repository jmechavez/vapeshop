package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		log.Fatal("Missing required environment variables.")
	}

	loc, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		log.Fatalf("Failed to load Philippine timezone: %v", err)
	}

	now := time.Now().In(loc)
	eventName := os.Getenv("GITHUB_EVENT_NAME")
	scheduleCron := os.Getenv("SCHEDULE_CRON")

	// The workflow schedules:
	// 0 14 * * * = 10 PM Philippine time
	// 0 23 * * * = 7 AM Philippine time
	//
	// Trust the scheduled event rather than the actual execution hour,
	// because GitHub Actions may start a scheduled job late.
	isMorning := eventName == "schedule" &&
		scheduleCron == "0 23 * * *"

	var reportDate time.Time

	if eventName == "workflow_dispatch" || eventName == "" {
		// Manual runs report today's sales, regardless of the time.
		reportDate = now
	} else if isMorning {
		// Morning report: yesterday's sales.
		reportDate = now.AddDate(0, 0, -1)
	} else {
		// Evening report: today's sales.
		reportDate = now
	}

	log.Printf(
		"Starting report. Philippine time: %s; report date: %s; event: %s",
		now.Format("2006-01-02 15:04:05"),
		reportDate.Format("2006-01-02"),
		eventName,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to create database connection pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	startOfDay := time.Date(
		reportDate.Year(), reportDate.Month(), reportDate.Day(),
		0, 0, 0, 0, loc,
	)
	endOfDay := startOfDay.AddDate(0, 0, 1)

	startOfMonth := time.Date(
		reportDate.Year(), reportDate.Month(), 1,
		0, 0, 0, 0, loc,
	)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// 1. Daily payment statistics
	todayPayments, err := getPaymentStats(ctx, pool, startOfDay, endOfDay)
	if err != nil {
		log.Fatalf("Failed to fetch daily payment statistics: %v", err)
	}

	// 2. Daily sales count
	todaySalesCount, err := getSalesCount(ctx, pool, startOfDay, endOfDay)
	if err != nil {
		log.Fatalf("Failed to fetch daily sales count: %v", err)
	}

	// 3. Monthly payment statistics
	monthPayments, err := getPaymentStats(ctx, pool, startOfMonth, endOfMonth)
	if err != nil {
		log.Fatalf("Failed to fetch monthly payment statistics: %v", err)
	}

	// 4. Monthly sales count
	monthSalesCount, err := getSalesCount(ctx, pool, startOfMonth, endOfMonth)
	if err != nil {
		log.Fatalf("Failed to fetch monthly sales count: %v", err)
	}

	// 5. Partner-stock sales for the report date
	var partnerTotal float64
	var partnerCount int

	err = pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(si.subtotal), 0)::float8,
			COUNT(DISTINCT s.id)
		FROM sale_items si
		JOIN sales s ON s.id = si.sale_id
		JOIN products p ON p.id = si.product_id
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') >= $1
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') < $2
		  AND p.is_partner_stock = TRUE
	`, startOfDay, endOfDay).Scan(&partnerTotal, &partnerCount)

	if err != nil {
		log.Fatalf("Failed to fetch partner-stock sales: %v", err)
	}

	// 6. Build the Telegram message
	var sb strings.Builder

	if isMorning {
		sb.WriteString(fmt.Sprintf(
			"🌅 *Morning Reminder (%s)*\n\n",
			reportDate.Format("Jan 02, 2006"),
		))
	} else {
		sb.WriteString(fmt.Sprintf(
			"🌙 *Evening Report (%s)*\n\n",
			reportDate.Format("Jan 02, 2006"),
		))
	}

	sb.WriteString(fmt.Sprintf(
		"📅 *MGA HALIN %s*\n",
		reportDate.Format("Jan 02, 2006"),
	))
	sb.WriteString(fmt.Sprintf("• Sales Count: %d\n", todaySalesCount))
	sb.WriteString(fmt.Sprintf("• Cash: ₱%.2f\n", todayPayments.Cash))
	sb.WriteString(fmt.Sprintf("• GCash: ₱%.2f\n", todayPayments.GCash))
	sb.WriteString(fmt.Sprintf("• Maribank: ₱%.2f\n", todayPayments.Maribank))
	sb.WriteString(fmt.Sprintf("• Bank Transfer: ₱%.2f\n", todayPayments.BankTransfer))
	sb.WriteString(fmt.Sprintf("*TOTAL:* ₱%.2f\n\n", todayPayments.Total()))

	if partnerTotal > 0 {
		sb.WriteString(fmt.Sprintf(
			"🤝 *MGA HALIN SA STOCK NI JOAN(ATE) %s*\n",
			reportDate.Format("Jan 02, 2006"),
		))
		sb.WriteString(fmt.Sprintf("• Items Sold: %d\n", partnerCount))
		sb.WriteString(fmt.Sprintf("• Total: ₱%.2f\n", partnerTotal))
		sb.WriteString("*This amount belongs to JOAN(ATE).*\n\n")
	}

	sb.WriteString("📊 *MGA HALIN KARONG BULANA*\n")
	sb.WriteString(fmt.Sprintf("• Sales Count: %d\n", monthSalesCount))
	sb.WriteString(fmt.Sprintf("• Cash: ₱%.2f\n", monthPayments.Cash))
	sb.WriteString(fmt.Sprintf("• GCash: ₱%.2f\n", monthPayments.GCash))
	sb.WriteString(fmt.Sprintf("• Maribank: ₱%.2f\n", monthPayments.Maribank))
	sb.WriteString(fmt.Sprintf("• Bank Transfer: ₱%.2f\n", monthPayments.BankTransfer))
	sb.WriteString(fmt.Sprintf("*TOTAL:* ₱%.2f\n", monthPayments.Total()))

	// 7. Send the report and fail the workflow if Telegram rejects it.
	if err := sendTelegramMessage(botToken, chatID, sb.String()); err != nil {
		log.Fatalf("Failed to send Telegram report: %v", err)
	}

	log.Println("Daily Telegram report completed successfully.")
}

func getPaymentStats(
	ctx context.Context,
	pool *pgxpool.Pool,
	start, end time.Time,
) (PaymentStats, error) {
	var p PaymentStats

	rows, err := pool.Query(ctx, `
		SELECT sp.method, COALESCE(SUM(sp.amount), 0)::float8
		FROM sale_payments sp
		JOIN sales s ON s.id = sp.sale_id
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') >= $1
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila') < $2
		GROUP BY sp.method
	`, start, end)
	if err != nil {
		return p, fmt.Errorf("query payment statistics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var method string
		var amount float64

		if err := rows.Scan(&method, &amount); err != nil {
			return p, fmt.Errorf("scan payment statistics: %w", err)
		}

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

	if err := rows.Err(); err != nil {
		return p, fmt.Errorf("iterate payment statistics: %w", err)
	}

	return p, nil
}

func getSalesCount(
	ctx context.Context,
	pool *pgxpool.Pool,
	start, end time.Time,
) (int, error) {
	var count int

	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sales
		WHERE status = 'completed'
		  AND (sale_date AT TIME ZONE 'Asia/Manila') >= $1
		  AND (sale_date AT TIME ZONE 'Asia/Manila') < $2
	`, start, end).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("query sales count: %w", err)
	}

	return count, nil
}

func sendTelegramMessage(token, chatID, message string) error {
	apiURL := fmt.Sprintf(
		"https://api.telegram.org/bot%s/sendMessage",
		token,
	)

	data := url.Values{}
	data.Set("chat_id", chatID)
	data.Set("text", message)
	data.Set("parse_mode", "Markdown")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.PostForm(apiURL, data)
	if err != nil {
		return fmt.Errorf("contact Telegram API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read Telegram API response: %w", err)
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf(
			"invalid Telegram response (HTTP %s): %s",
			resp.Status,
			string(body),
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		return fmt.Errorf(
			"Telegram API rejected the message (HTTP %s): %s",
			resp.Status,
			result.Description,
		)
	}

	log.Println("Telegram report sent successfully!")
	return nil
}
