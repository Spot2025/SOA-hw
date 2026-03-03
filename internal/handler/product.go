package handler

import (
	"net/http"

	api "marketplace/internal/api"
	"marketplace/internal/apperror"
	"marketplace/internal/repository"
)

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	role := getUserRole(r)
	if role != "SELLER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	var req api.ProductCreate
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	uid := getUserID(r)
	var sellerID *int64
	if role == "SELLER" {
		sellerID = &uid
	}

	p := &repository.Product{
		Name:     req.Name,
		Price:    req.Price,
		Stock:    req.Stock,
		Category: req.Category,
		Status:   string(req.Status),
		SellerID: sellerID,
	}
	if req.Description != nil {
		p.Description = req.Description
	}

	if err := h.products.Create(r.Context(), p); err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toProductResponse(p))
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request, id int64) {
	p, err := h.products.GetByID(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProductResponse(p))
}

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request, params api.ListProductsParams) {
	page := 0
	size := 20
	if params.Page != nil {
		page = *params.Page
	}
	if params.Size != nil {
		size = *params.Size
	}

	filter := repository.ProductFilter{
		Page: page,
		Size: size,
	}
	if params.Status != nil {
		s := string(*params.Status)
		filter.Status = &s
	}
	if params.Category != nil {
		filter.Category = params.Category
	}

	result, err := h.products.List(r.Context(), filter)
	if err != nil {
		handleError(w, err)
		return
	}

	items := make([]api.ProductResponse, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, toProductResponse(&result.Items[i]))
	}

	writeJSON(w, http.StatusOK, api.ProductListResponse{
		Items:         &items,
		TotalElements: &result.TotalElements,
		Page:          &page,
		Size:          &size,
	})
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request, id int64) {
	role := getUserRole(r)
	if role != "SELLER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	var req api.ProductUpdate
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	uid := getUserID(r)
	updated, err := h.products.Update(r.Context(), id, func(p *repository.Product) {
		if req.Name != nil {
			p.Name = *req.Name
		}
		if req.Description != nil {
			p.Description = req.Description
		}
		if req.Price != nil {
			p.Price = *req.Price
		}
		if req.Stock != nil {
			p.Stock = *req.Stock
		}
		if req.Category != nil {
			p.Category = *req.Category
		}
		if req.Status != nil {
			p.Status = string(*req.Status)
		}
	})
	if err != nil {
		handleError(w, err)
		return
	}

	if role == "SELLER" && (updated.SellerID == nil || *updated.SellerID != uid) {
		handleError(w, apperror.AccessDenied())
		return
	}

	writeJSON(w, http.StatusOK, toProductResponse(updated))
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request, id int64) {
	role := getUserRole(r)
	if role != "SELLER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	if role == "SELLER" {
		p, err := h.products.GetByID(r.Context(), id)
		if err != nil {
			handleError(w, err)
			return
		}
		uid := getUserID(r)
		if p.SellerID == nil || *p.SellerID != uid {
			handleError(w, apperror.AccessDenied())
			return
		}
	}

	if err := h.products.SoftDelete(r.Context(), id); err != nil {
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toProductResponse(p *repository.Product) api.ProductResponse {
	status := api.ProductStatus(p.Status)
	createdAt := p.CreatedAt
	updatedAt := p.UpdatedAt
	return api.ProductResponse{
		Id:          &p.ID,
		Name:        &p.Name,
		Description: p.Description,
		Price:       &p.Price,
		Stock:       &p.Stock,
		Category:    &p.Category,
		Status:      &status,
		SellerId:    p.SellerID,
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}
}
