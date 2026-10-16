package reports

import "time"

// Range is the date range used by the report queries.
// Dates are "YYYY-MM-DD" strings because they come from <input type="date">.
type Range struct {
	From string
	To   string
}

// Summary holds the top-line numbers for the selected range.
type Summary struct {
	SalesCount int
	ItemsSold  int
}

// PaymentTotals is the ₱ amount collected per payment method.
type PaymentTotals struct {
	Cash         float64
	GCash        float64
	Maribank     float64
	BankTransfer float64
}

// Total returns the sum of all payment methods.
func (p PaymentTotals) Total() float64 {
	return p.Cash + p.GCash + p.Maribank + p.BankTransfer
}

// DailyTotal is one bar in the daily revenue chart.
type DailyTotal struct {
	Day     time.Time
	Revenue float64
	Count   int
}

// TopProduct is one row in the "top products" table.
type TopProduct struct {
	ProductID int64
	SKU       string
	Name      string
	QtySold   int
	Revenue   float64
}

// LowStock is one row in the "low stock" table.
type LowStock struct {
	ProductID    int64
	SKU          string
	Name         string
	CurrentStock int
	MinimumStock int
}

// RecentSale is one row in the "recent sales" table.
type RecentSale struct {
	ID            int64
	Reference     string
	SaleDate      time.Time
	CashierName   string
	ItemCount     int
	Total         float64
	PaymentMethod string
}

// ChartBar is one rectangle in the SVG bar chart.
type ChartBar struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
	Label  string
}

// Chart holds everything the SVG bar chart needs to render.
type Chart struct {
	Width  float64
	Height float64
	ChartH float64
	Bars   []ChartBar
	Days   []DailyTotal
}

// Report is everything the reports page needs in one struct.
type Report struct {
	Range       Range
	Summary     Summary
	Payments    PaymentTotals
	DailyTotals []DailyTotal
	TopProducts []TopProduct
	LowStock    []LowStock
	RecentSales []RecentSale
	Chart       Chart
}

// Dashboard is the smaller set of numbers for the dashboard page.
type Dashboard struct {
	TodaySalesCount int
	TodayPayments   PaymentTotals
	LowStockCount   int
	RecentSales     []RecentSale
}
