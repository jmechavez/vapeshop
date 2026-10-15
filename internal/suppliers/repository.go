// internal/suppliers/repository.go
package suppliers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// List returns all suppliers with movement counts and last movement date.
// Ordered by name.
func (r *Repository) List(ctx context.Context) ([]SupplierWithStats, error) {
	const q = `
		SELECT
			s.id,
			s.name,
			COALESCE(s.contact_person, '') AS contact_person,
			COALESCE(s.phone, '')          AS phone,
			COALESCE(s.email, '')          AS email,
			COALESCE(s.address, '')        AS address,
			s.created_at,
			COUNT(sm.id)                   AS movement_count,
			MAX(sm.movement_date)          AS last_movement
		FROM suppliers s
		LEFT JOIN stock_movements sm ON sm.supplier_id = s.id
		GROUP BY s.id
		ORDER BY s.name ASC
	`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	defer rows.Close()

	var out []SupplierWithStats
	for rows.Next() {
		var s SupplierWithStats
		if err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.ContactPerson,
			&s.Phone,
			&s.Email,
			&s.Address,
			&s.CreatedAt,
			&s.MovementCount,
			&s.LastMovement,
		); err != nil {
			return nil, fmt.Errorf("scan supplier: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Get returns a single supplier by ID.
func (r *Repository) Get(ctx context.Context, id int64) (Supplier, error) {
	const q = `
		SELECT
			id,
			name,
			COALESCE(contact_person, '') AS contact_person,
			COALESCE(phone, '')          AS phone,
			COALESCE(email, '')          AS email,
			COALESCE(address, '')        AS address,
			created_at
		FROM suppliers
		WHERE id = $1
	`

	var s Supplier
	err := r.db.QueryRow(ctx, q, id).Scan(
		&s.ID,
		&s.Name,
		&s.ContactPerson,
		&s.Phone,
		&s.Email,
		&s.Address,
		&s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Supplier{}, ErrNotFound
		}
		return Supplier{}, fmt.Errorf("get supplier: %w", err)
	}
	return s, nil
}

// Create inserts a supplier and returns the new row.
func (r *Repository) Create(ctx context.Context, in Input) (Supplier, error) {
	if err := validate(in); err != nil {
		return Supplier{}, err
	}

	const q = `
		INSERT INTO suppliers (name, contact_person, phone, email, address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name,
		          COALESCE(contact_person, '') AS contact_person,
		          COALESCE(phone, '')          AS phone,
		          COALESCE(email, '')          AS email,
		          COALESCE(address, '')        AS address,
		          created_at
	`
	var s Supplier
	err := r.db.QueryRow(ctx, q,
		in.Name,
		nullIfEmpty(in.ContactPerson),
		nullIfEmpty(in.Phone),
		nullIfEmpty(in.Email),
		nullIfEmpty(in.Address),
	).Scan(
		&s.ID,
		&s.Name,
		&s.ContactPerson,
		&s.Phone,
		&s.Email,
		&s.Address,
		&s.CreatedAt,
	)
	if err != nil {
		return Supplier{}, fmt.Errorf("create supplier: %w", err)
	}
	return s, nil
}

// Update modifies a supplier by ID and returns the updated row.
func (r *Repository) Update(ctx context.Context, id int64, in Input) (Supplier, error) {
	if err := validate(in); err != nil {
		return Supplier{}, err
	}

	const q = `
		UPDATE suppliers
		SET name = $2,
		    contact_person = $3,
		    phone = $4,
		    email = $5,
		    address = $6
		WHERE id = $1
		RETURNING id, name,
		          COALESCE(contact_person, '') AS contact_person,
		          COALESCE(phone, '')          AS phone,
		          COALESCE(email, '')          AS email,
		          COALESCE(address, '')        AS address,
		          created_at
	`
	var s Supplier
	err := r.db.QueryRow(ctx, q,
		id,
		in.Name,
		nullIfEmpty(in.ContactPerson),
		nullIfEmpty(in.Phone),
		nullIfEmpty(in.Email),
		nullIfEmpty(in.Address),
	).Scan(
		&s.ID,
		&s.Name,
		&s.ContactPerson,
		&s.Phone,
		&s.Email,
		&s.Address,
		&s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Supplier{}, ErrNotFound
		}
		return Supplier{}, fmt.Errorf("update supplier: %w", err)
	}
	return s, nil
}

// Delete removes a supplier, but refuses if any stock_movements reference it.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	// Check for references first, so we can return a friendly error.
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM stock_movements WHERE supplier_id = $1`,
		id,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check supplier references: %w", err)
	}
	if count > 0 {
		return ErrInUse
	}

	tag, err := r.db.Exec(ctx, `DELETE FROM suppliers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete supplier: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Purchases returns all stock movements (typically 'in') for a supplier,
// with product name and user name joined in. Newest first.
func (r *Repository) Purchases(ctx context.Context, supplierID int64) ([]PurchaseRow, error) {
	const q = `
		SELECT
			sm.id,
			sm.movement_date,
			sm.type,
			p.sku,
			p.name AS product_name,
			sm.quantity,
			sm.unit_cost,
			COALESCE(sm.reference, '') AS reference,
			COALESCE(sm.notes, '')     AS notes,
			COALESCE(u.username, '')   AS username
		FROM stock_movements sm
		JOIN products p       ON p.id = sm.product_id
		LEFT JOIN users u     ON u.id = sm.user_id
		WHERE sm.supplier_id = $1
		ORDER BY sm.movement_date DESC, sm.id DESC
	`
	rows, err := r.db.Query(ctx, q, supplierID)
	if err != nil {
		return nil, fmt.Errorf("list purchases: %w", err)
	}
	defer rows.Close()

	var out []PurchaseRow
	for rows.Next() {
		var p PurchaseRow
		if err := rows.Scan(
			&p.MovementID,
			&p.MovementDate,
			&p.Type,
			&p.SKU,
			&p.ProductName,
			&p.Quantity,
			&p.UnitCost,
			&p.Reference,
			&p.Notes,
			&p.Username,
		); err != nil {
			return nil, fmt.Errorf("scan purchase: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- helpers & errors ---

var (
	ErrNotFound = errors.New("suppliers: not found")
	ErrInUse    = errors.New("suppliers: in use by stock movements")
	ErrInvalid  = errors.New("suppliers: invalid input")
)

func validate(in Input) error {
	if strings.TrimSpace(in.Name) == "" {
		return ErrInvalid
	}
	if len(in.Name) > 200 {
		return ErrInvalid
	}
	return nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
