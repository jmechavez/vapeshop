package products

import "time"

type Product struct {
	ID           int64
	SKU          string
	Barcode      string
	Name         string
	Brand        string
	Version      string
	Flavor       string
	Category     string
	CostPrice    float64
	SellingPrice float64
	MinimumStock int
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// StockForDisplay is used when we want to show current stock in the list.
type ProductWithStock struct {
	Product
	CurrentStock int
}
