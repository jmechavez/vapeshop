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
// "Other" is special: the UI shows a free-text box for it.
var VoidReasons = []string{
	"Customer changed mind",
	"Wrong item",
	"Duplicate entry",
	"Payment failed",
	"Other",
}

// Errors returned by the repository so handlers can map them to HTTP codes.
var (
	ErrNotFound          = errors.New("sale not found")
	ErrInvalidStatus     = errors.New("sale status does not allow this action")
	ErrInsufficientStock = errors.New("insufficient stock for one or more items")
	ErrEmptyCart         = errors.New("cannot complete an empty sale")
	ErrVoidReasonMissing = errors.New("void reason is required")
)

// Sale represents one row in the sales table, plus a few joined/derived
// fields used by the UI (CashierName, ItemCount, Items, VoidedByName).
type Sale struct {
	ID            int64
	Reference     string
	UserID        int64
	CashierName   string // joined from users.username
	SaleDate      time.Time
	Subtotal      float64
	Discount      float64
	Total         float64
	PaymentMethod string
	Status        string
	CreatedAt     time.Time

	// Void info (only set when Status == StatusVoid)
	VoidReason   string
	VoidedBy     int64
	VoidedByName string // joined from users.username
	VoidedAt     *time.Time

	// Filled by the detail page and the cart fragment. Empty on list rows.
	Items     []SaleItem
	ItemCount int // used on the list page (COUNT of sale_items)
}

// SaleItem is one line in a sale (product + qty + price at time of sale).
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

// Filter is used by the list page. Dates are "YYYY-MM-DD" strings because
// they come straight from <input type="date">.
type Filter struct {
	Status    string // "" or "all" means no status filter
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

// VoidInput carries the reason chosen by the user. Reason is the dropdown
// value; if Reason == "Other", Note is used as the actual text.
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
