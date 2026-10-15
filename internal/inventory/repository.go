// internal/inventory/repository.go
package inventory

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

// List returns every active product with its current stock.
func (r *Repository) List(ctx context.Context) ([]InventoryRow, error) {
	const q = `
		SELECT
			p.id,
			p.sku,
			p.name,
			COALESCE(p.brand, '')    AS brand,
			COALESCE(p.category, '') AS category,
			COALESCE(i.current_stock, 0) AS current_stock,
			p.minimum_stock,
			COALESCE(i.updated_at, p.updated_at) AS updated_at
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = TRUE
		ORDER BY p.name ASC
	`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()

	var out []InventoryRow
	for rows.Next() {
		var it InventoryRow
		if err := rows.Scan(
			&it.ProductID, &it.SKU, &it.Name, &it.Brand, &it.Category,
			&it.CurrentStock, &it.MinimumStock, &it.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan inventory: %w", err)
		}
		it.IsLowStock = it.CurrentStock <= it.MinimumStock
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetRow returns one inventory row by product ID.
func (r *Repository) GetRow(ctx context.Context, productID int64) (InventoryRow, error) {
	const q = `
		SELECT
			p.id, p.sku, p.name,
			COALESCE(p.brand, '')    AS brand,
			COALESCE(p.category, '') AS category,
			COALESCE(i.current_stock, 0) AS current_stock,
			p.minimum_stock,
			COALESCE(i.updated_at, p.updated_at) AS updated_at
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.id = $1 AND p.active = TRUE
	`
	var it InventoryRow
	err := r.db.QueryRow(ctx, q, productID).Scan(
		&it.ProductID, &it.SKU, &it.Name, &it.Brand, &it.Category,
		&it.CurrentStock, &it.MinimumStock, &it.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InventoryRow{}, ErrNotFound
		}
		return InventoryRow{}, fmt.Errorf("get inventory row: %w", err)
	}
	it.IsLowStock = it.CurrentStock <= it.MinimumStock
	return it, nil
}

// ListSuppliers returns suppliers for the receive-stock dropdown.
func (r *Repository) ListSuppliers(ctx context.Context) ([]Supplier, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name FROM suppliers ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	defer rows.Close()

	var out []Supplier
	for rows.Next() {
		var s Supplier
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, fmt.Errorf("scan supplier: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Receive adds stock: insert movement of type 'in', increment inventory.
func (r *Repository) Receive(ctx context.Context, productID, userID int64, in ReceiveInput) error {
	if in.Quantity <= 0 {
		return ErrInvalidQuantity
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin receive: %w", err)
	}
	defer tx.Rollback(ctx)

	var uid *int64
	if userID > 0 {
		uid = &userID
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_movements
			(product_id, user_id, supplier_id, type, quantity, unit_cost, reference, notes)
		VALUES ($1, $2, $3, 'in', $4, $5, $6, $7)
	`, productID, uid, in.SupplierID, in.Quantity, in.UnitCost,
		nullIfEmpty(in.Reference), nullIfEmpty(in.Notes))
	if err != nil {
		return fmt.Errorf("insert movement: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory (product_id, current_stock, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (product_id) DO UPDATE
		SET current_stock = inventory.current_stock + EXCLUDED.current_stock,
		    updated_at    = NOW()
	`, productID, in.Quantity)
	if err != nil {
		return fmt.Errorf("upsert inventory: %w", err)
	}

	return tx.Commit(ctx)
}

// Adjust sets stock to an exact new quantity. Movement quantity is the delta.
func (r *Repository) Adjust(ctx context.Context, productID, userID int64, in AdjustInput) error {
	if in.NewQuantity < 0 {
		return ErrInvalidQuantity
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin adjust: %w", err)
	}
	defer tx.Rollback(ctx)

	var current int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(current_stock, 0) FROM inventory WHERE product_id = $1`,
		productID,
	).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read current stock: %w", err)
	}

	delta := in.NewQuantity - current

	var uid *int64
	if userID > 0 {
		uid = &userID
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_movements
			(product_id, user_id, type, quantity, notes)
		VALUES ($1, $2, 'adjust', $3, $4)
	`, productID, uid, delta, nullIfEmpty(in.Notes))
	if err != nil {
		return fmt.Errorf("insert adjust movement: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory (product_id, current_stock, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (product_id) DO UPDATE
		SET current_stock = EXCLUDED.current_stock,
		    updated_at    = NOW()
	`, productID, in.NewQuantity)
	if err != nil {
		return fmt.Errorf("set inventory: %w", err)
	}

	return tx.Commit(ctx)
}

// Movements returns history rows with optional filters.
//
// NOTE: query filters by product name OR sku (ILIKE), which needs the same
// value bound to two placeholders. We handle that as a special case below.
func (r *Repository) Movements(ctx context.Context, f MovementFilter) ([]Movement, error) {
	var (
		conds []string
		args  []any
		n     int
	)
	add := func(cond string, val any) {
		n++
		conds = append(conds, fmt.Sprintf(cond, n))
		args = append(args, val)
	}

	if f.Query != "" {
		// Same pattern goes into TWO placeholders, so we bind the value twice.
		n++
		like := "%" + f.Query + "%"
		conds = append(conds,
			fmt.Sprintf("(p.name ILIKE $%d OR p.sku ILIKE $%d)", n, n),
		)
		args = append(args, like)
	}
	if f.Type != "" {
		add("sm.type = $%d", f.Type)
	}
	if f.DateFrom != "" {
		add("sm.movement_date >= $%d", f.DateFrom)
	}
	if f.DateTo != "" {
		add("sm.movement_date <= $%d", f.DateTo)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	q := fmt.Sprintf(`
		SELECT
			sm.id,
			sm.product_id,
			p.sku,
			p.name AS product_name,
			sm.user_id,
			COALESCE(u.username, '') AS username,
			sm.supplier_id,
			COALESCE(s.name, '')     AS supplier_name,
			sm.type,
			sm.quantity,
			sm.unit_cost,
			COALESCE(sm.reference, '') AS reference,
			COALESCE(sm.notes, '')     AS notes,
			sm.movement_date,
			sm.created_at
		FROM stock_movements sm
		JOIN products p  ON p.id = sm.product_id
		LEFT JOIN users u     ON u.id = sm.user_id
		LEFT JOIN suppliers s ON s.id = sm.supplier_id
		%s
		ORDER BY sm.movement_date DESC, sm.id DESC
		LIMIT 500
	`, where)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	defer rows.Close()

	var out []Movement
	for rows.Next() {
		var m Movement
		if err := rows.Scan(
			&m.ID, &m.ProductID, &m.SKU, &m.ProductName,
			&m.UserID, &m.Username, &m.SupplierID, &m.SupplierName,
			&m.Type, &m.Quantity, &m.UnitCost,
			&m.Reference, &m.Notes, &m.MovementDate, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan movement: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- helpers & sentinel errors ---

var (
	ErrNotFound        = errors.New("inventory: not found")
	ErrInvalidQuantity = errors.New("inventory: invalid quantity")
)

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
