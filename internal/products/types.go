package products

import "time"

type Product struct {
	ID             int64
	SKU            string
	Barcode        string
	Name           string
	Brand          string
	Version        string
	Flavor         string
	Category       string
	CostPrice      float64
	SellingPrice   float64
	MinimumStock   int
	IsPartnerStock bool // NEW
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ProductWithStock struct {
	Product
	CurrentStock int
}

type Page struct {
	Page    int
	PerPage int
	Total   int
}

func (p Page) HasPrev() bool { return p.Page > 1 }
func (p Page) HasNext() bool { return p.Page*p.PerPage < p.Total }
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
func (p Page) From() int {
	if p.Total == 0 {
		return 0
	}
	return (p.Page-1)*p.PerPage + 1
}
func (p Page) To() int {
	end := p.Page * p.PerPage
	if end > p.Total {
		return p.Total
	}
	return end
}
