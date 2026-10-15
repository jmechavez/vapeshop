// internal/suppliers/types.go
package suppliers

import "time"

// Supplier is one row on the /suppliers list and detail page.
type Supplier struct {
	ID            int64
	Name          string
	ContactPerson string
	Phone         string
	Email         string
	Address       string
	CreatedAt     time.Time
}

// SupplierWithStats adds per-supplier counts for the list page.
// MovementCount = number of stock_movements rows referencing this supplier.
// LastMovement  = nil if the supplier has no movements yet.
type SupplierWithStats struct {
	Supplier
	MovementCount int
	LastMovement  *time.Time
}

// Input is the form data for create/update.
type Input struct {
	Name          string
	ContactPerson string
	Phone         string
	Email         string
	Address       string
}

// PurchaseRow is one row on the supplier detail page.
// It shows what we bought from this supplier.
type PurchaseRow struct {
	MovementID   int64
	MovementDate time.Time
	Type         string // usually 'in'
	SKU          string
	ProductName  string
	Quantity     int
	UnitCost     *float64
	Reference    string
	Notes        string
	Username     string
}
