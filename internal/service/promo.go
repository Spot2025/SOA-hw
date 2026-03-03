package service

import (
	"context"

	"marketplace/internal/repository"
)

type PromoService struct {
	repo *repository.PromoRepo
}

func NewPromoService(repo *repository.PromoRepo) *PromoService {
	return &PromoService{repo: repo}
}

func (s *PromoService) Create(ctx context.Context, p *repository.PromoCode) error {
	return s.repo.Create(ctx, p)
}
