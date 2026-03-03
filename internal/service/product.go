package service

import (
	"context"

	"marketplace/internal/apperror"
	"marketplace/internal/repository"
)

type ProductService struct {
	repo *repository.ProductRepo
}

func NewProductService(repo *repository.ProductRepo) *ProductService {
	return &ProductService{repo: repo}
}

func (s *ProductService) Create(ctx context.Context, p *repository.Product) error {
	return s.repo.Create(ctx, p)
}

func (s *ProductService) GetByID(ctx context.Context, id int64) (*repository.Product, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperror.ProductNotFound(id)
	}
	return p, nil
}

func (s *ProductService) List(ctx context.Context, f repository.ProductFilter) (*repository.ProductListResult, error) {
	return s.repo.List(ctx, f)
}

func (s *ProductService) Update(ctx context.Context, id int64, updater func(p *repository.Product)) (*repository.Product, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperror.ProductNotFound(id)
	}

	updater(p)
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

func (s *ProductService) SoftDelete(ctx context.Context, id int64) error {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return apperror.ProductNotFound(id)
	}
	return s.repo.SoftDelete(ctx, id)
}
