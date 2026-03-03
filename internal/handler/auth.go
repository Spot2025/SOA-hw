package handler

import (
	"net/http"

	api "marketplace/internal/api"
)

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req api.RegisterRequest
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	tokens, err := h.auth.Register(r.Context(), req.Username, req.Password, string(req.Role))
	if err != nil {
		handleError(w, err)
		return
	}

	role := api.UserRole(tokens.Role)
	writeJSON(w, http.StatusCreated, api.AuthResponse{
		AccessToken:  &tokens.AccessToken,
		RefreshToken: &tokens.RefreshToken,
		UserId:       &tokens.UserID,
		Role:         &role,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req api.LoginRequest
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	tokens, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		handleError(w, err)
		return
	}

	role := api.UserRole(tokens.Role)
	writeJSON(w, http.StatusOK, api.AuthResponse{
		AccessToken:  &tokens.AccessToken,
		RefreshToken: &tokens.RefreshToken,
		UserId:       &tokens.UserID,
		Role:         &role,
	})
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req api.RefreshRequest
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "VALIDATION_ERROR", "message": "Invalid request body"})
		return
	}

	tokens, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		handleError(w, err)
		return
	}

	role := api.UserRole(tokens.Role)
	writeJSON(w, http.StatusOK, api.AuthResponse{
		AccessToken:  &tokens.AccessToken,
		RefreshToken: &tokens.RefreshToken,
		UserId:       &tokens.UserID,
		Role:         &role,
	})
}
