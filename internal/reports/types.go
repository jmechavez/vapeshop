package reports

import "time"

type Range struct {
	From string
	To   string
}

type Summary struct {
	SalesCount int
	ItemsSold  int
}

type PaymentTotals struct {
	Cash         float64
	GCash        float64
	Maribank     float64
	BankTransfer float64
}

func (p PaymentTotals) Total() float64 {
	return p.Cash + p.GCash + p.Maribank + p.BankTransfer
}

type DailyTotal struct {
	Day     time.Time
	Revenue float64
	Count   int
}

type TopProduct struct {
	ProductID int64
	SKU       string
	Name      string
	QtySold   int
	Revenue   float64
}

type LowStock struct {
	ProductID    int64
	SKU          string
	Name         string
	CurrentStock int
	MinimumStock int
}

// RecentSale now holds the split payments
type RecentSale struct {
	ID            int64
	Reference     string
	SaleDate      time.Time
	CashierName   string
	ItemCount     int
	Total         float64
	PaymentMethod string
	Payments      []SplitPayment // NEW
}

// SplitPayment represents one row in the sale_payments table
type SplitPayment struct {
	Method string
	Amount float64
}

type ChartBar struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
	Label  string
}

type Chart struct {
	Width  float64
	Height float64
	ChartH float64
	Bars   []ChartBar
	Days   []DailyTotal
}

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

type Dashboard struct {
	TodaySalesCount int
	TodayPayments   PaymentTotals
	LowStockCount   int
	RecentSales     []RecentSale
}
