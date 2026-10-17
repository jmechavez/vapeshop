package sales

import (
	"errors"
	"time"
)

// Statuses for a sale.
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusVoid      = "void"
)

// Payment methods (must match the CHECK constraint in the DB).
const (
	PaymentCash         = "Cash"
	PaymentGCash        = "GCash"
	PaymentMaribank     = "Maribank"
	PaymentBankTransfer = "Bank Transfer"
)

// PaymentMethods is the ordered list used to render the <select> in the UI.
var PaymentMethods = []string{
	PaymentCash,
	PaymentGCash,
	PaymentMaribank,
	PaymentBankTransfer,
}

// Statuses is the ordered list used to render the status filter.
var Statuses = []string{
	StatusPending,
	StatusCompleted,
	StatusVoid,
}

// VoidReasons is the preset list shown when voiding a sale.
var VoidReasons = []string{
	"Customer changed mind",
	"Wrong item",
	"Duplicate entry",
	"Payment failed",
	"Other",
}

// Errors returned by the repository.
var (
	ErrNotFound          = errors.New("sale not found")
	ErrInvalidStatus     = errors.New("sale status does not allow this action")
	ErrInsufficientStock = errors.New("insufficient stock for one or more items")
	ErrEmptyCart         = errors.New("cannot complete an empty sale")
	ErrVoidReasonMissing = errors.New("void reason is required")
)

// Sale represents one row in the sales table.
type Sale struct {
	ID            int64
	Reference     string
	UserID        int64
	CashierName   string
	SaleDate      time.Time
	Subtotal      float64
	Discount      float64
	Total         float64
	PaymentMethod string
	Status        string
	CreatedAt     time.Time

	VoidReason   string
	VoidedBy     int64
	VoidedByName string
	VoidedAt     *time.Time

	Items     []SaleItem
	ItemCount int
}

// SaleItem is one line in a sale.
type SaleItem struct {
	ID          int64
	SaleID      int64
	ProductID   int64
	SKU         string
	ProductName string
	Quantity    int
	UnitPrice   float64
	Subtotal    float64
}

// Filter is used by the list page.
type Filter struct {
	Status    string
	DateFrom  string
	DateTo    string
	Reference string
}

// ProductSuggestion is a lightweight row returned by the POS search box.
type ProductSuggestion struct {
	ID           int64
	SKU          string
	Name         string
	SellingPrice float64
	CurrentStock int
}

// AddItemInput is used when adding a product to a pending sale's cart.
type AddItemInput struct {
	ProductID int64
	Quantity  int
}

// VoidInput carries the reason chosen by the user.
type VoidInput struct {
	Reason string
	Note   string
}

// FinalReason returns the string to store in sales.void_reason.
func (v VoidInput) FinalReason() string {
	if v.Reason == "Other" {
		if v.Note == "" {
			return "Other"
		}
		return "Other: " + v.Note
	}
	return v.Reason
}

// Page carries pagination metadata for the sales list.
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
