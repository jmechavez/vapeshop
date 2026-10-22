package reports

import (
	"context"
	"strconv"
	"time"

	"github.com/jmechavez/vapeshop/internal/database"
)

func normalizeRange(r Range) Range {
	loc, _ := time.LoadLocation("Asia/Manila")
	today := time.Now().In(loc)

	if r.From == "" {
		r.From = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc).
			Format("2006-01-02")
	}
	if r.To == "" {
		r.To = today.Format("2006-01-02")
	}
	if r.From > r.To {
		r.From, r.To = r.To, r.From
	}
	return r
}

func GetSummary(ctx context.Context, r Range) (Summary, error) {
	r = normalizeRange(r)
	var s Summary

	err := database.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COALESCE(SUM((SELECT COALESCE(SUM(quantity), 0) FROM sale_items WHERE sale_id = s.id)), 0)::int
		FROM sales s
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date >= $1::date
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date <= $2::date
	`, r.From, r.To).Scan(&s.SalesCount, &s.ItemsSold)
	if err != nil {
		return Summary{}, err
	}
	return s, nil
}

func GetPaymentTotals(ctx context.Context, r Range) (PaymentTotals, error) {
	r = normalizeRange(r)
	var p PaymentTotals

	rows, err := database.Pool.Query(ctx, `
		SELECT sp.method, COALESCE(SUM(sp.amount), 0)::float8
		FROM sale_payments sp
		JOIN sales s ON s.id = sp.sale_id
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date >= $1::date
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date <= $2::date
		GROUP BY sp.method
	`, r.From, r.To)
	if err != nil {
		return PaymentTotals{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var method string
		var amount float64
		if err := rows.Scan(&method, &amount); err != nil {
			return PaymentTotals{}, err
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
	return p, rows.Err()
}

func GetDailyTotals(ctx context.Context, r Range) ([]DailyTotal, error) {
	r = normalizeRange(r)

	rows, err := database.Pool.Query(ctx, `
		SELECT
			date_trunc('day', s.sale_date AT TIME ZONE 'Asia/Manila')::date AS day,
			COALESCE(SUM(s.total), 0)::float8     AS revenue,
			COUNT(*)::int                         AS count
		FROM sales s
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date >= $1::date
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date <= $2::date
		GROUP BY day
		ORDER BY day ASC
	`, r.From, r.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyTotal
	for rows.Next() {
		var d DailyTotal
		if err := rows.Scan(&d.Day, &d.Revenue, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func GetTopProducts(ctx context.Context, r Range, limit int) ([]TopProduct, error) {
	r = normalizeRange(r)
	if limit <= 0 {
		limit = 10
	}

	rows, err := database.Pool.Query(ctx, `
		SELECT
			si.product_id,
			si.sku,
			si.product_name,
			SUM(si.quantity)::int,
			SUM(si.subtotal)::float8
		FROM sale_items si
		JOIN sales s ON s.id = si.sale_id
		WHERE s.status = 'completed'
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date >= $1::date
		  AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date <= $2::date
		GROUP BY si.product_id, si.sku, si.product_name
		ORDER BY SUM(si.quantity) DESC, SUM(si.subtotal) DESC
		LIMIT $3
	`, r.From, r.To, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopProduct
	for rows.Next() {
		var p TopProduct
		if err := rows.Scan(&p.ProductID, &p.SKU, &p.Name, &p.QtySold, &p.Revenue); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func GetLowStock(ctx context.Context, limit int) ([]LowStock, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := database.Pool.Query(ctx, `
		SELECT
			p.id,
			p.sku,
			p.name,
			COALESCE(i.current_stock, 0)::int,
			p.minimum_stock::int
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = true
		  AND COALESCE(i.current_stock, 0) <= p.minimum_stock
		ORDER BY (COALESCE(i.current_stock, 0) - p.minimum_stock) ASC, p.name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LowStock
	for rows.Next() {
		var l LowStock
		if err := rows.Scan(&l.ProductID, &l.SKU, &l.Name, &l.CurrentStock, &l.MinimumStock); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func GetRecentSales(ctx context.Context, limit int) ([]RecentSale, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := database.Pool.Query(ctx, `
		SELECT
			s.id,
			s.reference,
			s.sale_date,
			COALESCE(u.username, ''),
			COALESCE((SELECT COUNT(*)::int FROM sale_items WHERE sale_id = s.id), 0),
			s.total::float8,
			s.payment_method
		FROM sales s
		LEFT JOIN users u ON u.id = s.user_id
		ORDER BY s.sale_date DESC, s.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecentSale
	for rows.Next() {
		var s RecentSale
		if err := rows.Scan(
			&s.ID, &s.Reference, &s.SaleDate, &s.CashierName,
			&s.ItemCount, &s.Total, &s.PaymentMethod,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func BuildChart(days []DailyTotal) Chart {
	const width = 800.0
	const height = 220.0
	const padBottom = 30.0
	chartH := height - padBottom

	maxRev := 0.0
	for _, d := range days {
		if d.Revenue > maxRev {
			maxRev = d.Revenue
		}
	}
	if maxRev == 0 {
		maxRev = 1
	}

	c := Chart{
		Width:  width,
		Height: height,
		ChartH: chartH,
		Days:   days,
	}

	n := len(days)
	if n == 0 {
		return c
	}

	slot := width / float64(n)
	barW := slot - 4
	if barW < 2 {
		barW = 2
	}

	bars := make([]ChartBar, 0, n)
	for i, d := range days {
		h := (d.Revenue / maxRev) * (chartH - 10)
		if h < 1 {
			h = 1
		}
		bars = append(bars, ChartBar{
			X:      float64(i)*slot + 2,
			Y:      chartH - h,
			Width:  barW,
			Height: h,
			Label: d.Day.Format("2006-01-02") +
				" — ₱" + strconv.FormatFloat(d.Revenue, 'f', 2, 64) +
				" (" + strconv.Itoa(d.Count) + " sales)",
		})
	}
	c.Bars = bars
	return c
}

func LoadReport(ctx context.Context, r Range) (Report, error) {
	r = normalizeRange(r)

	summary, err := GetSummary(ctx, r)
	if err != nil {
		return Report{}, err
	}
	payments, err := GetPaymentTotals(ctx, r)
	if err != nil {
		return Report{}, err
	}
	daily, err := GetDailyTotals(ctx, r)
	if err != nil {
		return Report{}, err
	}
	top, err := GetTopProducts(ctx, r, 10)
	if err != nil {
		return Report{}, err
	}
	low, err := GetLowStock(ctx, 20)
	if err != nil {
		return Report{}, err
	}
	recent, err := GetRecentSales(ctx, 10)
	if err != nil {
		return Report{}, err
	}

	return Report{
		Range:       r,
		Summary:     summary,
		Payments:    payments,
		DailyTotals: daily,
		TopProducts: top,
		LowStock:    low,
		RecentSales: recent,
		Chart:       BuildChart(daily),
	}, nil
}

func LoadDashboard(ctx context.Context) (Dashboard, error) {
	loc, _ := time.LoadLocation("Asia/Manila")
	today := time.Now().In(loc).Format("2006-01-02")
	var d Dashboard

	err := database.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM sales
		WHERE status = 'completed'
		  AND (sale_date AT TIME ZONE 'Asia/Manila')::date = $1::date
	`, today).Scan(&d.TodaySalesCount)
	if err != nil {
		return Dashboard{}, err
	}

	payments, err := GetPaymentTotals(ctx, Range{From: today, To: today})
	if err != nil {
		return Dashboard{}, err
	}
	d.TodayPayments = payments

	err = database.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = true
		  AND COALESCE(i.current_stock, 0) <= p.minimum_stock
	`).Scan(&d.LowStockCount)
	if err != nil {
		return Dashboard{}, err
	}

	err = database.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(si.subtotal), 0), COUNT(DISTINCT s.id)
		FROM sale_items si
		JOIN sales s ON s.id = si.sale_id
		JOIN products p ON p.id = si.product_id
		WHERE s.status = 'completed'
		AND (s.sale_date AT TIME ZONE 'Asia/Manila')::date = $1::date
		AND p.is_partner_stock = TRUE
	`, today).Scan(&d.TodayPartnerStockTotal, &d.TodayPartnerStockCount)
	if err != nil {
		return Dashboard{}, err
	}

	err = database.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM sales
		WHERE status = 'void'
		AND (sale_date AT TIME ZONE 'Asia/Manila')::date = $1::date
	`, today).Scan(&d.TodayVoidsCount)
	if err != nil {
		return Dashboard{}, err
	}

	recent, err := GetRecentSales(ctx, 5)
	if err != nil {
		return Dashboard{}, err
	}
	d.RecentSales = recent
	return d, nil
}
