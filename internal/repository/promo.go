package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PromoCode struct {
	ID             int64
	Code           string
	DiscountType   string
	DiscountValue  float64
	MinOrderAmount float64
	MaxUses        int
	CurrentUses    int
	ValidFrom      time.Time
	ValidUntil     time.Time
	Active         bool
}

type PromoRepo struct {
	pool *pgxpool.Pool
}

func NewPromoRepo(pool *pgxpool.Pool) *PromoRepo {
	return &PromoRepo{pool: pool}
}

func (r *PromoRepo) Create(ctx context.Context, p *PromoCode) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO promo_codes (code, discount_type, discount_value, min_order_amount, max_uses, valid_from, valid_until)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id`,
		p.Code, p.DiscountType, p.DiscountValue, p.MinOrderAmount, p.MaxUses, p.ValidFrom, p.ValidUntil,
	).Scan(&p.ID)
}

func (r *PromoRepo) GetByCode(ctx context.Context, code string) (*PromoCode, error) {
	p := &PromoCode{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, code, discount_type, discount_value, min_order_amount, max_uses, current_uses, valid_from, valid_until, active
		 FROM promo_codes WHERE code = $1`, code,
	).Scan(&p.ID, &p.Code, &p.DiscountType, &p.DiscountValue, &p.MinOrderAmount, &p.MaxUses, &p.CurrentUses, &p.ValidFrom, &p.ValidUntil, &p.Active)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *PromoRepo) GetByCodeForUpdateTx(ctx context.Context, tx pgx.Tx, code string) (*PromoCode, error) {
	p := &PromoCode{}
	err := tx.QueryRow(ctx,
		`SELECT id, code, discount_type, discount_value, min_order_amount, max_uses, current_uses, valid_from, valid_until, active
		 FROM promo_codes WHERE code = $1 FOR UPDATE`, code,
	).Scan(&p.ID, &p.Code, &p.DiscountType, &p.DiscountValue, &p.MinOrderAmount, &p.MaxUses, &p.CurrentUses, &p.ValidFrom, &p.ValidUntil, &p.Active)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *PromoRepo) GetByIDForUpdateTx(ctx context.Context, tx pgx.Tx, id int64) (*PromoCode, error) {
	p := &PromoCode{}
	err := tx.QueryRow(ctx,
		`SELECT id, code, discount_type, discount_value, min_order_amount, max_uses, current_uses, valid_from, valid_until, active
		 FROM promo_codes WHERE id = $1 FOR UPDATE`, id,
	).Scan(&p.ID, &p.Code, &p.DiscountType, &p.DiscountValue, &p.MinOrderAmount, &p.MaxUses, &p.CurrentUses, &p.ValidFrom, &p.ValidUntil, &p.Active)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *PromoRepo) IncrementUsesTx(ctx context.Context, tx pgx.Tx, id int64) error {
	_, err := tx.Exec(ctx, `UPDATE promo_codes SET current_uses = current_uses + 1 WHERE id = $1`, id)
	return err
}

func (r *PromoRepo) DecrementUsesTx(ctx context.Context, tx pgx.Tx, id int64) error {
	_, err := tx.Exec(ctx, `UPDATE promo_codes SET current_uses = current_uses - 1 WHERE id = $1 AND current_uses > 0`, id)
	return err
}
