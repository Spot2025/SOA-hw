package handler

import (
	"net/http"

	api "marketplace/internal/api"
	"marketplace/internal/apperror"
	"marketplace/internal/repository"
)

func (h *Handler) CreatePromoCode(w http.ResponseWriter, r *http.Request) {
	role := getUserRole(r)
	if role != "SELLER" && role != "ADMIN" {
		handleError(w, apperror.AccessDenied())
		return
	}

	var req api.PromoCodeCreate
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	pc := &repository.PromoCode{
		Code:           req.Code,
		DiscountType:   string(req.DiscountType),
		DiscountValue:  req.DiscountValue,
		MinOrderAmount: req.MinOrderAmount,
		MaxUses:        req.MaxUses,
		ValidFrom:      req.ValidFrom,
		ValidUntil:     req.ValidUntil,
		Active:         true,
	}

	if err := h.promos.Create(r.Context(), pc); err != nil {
		handleError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toPromoResponse(pc))
}

func toPromoResponse(p *repository.PromoCode) api.PromoCodeResponse {
	dt := api.DiscountType(p.DiscountType)
	return api.PromoCodeResponse{
		Id:             &p.ID,
		Code:           &p.Code,
		DiscountType:   &dt,
		DiscountValue:  &p.DiscountValue,
		MinOrderAmount: &p.MinOrderAmount,
		MaxUses:        &p.MaxUses,
		CurrentUses:    &p.CurrentUses,
		ValidFrom:      &p.ValidFrom,
		ValidUntil:     &p.ValidUntil,
		Active:         &p.Active,
	}
}
