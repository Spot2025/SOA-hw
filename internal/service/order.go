package service

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"marketplace/internal/apperror"
	"marketplace/internal/config"
	"marketplace/internal/repository"
)

var validTransitions = map[string][]string{
	"CREATED":         {"PAYMENT_PENDING", "CANCELED"},
	"PAYMENT_PENDING": {"PAID", "CANCELED"},
	"PAID":            {"SHIPPED"},
	"SHIPPED":         {"COMPLETED"},
}

type OrderService struct {
	orderRepo   *repository.OrderRepo
	productRepo *repository.ProductRepo
	promoRepo   *repository.PromoRepo
	userOpRepo  *repository.UserOpRepo
	cfg         *config.Config
}

func NewOrderService(
	orderRepo *repository.OrderRepo,
	productRepo *repository.ProductRepo,
	promoRepo *repository.PromoRepo,
	userOpRepo *repository.UserOpRepo,
	cfg *config.Config,
) *OrderService {
	return &OrderService{
		orderRepo:   orderRepo,
		productRepo: productRepo,
		promoRepo:   promoRepo,
		userOpRepo:  userOpRepo,
		cfg:         cfg,
	}
}

type CreateOrderInput struct {
	UserID    int64
	Items     []OrderItemInput
	PromoCode *string
}

type OrderItemInput struct {
	ProductID int64
	Quantity  int
}

type OrderResult struct {
	Order *repository.Order
	Items []repository.OrderItem
	Promo *string
}

func (s *OrderService) Create(ctx context.Context, input CreateOrderInput) (*OrderResult, error) {
	tx, err := s.orderRepo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Rate limit
	if err := s.checkRateLimit(ctx, tx, input.UserID, "CREATE_ORDER"); err != nil {
		return nil, err
	}

	// 2. Active order check
	hasActive, err := s.orderRepo.HasActiveOrder(ctx, tx, input.UserID)
	if err != nil {
		return nil, err
	}
	if hasActive {
		return nil, apperror.OrderHasActive()
	}

	// 3-4. Validate products and stock
	products, err := s.validateAndLockProducts(ctx, tx, input.Items)
	if err != nil {
		return nil, err
	}

	// 5. Reserve stock
	for _, item := range input.Items {
		if err := s.productRepo.UpdateStockTx(ctx, tx, item.ProductID, -item.Quantity); err != nil {
			return nil, err
		}
	}

	// 6. Snapshot prices & calc total
	var totalAmount float64
	orderItems := make([]repository.OrderItem, len(input.Items))
	for i, item := range input.Items {
		price := products[item.ProductID].Price
		orderItems[i] = repository.OrderItem{
			ProductID:    item.ProductID,
			Quantity:     item.Quantity,
			PriceAtOrder: price,
		}
		totalAmount += price * float64(item.Quantity)
	}

	// 7. Promo code
	var promoCodeID *int64
	var discountAmount float64
	var promoCodeStr *string
	if input.PromoCode != nil && *input.PromoCode != "" {
		promo, discount, err := s.applyPromoCode(ctx, tx, *input.PromoCode, totalAmount)
		if err != nil {
			return nil, err
		}
		promoCodeID = &promo.ID
		discountAmount = discount
		totalAmount -= discount
		promoCodeStr = &promo.Code
	}

	totalAmount = math.Round(totalAmount*100) / 100
	discountAmount = math.Round(discountAmount*100) / 100

	order := &repository.Order{
		UserID:         input.UserID,
		Status:         "CREATED",
		PromoCodeID:    promoCodeID,
		TotalAmount:    totalAmount,
		DiscountAmount: discountAmount,
	}
	if err := s.orderRepo.CreateTx(ctx, tx, order); err != nil {
		return nil, err
	}

	for i := range orderItems {
		orderItems[i].OrderID = order.ID
	}
	if err := s.orderRepo.CreateItemsTx(ctx, tx, orderItems); err != nil {
		return nil, err
	}

	// 8. Record operation
	if err := s.userOpRepo.CreateTx(ctx, tx, input.UserID, "CREATE_ORDER"); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &OrderResult{Order: order, Items: orderItems, Promo: promoCodeStr}, nil
}

func (s *OrderService) Update(ctx context.Context, orderID, userID int64, isAdmin bool, items []OrderItemInput) (*OrderResult, error) {
	tx, err := s.orderRepo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	order, err := s.orderRepo.GetByIDForUpdate(ctx, tx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, apperror.OrderNotFound(orderID)
	}

	// 1. Ownership
	if !isAdmin && order.UserID != userID {
		return nil, apperror.OrderOwnershipViolation()
	}

	// 2. State check
	if order.Status != "CREATED" {
		return nil, apperror.InvalidStateTransition(order.Status, "update")
	}

	// 3. Rate limit
	if err := s.checkRateLimit(ctx, tx, order.UserID, "UPDATE_ORDER"); err != nil {
		return nil, err
	}

	// 4. Return previous stock
	oldItems, err := s.orderRepo.GetItemsByOrderIDTx(ctx, tx, orderID)
	if err != nil {
		return nil, err
	}
	for _, item := range oldItems {
		if err := s.productRepo.UpdateStockTx(ctx, tx, item.ProductID, item.Quantity); err != nil {
			return nil, err
		}
	}

	// 5. Validate & reserve new items
	products, err := s.validateAndLockProducts(ctx, tx, items)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := s.productRepo.UpdateStockTx(ctx, tx, item.ProductID, -item.Quantity); err != nil {
			return nil, err
		}
	}

	// Delete old items, create new
	if err := s.orderRepo.DeleteItemsTx(ctx, tx, orderID); err != nil {
		return nil, err
	}

	var totalAmount float64
	orderItems := make([]repository.OrderItem, len(items))
	for i, item := range items {
		price := products[item.ProductID].Price
		orderItems[i] = repository.OrderItem{
			OrderID:      orderID,
			ProductID:    item.ProductID,
			Quantity:     item.Quantity,
			PriceAtOrder: price,
		}
		totalAmount += price * float64(item.Quantity)
	}
	if err := s.orderRepo.CreateItemsTx(ctx, tx, orderItems); err != nil {
		return nil, err
	}

	// 6. Re-check promo
	var promoCodeStr *string
	var discountAmount float64
	if order.PromoCodeID != nil {
		promo, err := s.promoRepo.GetByIDForUpdateTx(ctx, tx, *order.PromoCodeID)
		if err != nil {
			return nil, err
		}
		if promo != nil && totalAmount >= promo.MinOrderAmount {
			discount := s.calculateDiscount(promo, totalAmount)
			discountAmount = discount
			totalAmount -= discount
			promoCodeStr = &promo.Code
		} else {
			// Promo no longer applies — remove it and decrement uses
			if promo != nil {
				if err := s.promoRepo.DecrementUsesTx(ctx, tx, promo.ID); err != nil {
					return nil, err
				}
			}
			order.PromoCodeID = nil
		}
	}

	totalAmount = math.Round(totalAmount*100) / 100
	discountAmount = math.Round(discountAmount*100) / 100

	order.TotalAmount = totalAmount
	order.DiscountAmount = discountAmount
	if err := s.orderRepo.UpdateTx(ctx, tx, order); err != nil {
		return nil, err
	}

	// 7. Record operation
	if err := s.userOpRepo.CreateTx(ctx, tx, order.UserID, "UPDATE_ORDER"); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &OrderResult{Order: order, Items: orderItems, Promo: promoCodeStr}, nil
}

func (s *OrderService) Cancel(ctx context.Context, orderID, userID int64, isAdmin bool) (*OrderResult, error) {
	tx, err := s.orderRepo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	order, err := s.orderRepo.GetByIDForUpdate(ctx, tx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, apperror.OrderNotFound(orderID)
	}

	if !isAdmin && order.UserID != userID {
		return nil, apperror.OrderOwnershipViolation()
	}

	if order.Status != "CREATED" && order.Status != "PAYMENT_PENDING" {
		return nil, apperror.InvalidStateTransition(order.Status, "CANCELED")
	}

	// Return stock
	items, err := s.orderRepo.GetItemsByOrderIDTx(ctx, tx, orderID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := s.productRepo.UpdateStockTx(ctx, tx, item.ProductID, item.Quantity); err != nil {
			return nil, err
		}
	}

	// Return promo usage
	if order.PromoCodeID != nil {
		if err := s.promoRepo.DecrementUsesTx(ctx, tx, *order.PromoCodeID); err != nil {
			return nil, err
		}
	}

	order.Status = "CANCELED"
	if err := s.orderRepo.UpdateTx(ctx, tx, order); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &OrderResult{Order: order, Items: items}, nil
}

func (s *OrderService) UpdateStatus(ctx context.Context, orderID int64, newStatus string) (*OrderResult, error) {
	tx, err := s.orderRepo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	order, err := s.orderRepo.GetByIDForUpdate(ctx, tx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, apperror.OrderNotFound(orderID)
	}

	if !isValidTransition(order.Status, newStatus) {
		return nil, apperror.InvalidStateTransition(order.Status, newStatus)
	}

	order.Status = newStatus
	if err := s.orderRepo.UpdateTx(ctx, tx, order); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	items, _ := s.orderRepo.GetItemsByOrderID(ctx, orderID)
	return &OrderResult{Order: order, Items: items}, nil
}

func (s *OrderService) GetByID(ctx context.Context, orderID int64) (*OrderResult, error) {
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, apperror.OrderNotFound(orderID)
	}
	items, err := s.orderRepo.GetItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	return &OrderResult{Order: order, Items: items}, nil
}

// ── helpers ──────────────────────────────────────────────

func (s *OrderService) checkRateLimit(ctx context.Context, tx pgx.Tx, userID int64, opType string) error {
	lastOp, err := s.userOpRepo.GetLastOperation(ctx, tx, userID, opType)
	if err != nil {
		return err
	}
	if lastOp != nil {
		elapsed := time.Since(lastOp.CreatedAt)
		if elapsed < time.Duration(s.cfg.OrderRateLimitMinutes)*time.Minute {
			return apperror.OrderLimitExceeded()
		}
	}
	return nil
}

func (s *OrderService) validateAndLockProducts(ctx context.Context, tx pgx.Tx, items []OrderItemInput) (map[int64]*repository.Product, error) {
	products := make(map[int64]*repository.Product, len(items))
	var stockIssues []map[string]interface{}

	for _, item := range items {
		p, err := s.productRepo.GetByIDForUpdate(ctx, tx, item.ProductID)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, apperror.ProductNotFound(item.ProductID)
		}
		if p.Status != "ACTIVE" {
			return nil, apperror.ProductInactive(item.ProductID)
		}
		if p.Stock < item.Quantity {
			stockIssues = append(stockIssues, map[string]interface{}{
				"product_id": item.ProductID,
				"requested":  item.Quantity,
				"available":  p.Stock,
			})
		}
		products[item.ProductID] = p
	}

	if len(stockIssues) > 0 {
		return nil, apperror.InsufficientStock(stockIssues)
	}

	return products, nil
}

func (s *OrderService) applyPromoCode(ctx context.Context, tx pgx.Tx, code string, totalAmount float64) (*repository.PromoCode, float64, error) {
	promo, err := s.promoRepo.GetByCodeForUpdateTx(ctx, tx, code)
	if err != nil {
		return nil, 0, err
	}
	if promo == nil {
		return nil, 0, apperror.PromoCodeInvalid("Promo code not found")
	}

	now := time.Now()
	if !promo.Active {
		return nil, 0, apperror.PromoCodeInvalid("Promo code is not active")
	}
	if promo.CurrentUses >= promo.MaxUses {
		return nil, 0, apperror.PromoCodeInvalid("Promo code usage limit reached")
	}
	if now.Before(promo.ValidFrom) || now.After(promo.ValidUntil) {
		return nil, 0, apperror.PromoCodeInvalid("Promo code has expired or is not yet valid")
	}

	if totalAmount < promo.MinOrderAmount {
		return nil, 0, apperror.PromoCodeMinAmount(promo.MinOrderAmount, totalAmount)
	}

	discount := s.calculateDiscount(promo, totalAmount)

	if err := s.promoRepo.IncrementUsesTx(ctx, tx, promo.ID); err != nil {
		return nil, 0, err
	}

	return promo, discount, nil
}

func (s *OrderService) calculateDiscount(promo *repository.PromoCode, totalAmount float64) float64 {
	var discount float64
	switch promo.DiscountType {
	case "PERCENTAGE":
		discount = totalAmount * promo.DiscountValue / 100
		maxDiscount := totalAmount * 0.7
		if discount > maxDiscount {
			discount = totalAmount
		}
	case "FIXED_AMOUNT":
		discount = promo.DiscountValue
		if discount > totalAmount {
			discount = totalAmount
		}
	}
	return math.Round(discount*100) / 100
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
