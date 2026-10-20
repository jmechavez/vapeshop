package products

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jmechavez/vapeshop/internal/database"
)

var (
	ErrNotFound = errors.New("product: not found")
)

func List(ctx context.Context, q string, page, perPage int) ([]ProductWithStock, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	offset := (page - 1) * perPage

	var (
		conds []string
		args  []any
		n     int
	)
	baseWhere := "WHERE p.active = TRUE"

	if strings.TrimSpace(q) != "" {
		n++
		like := "%" + strings.TrimSpace(q) + "%"
		conds = append(conds,
			fmt.Sprintf("(p.name ILIKE $%d OR p.sku ILIKE $%d OR COALESCE(p.brand,'') ILIKE $%d)", n, n, n),
		)
		args = append(args, like)
	}

	where := baseWhere
	if len(conds) > 0 {
		where += " AND " + strings.Join(conds, " AND ")
	}

	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM products p %s`, where)
	var total int
	if err := database.Pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	rowsSQL := fmt.Sprintf(`
		SELECT
			p.id, p.sku, COALESCE(p.barcode,''), p.name,
			COALESCE(p.brand,''), COALESCE(p.version,''), COALESCE(p.flavor,''),
			COALESCE(p.category,''),
			p.cost_price::float8, p.selling_price::float8,
			p.minimum_stock, p.is_partner_stock, p.active, p.created_at, p.updated_at,
			COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		%s
		ORDER BY p.name ASC
		LIMIT %d OFFSET %d
	`, where, perPage, offset)

	rows, err := database.Pool.Query(ctx, rowsSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var out []ProductWithStock
	for rows.Next() {
		var p ProductWithStock
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Barcode, &p.Name,
			&p.Brand, &p.Version, &p.Flavor, &p.Category,
			&p.CostPrice, &p.SellingPrice,
			&p.MinimumStock, &p.IsPartnerStock, &p.Active, &p.CreatedAt, &p.UpdatedAt,
			&p.CurrentStock,
		); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func ListAll(ctx context.Context) ([]ProductWithStock, error) {
	const q = `
		SELECT
			p.id, p.sku, COALESCE(p.barcode,''), p.name,
			COALESCE(p.brand,''), COALESCE(p.version,''), COALESCE(p.flavor,''),
			COALESCE(p.category,''),
			p.cost_price::float8, p.selling_price::float8,
			p.minimum_stock, p.is_partner_stock, p.active, p.created_at, p.updated_at,
			COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = TRUE
		ORDER BY p.name ASC
		LIMIT 500
	`
	rows, err := database.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all products: %w", err)
	}
	defer rows.Close()

	var out []ProductWithStock
	for rows.Next() {
		var p ProductWithStock
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Barcode, &p.Name,
			&p.Brand, &p.Version, &p.Flavor, &p.Category,
			&p.CostPrice, &p.SellingPrice,
			&p.MinimumStock, &p.IsPartnerStock, &p.Active, &p.CreatedAt, &p.UpdatedAt,
			&p.CurrentStock,
		); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func Get(ctx context.Context, id int64) (*ProductWithStock, error) {
	const q = `
		SELECT
			p.id, p.sku, COALESCE(p.barcode,''), p.name,
			COALESCE(p.brand,''), COALESCE(p.version,''), COALESCE(p.flavor,''),
			COALESCE(p.category,''),
			p.cost_price::float8, p.selling_price::float8,
			p.minimum_stock, p.is_partner_stock, p.active, p.created_at, p.updated_at,
			COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.id = $1 AND p.active = TRUE
	`
	var p ProductWithStock
	err := database.Pool.QueryRow(ctx, q, id).Scan(
		&p.ID, &p.SKU, &p.Barcode, &p.Name,
		&p.Brand, &p.Version, &p.Flavor, &p.Category,
		&p.CostPrice, &p.SellingPrice,
		&p.MinimumStock, &p.IsPartnerStock, &p.Active, &p.CreatedAt, &p.UpdatedAt,
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

func Create(ctx context.Context, p *Product) (int64, error) {
	const q = `
		INSERT INTO products
			(sku, barcode, name, brand, version, flavor, category,
			 cost_price, selling_price, minimum_stock, is_partner_stock, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, TRUE)
		RETURNING id
	`
	var id int64
	err := database.Pool.QueryRow(ctx, q,
		p.SKU, nullIfEmpty(p.Barcode), p.Name,
		nullIfEmpty(p.Brand), nullIfEmpty(p.Version), nullIfEmpty(p.Flavor), nullIfEmpty(p.Category),
		p.CostPrice, p.SellingPrice, p.MinimumStock, p.IsPartnerStock,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create product: %w", err)
	}
	return id, nil
}

func Update(ctx context.Context, p *Product) error {
	const q = `
		UPDATE products SET
			sku = $1, barcode = $2, name = $3, brand = $4, version = $5,
			flavor = $6, category = $7, cost_price = $8, selling_price = $9,
			minimum_stock = $10, is_partner_stock = $11, updated_at = NOW()
		WHERE id = $12 AND active = TRUE
	`
	cmd, err := database.Pool.Exec(ctx, q,
		p.SKU, nullIfEmpty(p.Barcode), p.Name,
		nullIfEmpty(p.Brand), nullIfEmpty(p.Version), nullIfEmpty(p.Flavor), nullIfEmpty(p.Category),
		p.CostPrice, p.SellingPrice, p.MinimumStock, p.IsPartnerStock,
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func SoftDelete(ctx context.Context, id int64) error {
	cmd, err := database.Pool.Exec(ctx, `UPDATE products SET active = FALSE, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("soft delete product: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
