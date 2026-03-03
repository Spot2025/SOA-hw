package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserOperation struct {
	ID            int64
	UserID        int64
	OperationType string
	CreatedAt     time.Time
}

type UserOpRepo struct {
	pool *pgxpool.Pool
}

func NewUserOpRepo(pool *pgxpool.Pool) *UserOpRepo {
	return &UserOpRepo{pool: pool}
}

func (r *UserOpRepo) GetLastOperation(ctx context.Context, tx pgx.Tx, userID int64, opType string) (*UserOperation, error) {
	op := &UserOperation{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, operation_type, created_at
		 FROM user_operations
		 WHERE user_id = $1 AND operation_type = $2
		 ORDER BY created_at DESC LIMIT 1`, userID, opType,
	).Scan(&op.ID, &op.UserID, &op.OperationType, &op.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return op, err
}

func (r *UserOpRepo) CreateTx(ctx context.Context, tx pgx.Tx, userID int64, opType string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO user_operations (user_id, operation_type) VALUES ($1, $2)`, userID, opType)
	return err
}
