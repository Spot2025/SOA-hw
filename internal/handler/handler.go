package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"

	"marketplace/internal/apperror"
	"marketplace/internal/middleware"
	"marketplace/internal/service"
)

type Handler struct {
	products *service.ProductService
	orders   *service.OrderService
	auth     *service.AuthService
	promos   *service.PromoService
	log      zerolog.Logger
}

func New(
	products *service.ProductService,
	orders *service.OrderService,
	auth *service.AuthService,
	promos *service.PromoService,
	log zerolog.Logger,
) *Handler {
	return &Handler{
		products: products,
		orders:   orders,
		auth:     auth,
		promos:   promos,
		log:      log,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*apperror.AppError); ok {
		writeJSON(w, appErr.HTTPStatus, appErr)
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error_code": "INTERNAL_ERROR",
		"message":    "Internal server error",
	})
}

func decodeBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func getUserID(r *http.Request) int64 {
	return middleware.GetUserID(r.Context())
}

func getUserRole(r *http.Request) string {
	return middleware.GetUserRole(r.Context())
}

func isAdmin(r *http.Request) bool {
	return getUserRole(r) == "ADMIN"
}
