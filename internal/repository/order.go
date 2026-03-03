package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Order struct {
	ID             int64
	UserID         int64
	Status         string
	PromoCodeID    *int64
	TotalAmount    float64
	DiscountAmount float64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type OrderItem struct {
	ID           int64
	OrderID      int64
	ProductID    int64
	Quantity     int
	PriceAtOrder float64
}

type OrderRepo struct {
	pool *pgxpool.Pool
}

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{pool: pool}
}

func (r *OrderRepo) GetByID(ctx context.Context, id int64) (*Order, error) {
	o := &Order{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, promo_code_id, total_amount, discount_amount, created_at, updated_at
		 FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.PromoCodeID, &o.TotalAmount, &o.DiscountAmount, &o.CreatedAt, &o.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return o, err
}

func (r *OrderRepo) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (*Order, error) {
	o := &Order{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, status, promo_code_id, total_amount, discount_amount, created_at, updated_at
		 FROM orders WHERE id = $1 FOR UPDATE`, id,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.PromoCodeID, &o.TotalAmount, &o.DiscountAmount, &o.CreatedAt, &o.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return o, err
}

func (r *OrderRepo) HasActiveOrder(ctx context.Context, tx pgx.Tx, userID int64) (bool, error) {
	var count int
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE user_id = $1 AND status IN ('CREATED','PAYMENT_PENDING')`, userID,
	).Scan(&count)
	return count > 0, err
}

func (r *OrderRepo) CreateTx(ctx context.Context, tx pgx.Tx, o *Order) error {
	return tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, status, promo_code_id, total_amount, discount_amount)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at, updated_at`,
		o.UserID, o.Status, o.PromoCodeID, o.TotalAmount, o.DiscountAmount,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
}

func (r *OrderRepo) UpdateTx(ctx context.Context, tx pgx.Tx, o *Order) error {
	return tx.QueryRow(ctx,
		`UPDATE orders SET status=$1, promo_code_id=$2, total_amount=$3, discount_amount=$4
		 WHERE id=$5 RETURNING updated_at`,
		o.Status, o.PromoCodeID, o.TotalAmount, o.DiscountAmount, o.ID,
	).Scan(&o.UpdatedAt)
}

func (r *OrderRepo) CreateItemsTx(ctx context.Context, tx pgx.Tx, items []OrderItem) error {
	for i := range items {
		err := tx.QueryRow(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, price_at_order)
			 VALUES ($1,$2,$3,$4) RETURNING id`,
			items[i].OrderID, items[i].ProductID, items[i].Quantity, items[i].PriceAtOrder,
		).Scan(&items[i].ID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *OrderRepo) DeleteItemsTx(ctx context.Context, tx pgx.Tx, orderID int64) error {
	_, err := tx.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, orderID)
	return err
}

func (r *OrderRepo) GetItemsByOrderID(ctx context.Context, orderID int64) ([]OrderItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, order_id, product_id, quantity, price_at_order
		 FROM order_items WHERE order_id = $1`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var it OrderItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Quantity, &it.PriceAtOrder); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

func (r *OrderRepo) GetItemsByOrderIDTx(ctx context.Context, tx pgx.Tx, orderID int64) ([]OrderItem, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, order_id, product_id, quantity, price_at_order
		 FROM order_items WHERE order_id = $1`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var it OrderItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Quantity, &it.PriceAtOrder); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

func (r *OrderRepo) Pool() *pgxpool.Pool {
	return r.pool
}
