package products

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jmechavez/vapeshop/internal/database"
)

var ErrNotFound = errors.New("product not found")

// List returns all products with their current stock, alphabetically.
func List(ctx context.Context, search string) ([]ProductWithStock, error) {
	query := `
		SELECT p.id, p.sku, COALESCE(p.barcode, ''), p.name,
		       COALESCE(p.brand, ''), COALESCE(p.version, ''), COALESCE(p.flavor, ''),
		       COALESCE(p.category, ''),
		       p.cost_price, p.selling_price, p.minimum_stock, p.active,
		       p.created_at, p.updated_at,
		       COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = TRUE
	`
	args := []any{}
	if search != "" {
		query += ` AND (p.name ILIKE $1 OR p.sku ILIKE $1 OR COALESCE(p.brand,'') ILIKE $1)`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY p.name ASC`

	rows, err := database.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProductWithStock
	for rows.Next() {
		var p ProductWithStock
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Barcode, &p.Name,
			&p.Brand, &p.Version, &p.Flavor, &p.Category,
			&p.CostPrice, &p.SellingPrice, &p.MinimumStock, &p.Active,
			&p.CreatedAt, &p.UpdatedAt,
			&p.CurrentStock,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one product with stock.
func Get(ctx context.Context, id int64) (*ProductWithStock, error) {
	row := database.Pool.QueryRow(ctx, `
		SELECT p.id, p.sku, COALESCE(p.barcode, ''), p.name,
		       COALESCE(p.brand, ''), COALESCE(p.version, ''), COALESCE(p.flavor, ''),
		       COALESCE(p.category, ''),
		       p.cost_price, p.selling_price, p.minimum_stock, p.active,
		       p.created_at, p.updated_at,
		       COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.id = $1`, id)

	var p ProductWithStock
	err := row.Scan(
		&p.ID, &p.SKU, &p.Barcode, &p.Name,
		&p.Brand, &p.Version, &p.Flavor, &p.Category,
		&p.CostPrice, &p.SellingPrice, &p.MinimumStock, &p.Active,
		&p.CreatedAt, &p.UpdatedAt,
		&p.CurrentStock,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Create inserts a new product and creates its inventory row with 0 stock.
func Create(ctx context.Context, p *Product) (int64, error) {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO products
			(sku, barcode, name, brand, version, flavor, category,
			 cost_price, selling_price, minimum_stock, active)
		VALUES ($1, NULLIF($2,''), $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''),
		        NULLIF($7,''), $8, $9, $10, TRUE)
		RETURNING id`,
		p.SKU, p.Barcode, p.Name, p.Brand, p.Version, p.Flavor, p.Category,
		p.CostPrice, p.SellingPrice, p.MinimumStock,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO inventory (product_id, current_stock) VALUES ($1, 0)`, id)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

// Update writes changed fields.
func Update(ctx context.Context, p *Product) error {
	_, err := database.Pool.Exec(ctx, `
		UPDATE products SET
			sku = $1, barcode = NULLIF($2,''), name = $3,
			brand = NULLIF($4,''), version = NULLIF($5,''), flavor = NULLIF($6,''),
			category = NULLIF($7,''),
			cost_price = $8, selling_price = $9, minimum_stock = $10,
			updated_at = NOW()
		WHERE id = $11`,
		p.SKU, p.Barcode, p.Name, p.Brand, p.Version, p.Flavor, p.Category,
		p.CostPrice, p.SellingPrice, p.MinimumStock, p.ID,
	)
	return err
}

// SoftDelete marks the product inactive. Keeps history intact.
func SoftDelete(ctx context.Context, id int64) error {
	_, err := database.Pool.Exec(ctx,
		`UPDATE products SET active = FALSE, updated_at = NOW() WHERE id = $1`, id)
	return err
}
