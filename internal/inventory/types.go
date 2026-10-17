// internal/inventory/types.go
package inventory

import "time"

// InventoryRow is one row on the /inventory list page.
type InventoryRow struct {
	ProductID    int64
	SKU          string
	Name         string
	Brand        string
	Category     string
	CurrentStock int
	MinimumStock int
	IsLowStock   bool
	UpdatedAt    time.Time
}

// Supplier is a minimal shape for the receive-stock dropdown.
type Supplier struct {
	ID   int64
	Name string
}

// ReceiveInput is the form data for "Receive stock".
type ReceiveInput struct {
	Quantity   int
	UnitCost   float64
	SupplierID *int64
	Reference  string
	Notes      string
}

// AdjustInput is the form data for "Adjust stock".
type AdjustInput struct {
	NewQuantity int
	Notes       string
}

// Movement is one row on the /inventory/movements page.
type Movement struct {
	ID           int64
	ProductID    int64
	SKU          string
	ProductName  string
	UserID       *int64
	Username     string
	SupplierID   *int64
	SupplierName string
	Type         string
	Quantity     int
	UnitCost     *float64
	Reference    string
	Notes        string
	MovementDate time.Time
	CreatedAt    time.Time
}

// MovementFilter holds the filter form values for /inventory/movements.
type MovementFilter struct {
	Query    string
	Type     string
	DateFrom string // YYYY-MM-DD
	DateTo   string // YYYY-MM-DD
}

// ListFilter holds the filter + paging state for /inventory.
type ListFilter struct {
	Query   string // matches name, SKU, or brand (ILIKE)
	LowOnly bool   // only rows where current_stock <= minimum_stock
	Page    int    // 1-based
	PerPage int    // default 25
}

// Page carries pagination metadata from repo → handler → template.
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
