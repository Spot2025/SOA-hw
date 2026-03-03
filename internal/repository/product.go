package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Product struct {
	ID          int64
	Name        string
	Description *string
	Price       float64
	Stock       int
	Category    string
	Status      string
	SellerID    *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ProductFilter struct {
	Status   *string
	Category *string
	Page     int
	Size     int
}

type ProductListResult struct {
	Items         []Product
	TotalElements int64
}

type ProductRepo struct {
	pool *pgxpool.Pool
}

func NewProductRepo(pool *pgxpool.Pool) *ProductRepo {
	return &ProductRepo{pool: pool}
}

func (r *ProductRepo) Create(ctx context.Context, p *Product) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO products (name, description, price, stock, category, status, seller_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id, created_at, updated_at`,
		p.Name, p.Description, p.Price, p.Stock, p.Category, p.Status, p.SellerID,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *ProductRepo) GetByID(ctx context.Context, id int64) (*Product, error) {
	p := &Product{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, price, stock, category, status, seller_id, created_at, updated_at
		 FROM products WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.Category, &p.Status, &p.SellerID, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *ProductRepo) List(ctx context.Context, f ProductFilter) (*ProductListResult, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	idx := 1

	if f.Status != nil {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, *f.Status)
		idx++
	}
	if f.Category != nil {
		where += fmt.Sprintf(" AND category = $%d", idx)
		args = append(args, *f.Category)
		idx++
	}

	var total int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM products "+where, args...).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := f.Page * f.Size
	query := fmt.Sprintf(
		`SELECT id, name, description, price, stock, category, status, seller_id, created_at, updated_at
		 FROM products %s ORDER BY id LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, f.Size, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.Category, &p.Status, &p.SellerID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return &ProductListResult{Items: items, TotalElements: total}, nil
}

func (r *ProductRepo) Update(ctx context.Context, p *Product) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE products SET name=$1, description=$2, price=$3, stock=$4, category=$5, status=$6
		 WHERE id=$7`,
		p.Name, p.Description, p.Price, p.Stock, p.Category, p.Status, p.ID)
	return err
}

func (r *ProductRepo) SoftDelete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE products SET status='ARCHIVED' WHERE id=$1`, id)
	return err
}

func (r *ProductRepo) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (*Product, error) {
	p := &Product{}
	err := tx.QueryRow(ctx,
		`SELECT id, name, description, price, stock, category, status, seller_id, created_at, updated_at
		 FROM products WHERE id = $1 FOR UPDATE`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.Category, &p.Status, &p.SellerID, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *ProductRepo) UpdateStockTx(ctx context.Context, tx pgx.Tx, productID int64, delta int) error {
	_, err := tx.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id = $2`, delta, productID)
	return err
}

func (r *ProductRepo) Pool() *pgxpool.Pool {
	return r.pool
}
