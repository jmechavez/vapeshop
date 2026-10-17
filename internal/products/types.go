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

// ProductWithStock is used when we want to show current stock in the list.
type ProductWithStock struct {
	Product
	CurrentStock int
}

// Page carries pagination metadata for the products list.
type Page struct {
	Page    int
	PerPage int
	Total   int
}

// HasPrev reports whether a previous page exists.
func (p Page) HasPrev() bool { return p.Page > 1 }

// HasNext reports whether a next page exists.
func (p Page) HasNext() bool { return p.Page*p.PerPage < p.Total }

// TotalPages is the ceiling of Total/PerPage (minimum 1).
func (p Page) TotalPages() int {
	if p.PerPage <= 0 {
		return 1
	}
	t := (p.Total + p.PerPage - 1) / p.PerPage
	if t < 1 {
		return 1
	}
	return t
}

// From is the 1-based index of the first row on this page.
func (p Page) From() int {
	if p.Total == 0 {
		return 0
	}
	return (p.Page-1)*p.PerPage + 1
}

// To is the 1-based index of the last row on this page.
func (p Page) To() int {
	end := p.Page * p.PerPage
	if end > p.Total {
		return p.Total
	}
	return end
}
