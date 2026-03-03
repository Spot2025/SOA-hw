package handler

import (
	"net/http"

	api "marketplace/internal/api"
	"marketplace/internal/apperror"
	"marketplace/internal/repository"
	"marketplace/internal/service"
)

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	role := getUserRole(r)
	if role != "USER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	var req api.OrderCreate
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	items := make([]service.OrderItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.OrderItemInput{ProductID: it.ProductId, Quantity: it.Quantity}
	}

	input := service.CreateOrderInput{
		UserID:    getUserID(r),
		Items:     items,
		PromoCode: req.PromoCode,
	}

	result, err := h.orders.Create(r.Context(), input)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toOrderResponse(result))
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request, id int64) {
	role := getUserRole(r)
	if role != "USER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	result, err := h.orders.GetByID(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}

	if role != "ADMIN" && result.Order.UserID != getUserID(r) {
		handleError(w, apperror.OrderOwnershipViolation())
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(result))
}

func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request, id int64) {
	role := getUserRole(r)
	if role != "USER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	var req api.OrderUpdate
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	items := make([]service.OrderItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.OrderItemInput{ProductID: it.ProductId, Quantity: it.Quantity}
	}

	result, err := h.orders.Update(r.Context(), id, getUserID(r), isAdmin(r), items)
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(result))
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request, id int64) {
	role := getUserRole(r)
	if role != "USER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	result, err := h.orders.Cancel(r.Context(), id, getUserID(r), isAdmin(r))
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(result))
}

func (h *Handler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request, id int64) {
	var req api.OrderStatusUpdate
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	result, err := h.orders.UpdateStatus(r.Context(), id, string(req.Status))
	if err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(result))
}

func toOrderResponse(res *service.OrderResult) api.OrderResponse {
	o := res.Order
	status := api.OrderStatus(o.Status)
	createdAt := o.CreatedAt
	updatedAt := o.UpdatedAt

	items := make([]api.OrderItemResponse, len(res.Items))
	for i, it := range res.Items {
		items[i] = toOrderItemResponse(&it)
	}

	return api.OrderResponse{
		Id:             &o.ID,
		UserId:         &o.UserID,
		Status:         &status,
		Items:          &items,
		PromoCode:      res.Promo,
		TotalAmount:    &o.TotalAmount,
		DiscountAmount: &o.DiscountAmount,
		CreatedAt:      &createdAt,
		UpdatedAt:      &updatedAt,
	}
}

func toOrderItemResponse(it *repository.OrderItem) api.OrderItemResponse {
	return api.OrderItemResponse{
		Id:           &it.ID,
		ProductId:    &it.ProductID,
		Quantity:     &it.Quantity,
		PriceAtOrder: &it.PriceAtOrder,
	}
}
