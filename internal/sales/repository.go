package sales

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jmechavez/vapeshop/internal/database"
)

func List(ctx context.Context, f Filter, page, perPage int) ([]Sale, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	offset := (page - 1) * perPage

	var conds []string
	var args []any
	n := 0
	add := func(cond string, val any) {
		n++
		conds = append(conds, fmt.Sprintf(cond, n))
		args = append(args, val)
	}

	if f.Status != "" && f.Status != "all" {
		add("s.status = $%d", f.Status)
	}
	if f.DateFrom != "" {
		add("(s.sale_date AT TIME ZONE 'Asia/Manila')::date >= $%d::date", f.DateFrom)
	}
	if f.DateTo != "" {
		add("(s.sale_date AT TIME ZONE 'Asia/Manila')::date <= $%d::date", f.DateTo)
	}
	if f.Reference != "" {
		n++
		like := "%" + f.Reference + "%"
		conds = append(conds, fmt.Sprintf("s.reference ILIKE $%d", n))
		args = append(args, like)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM sales s %s`, where)
	if err := database.Pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sales: %w", err)
	}

	pageSQL := fmt.Sprintf(`
		SELECT
			s.id, s.reference, s.user_id, COALESCE(u.username, ''),
			s.sale_date, s.subtotal, s.discount, s.total,
			s.payment_method, s.status, s.created_at,
			(SELECT COUNT(*)::int FROM sale_items WHERE sale_id = s.id)
		FROM sales s
		LEFT JOIN users u ON u.id = s.user_id
		%s
		ORDER BY s.sale_date DESC, s.id DESC
		LIMIT %d OFFSET %d
	`, where, perPage, offset)

	rows, err := database.Pool.Query(ctx, pageSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list sales: %w", err)
	}
	defer rows.Close()

	var out []Sale
	for rows.Next() {
		var s Sale
		if err := rows.Scan(
			&s.ID, &s.Reference, &s.UserID, &s.CashierName,
			&s.SaleDate, &s.Subtotal, &s.Discount, &s.Total,
			&s.PaymentMethod, &s.Status, &s.CreatedAt, &s.ItemCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan sale: %w", err)
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func GetByID(ctx context.Context, id int64) (Sale, error) {
	const q = `
		SELECT
			s.id, s.reference, s.user_id, COALESCE(u.username, ''),
			s.sale_date, s.subtotal, s.discount, s.total,
			s.payment_method, s.status, s.created_at,
			COALESCE(s.void_reason, ''),
			COALESCE(s.voided_by, 0),
			COALESCE(v.username, ''),
			s.voided_at
		FROM sales s
		LEFT JOIN users u ON u.id = s.user_id
		LEFT JOIN users v ON v.id = s.voided_by
		WHERE s.id = $1
	`
	var s Sale
	err := database.Pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.Reference, &s.UserID, &s.CashierName,
		&s.SaleDate, &s.Subtotal, &s.Discount, &s.Total,
		&s.PaymentMethod, &s.Status, &s.CreatedAt,
		&s.VoidReason, &s.VoidedBy, &s.VoidedByName, &s.VoidedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sale{}, ErrNotFound
		}
		return Sale{}, fmt.Errorf("get sale: %w", err)
	}

	itemRows, err := database.Pool.Query(ctx, `
		SELECT id, sale_id, product_id, sku, product_name, quantity, unit_price, subtotal
		FROM sale_items WHERE sale_id = $1 ORDER BY id ASC
	`, id)
	if err != nil {
		return Sale{}, fmt.Errorf("get items: %w", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var it SaleItem
		if err := itemRows.Scan(
			&it.ID, &it.SaleID, &it.ProductID, &it.SKU, &it.ProductName,
			&it.Quantity, &it.UnitPrice, &it.Subtotal,
		); err != nil {
			return Sale{}, fmt.Errorf("scan item: %w", err)
		}
		s.Items = append(s.Items, it)
	}
	s.ItemCount = len(s.Items)

	payRows, err := database.Pool.Query(ctx, `SELECT method, amount::float8 FROM sale_payments WHERE sale_id = $1`, id)
	if err == nil {
		defer payRows.Close()
		for payRows.Next() {
			var p SplitPayment
			if err := payRows.Scan(&p.Method, &p.Amount); err == nil {
				s.Payments = append(s.Payments, p)
			}
		}
	}

	return s, itemRows.Err()
}

func FindLatestPendingForUser(ctx context.Context, userID int64) (int64, bool, error) {
	var id int64
	err := database.Pool.QueryRow(ctx,
		`SELECT id FROM sales WHERE user_id = $1 AND status = 'pending' ORDER BY id DESC LIMIT 1`,
		userID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

func CreatePending(ctx context.Context, userID int64, method string) (int64, error) {
	var id int64
	err := database.Pool.QueryRow(ctx, `
		INSERT INTO sales (reference, user_id, payment_method, status)
		VALUES ('PENDING', $1, $2, 'pending')
		RETURNING id
	`, userID, method).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create pending sale: %w", err)
	}
	_, err = database.Pool.Exec(ctx, `
		UPDATE sales SET reference = 'S-' || to_char(NOW(), 'YYYYMMDD') || '-' || id WHERE id = $1
	`, id)
	if err != nil {
		return 0, fmt.Errorf("update ref: %w", err)
	}
	return id, nil
}

func SearchProducts(ctx context.Context, q string) ([]ProductSuggestion, error) {
	if strings.TrimSpace(q) == "" {
		return []ProductSuggestion{}, nil
	}
	const sql = `
		SELECT p.id, p.sku, p.name, p.selling_price::float8, COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = TRUE
		  AND (p.name ILIKE $1 OR p.sku ILIKE $1 OR COALESCE(p.brand, '') ILIKE $1)
		ORDER BY p.name ASC
		LIMIT 20
	`
	rows, err := database.Pool.Query(ctx, sql, "%"+q+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProductSuggestion
	for rows.Next() {
		var p ProductSuggestion
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.SellingPrice, &p.CurrentStock); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func updateSaleTotals(ctx context.Context, tx pgx.Tx, saleID int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE sales 
		SET subtotal = (SELECT COALESCE(SUM(subtotal), 0) FROM sale_items WHERE sale_id = $1),
		    total = (SELECT COALESCE(SUM(subtotal), 0) FROM sale_items WHERE sale_id = $1) - discount
		WHERE id = $1
	`, saleID)
	return err
}

func AddItem(ctx context.Context, saleID, productID int64, qty int) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var stock int
	err = tx.QueryRow(ctx, `SELECT COALESCE(current_stock, 0) FROM inventory WHERE product_id = $1`, productID).Scan(&stock)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var existingQty int
	_ = tx.QueryRow(ctx, `SELECT quantity FROM sale_items WHERE sale_id = $1 AND product_id = $2`, saleID, productID).Scan(&existingQty)

	if stock < existingQty+qty {
		return ErrInsufficientStock
	}

	var sku, name string
	var price float64
	err = tx.QueryRow(ctx, `SELECT sku, name, selling_price::float8 FROM products WHERE id = $1`, productID).Scan(&sku, &name, &price)
	if err != nil {
		return err
	}

	if existingQty > 0 {
		_, err = tx.Exec(ctx, `
			UPDATE sale_items 
			SET quantity = quantity + $1::int, 
			    subtotal = (quantity + $1::int)::numeric * unit_price 
			WHERE sale_id = $2 AND product_id = $3
		`, qty, saleID, productID)
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO sale_items (sale_id, product_id, sku, product_name, quantity, unit_price, subtotal) 
			VALUES ($1, $2, $3, $4, $5::int, $6::numeric, $5::int * $6::numeric)
		`, saleID, productID, sku, name, qty, price)
	}
	if err != nil {
		return err
	}

	if err := updateSaleTotals(ctx, tx, saleID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func UpdateItemQty(ctx context.Context, saleID, itemID int64, qty int) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if qty <= 0 {
		_, err = tx.Exec(ctx, `DELETE FROM sale_items WHERE id = $1 AND sale_id = $2`, itemID, saleID)
	} else {
		var productID int64
		_ = tx.QueryRow(ctx, `SELECT product_id FROM sale_items WHERE id = $1 AND sale_id = $2`, itemID, saleID).Scan(&productID)
		var stock int
		_ = tx.QueryRow(ctx, `SELECT COALESCE(current_stock, 0) FROM inventory WHERE product_id = $1`, productID).Scan(&stock)
		if stock < qty {
			return ErrInsufficientStock
		}
		_, err = tx.Exec(ctx, `
			UPDATE sale_items 
			SET quantity = $1::int, 
			    subtotal = $1::int * unit_price 
			WHERE id = $2 AND sale_id = $3
		`, qty, itemID, saleID)
	}
	if err != nil {
		return err
	}

	if err := updateSaleTotals(ctx, tx, saleID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func RemoveItem(ctx context.Context, saleID, itemID int64) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM sale_items WHERE id = $1 AND sale_id = $2`, itemID, saleID)
	if err != nil {
		return err
	}

	if err := updateSaleTotals(ctx, tx, saleID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func UpdateDiscount(ctx context.Context, saleID int64, disc float64) error {
	_, err := database.Pool.Exec(ctx, `UPDATE sales SET discount = $1, total = subtotal - $1 WHERE id = $2 AND status = 'pending'`, disc, saleID)
	return err
}

func UpdateSplitPayments(ctx context.Context, saleID int64, payments []SplitPayment) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM sale_payments WHERE sale_id = $1`, saleID)
	if err != nil {
		return err
	}

	primaryMethod := ""
	for _, p := range payments {
		if p.Amount > 0 {
			_, err = tx.Exec(ctx, `INSERT INTO sale_payments (sale_id, method, amount) VALUES ($1, $2, $3)`, saleID, p.Method, p.Amount)
			if err != nil {
				return err
			}
			if primaryMethod == "" {
				primaryMethod = p.Method
			} else if primaryMethod != p.Method {
				primaryMethod = "Split"
			}
		}
	}

	if primaryMethod == "" {
		primaryMethod = PaymentCash
	}

	_, err = tx.Exec(ctx, `UPDATE sales SET payment_method = $1 WHERE id = $2`, primaryMethod, saleID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// COMPLETE with backdate feature
func Complete(ctx context.Context, saleID, userID int64, saleDate string) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// If saleDate is provided, use it. Otherwise, keep the current sale_date.
	//
	// We want: sale_date = chosen_date at "now" (in Asia/Manila wall-clock time),
	// stored as a proper timestamptz so the app displays the picked day.
	dateClause := ""
	var args []any
	args = append(args, saleID)

	if saleDate != "" {
		dateClause = `, sale_date = (
			($2 || ' ' || to_char(NOW() AT TIME ZONE 'Asia/Manila', 'HH24:MI:SS'))::timestamp
		) AT TIME ZONE 'Asia/Manila'`
		args = append(args, saleDate)
	}

	query := fmt.Sprintf(`
		UPDATE sales s
		SET subtotal = (SELECT COALESCE(SUM(subtotal), 0) FROM sale_items WHERE sale_id = s.id),
		    total = (SELECT COALESCE(SUM(subtotal), 0) FROM sale_items WHERE sale_id = s.id) - s.discount,
		    status = 'completed'
		    %s
		WHERE id = $1 AND status = 'pending'
	`, dateClause)

	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM sales WHERE id = $1`, saleID).Scan(&status)
	if err != nil {
		return err
	}
	if status != "completed" {
		return ErrInvalidStatus
	}

	rows, err := tx.Query(ctx, `SELECT product_id, quantity FROM sale_items WHERE sale_id = $1`, saleID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type deduction struct {
		productID int64
		qty       int
	}
	var items []deduction
	for rows.Next() {
		var d deduction
		if err := rows.Scan(&d.productID, &d.qty); err != nil {
			return err
		}
		items = append(items, d)
	}

	var ref string
	_ = tx.QueryRow(ctx, `SELECT reference FROM sales WHERE id = $1`, saleID).Scan(&ref)

	for _, d := range items {
		_, err = tx.Exec(ctx, `
			INSERT INTO stock_movements (product_id, user_id, type, quantity, reference, notes)
			VALUES ($1, $2, 'out', $3, $4, 'Sale completion')
		`, d.productID, userID, d.qty, ref)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE inventory SET current_stock = current_stock - $1, updated_at = NOW()
			WHERE product_id = $2
		`, d.qty, d.productID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func Void(ctx context.Context, saleID, userID int64, in VoidInput) error {
	if in.FinalReason() == "" {
		return ErrVoidReasonMissing
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM sales WHERE id = $1`, saleID).Scan(&status)
	if err != nil {
		return err
	}
	if status == "void" {
		return ErrInvalidStatus
	}

	_, err = tx.Exec(ctx, `
		UPDATE sales
		SET status = 'void', void_reason = $1, voided_by = $2, voided_at = NOW()
		WHERE id = $3
	`, in.FinalReason(), userID, saleID)
	if err != nil {
		return err
	}

	if status == "completed" {
		rows, err := tx.Query(ctx, `SELECT product_id, quantity FROM sale_items WHERE sale_id = $1`, saleID)
		if err != nil {
			return err
		}
		defer rows.Close()

		type item struct {
			productID int64
			qty       int
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.productID, &it.qty); err != nil {
				return err
			}
			items = append(items, it)
		}

		var ref string
		_ = tx.QueryRow(ctx, `SELECT reference FROM sales WHERE id = $1`, saleID).Scan(&ref)

		for _, it := range items {
			_, err = tx.Exec(ctx, `
				INSERT INTO stock_movements (product_id, user_id, type, quantity, reference, notes)
				VALUES ($1, $2, 'in', $3, $4, 'Sale void')
			`, it.productID, userID, it.qty, "VOID "+ref)
			if err != nil {
				return err
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO inventory (product_id, current_stock, updated_at)
				VALUES ($1, $2, NOW())
				ON CONFLICT (product_id) DO UPDATE
				SET current_stock = inventory.current_stock + EXCLUDED.current_stock, updated_at = NOW()
			`, it.productID, it.qty)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}
