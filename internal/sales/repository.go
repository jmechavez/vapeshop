package sales

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jmechavez/vapeshop/internal/database"
)

// ---------- List ----------

func List(ctx context.Context, f Filter) ([]Sale, error) {
	var sb strings.Builder
	args := []any{}
	sb.WriteString(`
		SELECT s.id, s.reference, COALESCE(s.user_id, 0), COALESCE(u.username, ''),
		       s.sale_date, s.subtotal::float8, s.discount::float8, s.total::float8,
		       s.payment_method, s.status, s.created_at,
		       COALESCE(s.void_reason, ''),
		       COALESCE(s.voided_by, 0),
		       COALESCE(vu.username, ''),
		       s.voided_at,
		       COALESCE(cnt.c, 0)
		FROM sales s
		LEFT JOIN users u  ON u.id  = s.user_id
		LEFT JOIN users vu ON vu.id = s.voided_by
		LEFT JOIN (
			SELECT sale_id, COUNT(*)::int AS c
			FROM sale_items
			GROUP BY sale_id
		) cnt ON cnt.sale_id = s.id
		WHERE 1 = 1
	`)

	if f.Status != "" && f.Status != "all" {
		args = append(args, f.Status)
		sb.WriteString(fmt.Sprintf(" AND s.status = $%d", len(args)))
	}
	if f.DateFrom != "" {
		args = append(args, f.DateFrom)
		sb.WriteString(fmt.Sprintf(" AND s.sale_date >= $%d::date", len(args)))
	}
	if f.DateTo != "" {
		args = append(args, f.DateTo)
		sb.WriteString(fmt.Sprintf(" AND s.sale_date < ($%d::date + INTERVAL '1 day')", len(args)))
	}
	if f.Reference != "" {
		args = append(args, "%"+f.Reference+"%")
		sb.WriteString(fmt.Sprintf(" AND s.reference ILIKE $%d", len(args)))
	}
	sb.WriteString(" ORDER BY s.sale_date DESC, s.id DESC")

	rows, err := database.Pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Sale
	for rows.Next() {
		var s Sale
		if err := rows.Scan(
			&s.ID, &s.Reference, &s.UserID, &s.CashierName,
			&s.SaleDate, &s.Subtotal, &s.Discount, &s.Total,
			&s.PaymentMethod, &s.Status, &s.CreatedAt,
			&s.VoidReason, &s.VoidedBy, &s.VoidedByName, &s.VoidedAt,
			&s.ItemCount,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------- Detail ----------

func GetByID(ctx context.Context, id int64) (Sale, error) {
	var s Sale
	err := database.Pool.QueryRow(ctx, `
		SELECT s.id, s.reference, COALESCE(s.user_id, 0), COALESCE(u.username, ''),
		       s.sale_date, s.subtotal::float8, s.discount::float8, s.total::float8,
		       s.payment_method, s.status, s.created_at,
		       COALESCE(s.void_reason, ''),
		       COALESCE(s.voided_by, 0),
		       COALESCE(vu.username, ''),
		       s.voided_at
		FROM sales s
		LEFT JOIN users u  ON u.id  = s.user_id
		LEFT JOIN users vu ON vu.id = s.voided_by
		WHERE s.id = $1
	`, id).Scan(
		&s.ID, &s.Reference, &s.UserID, &s.CashierName,
		&s.SaleDate, &s.Subtotal, &s.Discount, &s.Total,
		&s.PaymentMethod, &s.Status, &s.CreatedAt,
		&s.VoidReason, &s.VoidedBy, &s.VoidedByName, &s.VoidedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sale{}, ErrNotFound
		}
		return Sale{}, err
	}

	items, err := listItems(ctx, id)
	if err != nil {
		return Sale{}, err
	}
	s.Items = items
	s.ItemCount = len(items)
	return s, nil
}

func listItems(ctx context.Context, saleID int64) ([]SaleItem, error) {
	rows, err := database.Pool.Query(ctx, `
		SELECT id, sale_id, product_id, sku, product_name,
		       quantity, unit_price::float8, subtotal::float8
		FROM sale_items
		WHERE sale_id = $1
		ORDER BY id
	`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SaleItem
	for rows.Next() {
		var it SaleItem
		if err := rows.Scan(
			&it.ID, &it.SaleID, &it.ProductID, &it.SKU, &it.ProductName,
			&it.Quantity, &it.UnitPrice, &it.Subtotal,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ---------- Find latest pending ----------

func FindLatestPendingForUser(ctx context.Context, userID int64) (int64, bool, error) {
	var id int64
	err := database.Pool.QueryRow(ctx, `
		SELECT id FROM sales
		WHERE user_id = $1 AND status = 'pending'
		ORDER BY id DESC
		LIMIT 1
	`, userID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

// ---------- Create pending ----------

func CreatePending(ctx context.Context, userID int64, paymentMethod string) (int64, error) {
	if paymentMethod == "" {
		paymentMethod = PaymentCash
	}

	ref := generateReference()
	var id int64
	for attempt := 0; attempt < 5; attempt++ {
		err := database.Pool.QueryRow(ctx, `
			INSERT INTO sales (reference, user_id, payment_method, status, subtotal, discount, total)
			VALUES ($1, $2, $3, 'pending', 0, 0, 0)
			RETURNING id
		`, ref, userID, paymentMethod).Scan(&id)

		if err == nil {
			return id, nil
		}
		if isUniqueViolation(err) {
			ref = generateReference()
			continue
		}
		return 0, err
	}
	return 0, fmt.Errorf("could not generate a unique sale reference")
}

func generateReference() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return "S-" + time.Now().Format("20060102") + "-" + string(b)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// ---------- Cart operations ----------

func AddItem(ctx context.Context, saleID, productID int64, qty int) error {
	if qty <= 0 {
		qty = 1
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockPendingSale(ctx, tx, saleID); err != nil {
		return err
	}

	var sku, name string
	var price float64
	err = tx.QueryRow(ctx, `
		SELECT sku, name, selling_price::float8
		FROM products
		WHERE id = $1 AND active = true
	`, productID).Scan(&sku, &name, &price)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// Check available stock for this product.
	var currentStock int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(current_stock, 0) FROM inventory WHERE product_id = $1 FOR UPDATE
	`, productID).Scan(&currentStock)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// How much is already in this cart for the same product?
	var existingID int64
	var existingQty int
	err = tx.QueryRow(ctx, `
		SELECT id, quantity FROM sale_items
		WHERE sale_id = $1 AND product_id = $2
	`, saleID, productID).Scan(&existingID, &existingQty)
	alreadyInCart := 0
	if err == nil {
		alreadyInCart = existingQty
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if alreadyInCart+qty > currentStock {
		return fmt.Errorf("%w: %s (have %d, need %d)", ErrInsufficientStock, sku, currentStock, alreadyInCart+qty)
	}

	if existingID > 0 {
		newQty := existingQty + qty
		_, err = tx.Exec(ctx, `
			UPDATE sale_items
			SET quantity = $1::int,
			    subtotal = $1::int * unit_price
			WHERE id = $2
		`, newQty, existingID)
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO sale_items (sale_id, product_id, sku, product_name, quantity, unit_price, subtotal)
			VALUES ($1, $2, $3, $4, $5::int, $6::numeric, $5::int * $6::numeric)
		`, saleID, productID, sku, name, qty, price)
	}
	if err != nil {
		return err
	}

	if err := recomputeTotals(ctx, tx, saleID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func UpdateItemQty(ctx context.Context, saleID, itemID int64, qty int) error {
	if qty <= 0 {
		return RemoveItem(ctx, saleID, itemID)
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockPendingSale(ctx, tx, saleID); err != nil {
		return err
	}

	// Stock check: the new qty must not exceed available stock.
	var productID int64
	var currentStock int
	err = tx.QueryRow(ctx, `
		SELECT si.product_id, COALESCE(i.current_stock, 0)
		FROM sale_items si
		LEFT JOIN inventory i ON i.product_id = si.product_id
		WHERE si.id = $1 AND si.sale_id = $2
	`, itemID, saleID).Scan(&productID, &currentStock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if qty > currentStock {
		return fmt.Errorf("%w: have %d, need %d", ErrInsufficientStock, currentStock, qty)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE sale_items
		SET quantity = $1::int,
		    subtotal = $1::int * unit_price
		WHERE id = $2 AND sale_id = $3
	`, qty, itemID, saleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if err := recomputeTotals(ctx, tx, saleID); err != nil {
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

	if err := lockPendingSale(ctx, tx, saleID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM sale_items WHERE id = $1 AND sale_id = $2
	`, itemID, saleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if err := recomputeTotals(ctx, tx, saleID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func UpdateDiscount(ctx context.Context, saleID int64, discount float64) error {
	if discount < 0 {
		discount = 0
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockPendingSale(ctx, tx, saleID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sales SET discount = $1::numeric WHERE id = $2
	`, discount, saleID); err != nil {
		return err
	}

	if err := recomputeTotals(ctx, tx, saleID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func UpdatePayment(ctx context.Context, saleID int64, method string) error {
	valid := false
	for _, m := range PaymentMethods {
		if m == method {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid payment method: %s", method)
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockPendingSale(ctx, tx, saleID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sales SET payment_method = $1 WHERE id = $2
	`, method, saleID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------- Complete ----------

func Complete(ctx context.Context, saleID, userID int64) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status, ref string
	err = tx.QueryRow(ctx, `
		SELECT status, reference FROM sales WHERE id = $1 FOR UPDATE
	`, saleID).Scan(&status, &ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != StatusPending {
		return ErrInvalidStatus
	}

	type line struct {
		ProductID int64
		SKU       string
		Qty       int
	}
	rows, err := tx.Query(ctx, `
		SELECT product_id, sku, quantity FROM sale_items WHERE sale_id = $1
	`, saleID)
	if err != nil {
		return err
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.ProductID, &l.SKU, &l.Qty); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return ErrEmptyCart
	}

	for _, l := range lines {
		var stock int
		err = tx.QueryRow(ctx, `
			SELECT current_stock FROM inventory WHERE product_id = $1 FOR UPDATE
		`, l.ProductID).Scan(&stock)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: %s has no inventory row", ErrInsufficientStock, l.SKU)
			}
			return err
		}
		if stock < l.Qty {
			return fmt.Errorf("%w: %s (have %d, need %d)", ErrInsufficientStock, l.SKU, stock, l.Qty)
		}
	}

	for _, l := range lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_movements
				(product_id, user_id, supplier_id, type, quantity, unit_cost, reference, notes, movement_date)
			VALUES ($1, $2, NULL, 'out', $3::int, NULL, $4, $5, now())
		`, l.ProductID, userID, l.Qty, ref, "Sale "+ref); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE inventory
			SET current_stock = current_stock - $1::int,
			    updated_at = now()
			WHERE product_id = $2
		`, l.Qty, l.ProductID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sales
		SET status = 'completed',
		    sale_date = now(),
		    user_id = $1
		WHERE id = $2
	`, userID, saleID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------- Void ----------

func Void(ctx context.Context, saleID, userID int64, in VoidInput) error {
	reason := strings.TrimSpace(in.FinalReason())
	if reason == "" {
		return ErrVoidReasonMissing
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status, ref string
	err = tx.QueryRow(ctx, `
		SELECT status, reference FROM sales WHERE id = $1 FOR UPDATE
	`, saleID).Scan(&status, &ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != StatusPending && status != StatusCompleted {
		return ErrInvalidStatus
	}

	if status == StatusCompleted {
		type line struct {
			ProductID int64
			Qty       int
		}
		rows, err := tx.Query(ctx, `
			SELECT product_id, quantity FROM sale_items WHERE sale_id = $1
		`, saleID)
		if err != nil {
			return err
		}
		var lines []line
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.ProductID, &l.Qty); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		voidRef := "VOID " + ref
		for _, l := range lines {
			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_movements
					(product_id, user_id, supplier_id, type, quantity, unit_cost, reference, notes, movement_date)
				VALUES ($1, $2, NULL, 'in', $3::int, NULL, $4, $5, now())
			`, l.ProductID, userID, l.Qty, voidRef, "Void "+ref); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE inventory
				SET current_stock = current_stock + $1::int,
				    updated_at = now()
				WHERE product_id = $2
			`, l.Qty, l.ProductID); err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sales
		SET status = 'void',
		    void_reason = $1,
		    voided_by = $2,
		    voided_at = now()
		WHERE id = $3
	`, reason, userID, saleID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------- Product search ----------

func SearchProducts(ctx context.Context, q string) ([]ProductSuggestion, error) {
	q = strings.TrimSpace(q)

	var sb strings.Builder
	args := []any{}
	sb.WriteString(`
		SELECT p.id, p.sku, p.name, p.selling_price::float8,
		       COALESCE(i.current_stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.active = true
	`)
	if q != "" {
		args = append(args, "%"+q+"%")
		sb.WriteString(fmt.Sprintf(
			" AND (p.name ILIKE $%d OR p.sku ILIKE $%d)",
			len(args), len(args),
		))
	}
	sb.WriteString(" ORDER BY p.name LIMIT 20")

	rows, err := database.Pool.Query(ctx, sb.String(), args...)
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

// ---------- helpers ----------

func lockPendingSale(ctx context.Context, tx pgx.Tx, saleID int64) error {
	var status string
	err := tx.QueryRow(ctx, `
		SELECT status FROM sales WHERE id = $1 FOR UPDATE
	`, saleID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != StatusPending {
		return ErrInvalidStatus
	}
	return nil
}

func recomputeTotals(ctx context.Context, tx pgx.Tx, saleID int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE sales
		SET subtotal = COALESCE(
				(SELECT SUM(subtotal) FROM sale_items WHERE sale_id = sales.id), 0),
		    total = GREATEST(
				COALESCE((SELECT SUM(subtotal) FROM sale_items WHERE sale_id = sales.id), 0)
				- sales.discount, 0)
		WHERE id = $1
	`, saleID)
	return err
}
