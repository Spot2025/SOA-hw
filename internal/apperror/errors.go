package apperror

import "net/http"

type AppError struct {
	HTTPStatus int                    `json:"-"`
	ErrorCode  string                 `json:"error_code"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

func ProductNotFound(id int64) *AppError {
	return &AppError{HTTPStatus: http.StatusNotFound, ErrorCode: "PRODUCT_NOT_FOUND", Message: "Product not found", Details: map[string]interface{}{"product_id": id}}
}

func ProductInactive(id int64) *AppError {
	return &AppError{HTTPStatus: http.StatusConflict, ErrorCode: "PRODUCT_INACTIVE", Message: "Product is not active", Details: map[string]interface{}{"product_id": id}}
}

func OrderNotFound(id int64) *AppError {
	return &AppError{HTTPStatus: http.StatusNotFound, ErrorCode: "ORDER_NOT_FOUND", Message: "Order not found", Details: map[string]interface{}{"order_id": id}}
}

func OrderLimitExceeded() *AppError {
	return &AppError{HTTPStatus: http.StatusTooManyRequests, ErrorCode: "ORDER_LIMIT_EXCEEDED", Message: "Rate limit for order operations exceeded"}
}

func OrderHasActive() *AppError {
	return &AppError{HTTPStatus: http.StatusConflict, ErrorCode: "ORDER_HAS_ACTIVE", Message: "User already has an active order"}
}

func InvalidStateTransition(from, to string) *AppError {
	return &AppError{HTTPStatus: http.StatusConflict, ErrorCode: "INVALID_STATE_TRANSITION", Message: "Invalid state transition", Details: map[string]interface{}{"from": from, "to": to}}
}

func InsufficientStock(details []map[string]interface{}) *AppError {
	return &AppError{HTTPStatus: http.StatusConflict, ErrorCode: "INSUFFICIENT_STOCK", Message: "Insufficient stock", Details: map[string]interface{}{"items": details}}
}

func PromoCodeInvalid(reason string) *AppError {
	return &AppError{HTTPStatus: http.StatusUnprocessableEntity, ErrorCode: "PROMO_CODE_INVALID", Message: reason}
}

func PromoCodeMinAmount(min, actual float64) *AppError {
	return &AppError{HTTPStatus: http.StatusUnprocessableEntity, ErrorCode: "PROMO_CODE_MIN_AMOUNT", Message: "Order amount below promo code minimum", Details: map[string]interface{}{"min_order_amount": min, "actual_amount": actual}}
}

func OrderOwnershipViolation() *AppError {
	return &AppError{HTTPStatus: http.StatusForbidden, ErrorCode: "ORDER_OWNERSHIP_VIOLATION", Message: "Order belongs to another user"}
}

func ValidationError(details map[string]interface{}) *AppError {
	return &AppError{HTTPStatus: http.StatusBadRequest, ErrorCode: "VALIDATION_ERROR", Message: "Validation error", Details: details}
}

func AccessDenied() *AppError {
	return &AppError{HTTPStatus: http.StatusForbidden, ErrorCode: "ACCESS_DENIED", Message: "Insufficient permissions"}
}

func TokenExpired() *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, ErrorCode: "TOKEN_EXPIRED", Message: "Access token has expired"}
}

func TokenInvalid() *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, ErrorCode: "TOKEN_INVALID", Message: "Invalid access token"}
}

func RefreshTokenInvalid() *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, ErrorCode: "REFRESH_TOKEN_INVALID", Message: "Invalid refresh token"}
}

func UsernameConflict() *AppError {
	return &AppError{HTTPStatus: http.StatusConflict, ErrorCode: "USERNAME_CONFLICT", Message: "Username already taken"}
}

func InvalidCredentials() *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, ErrorCode: "INVALID_CREDENTIALS", Message: "Invalid username or password"}
}
