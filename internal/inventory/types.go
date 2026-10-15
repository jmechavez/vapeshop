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
// Query matches product name OR SKU (case-insensitive).
type MovementFilter struct {
	Query    string // NEW
	Type     string
	DateFrom string // YYYY-MM-DD
	DateTo   string // YYYY-MM-DD
}
