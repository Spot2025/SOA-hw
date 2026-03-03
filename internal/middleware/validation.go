package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"marketplace/internal/api"
)

func NewValidator() (func(http.Handler) http.Handler, error) {
	swagger, err := api.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("failed to load swagger spec: %w", err)
	}
	swagger.Servers = openapi3.Servers{&openapi3.Server{URL: "/api/v1"}}

	return nethttpmiddleware.OapiRequestValidatorWithOptions(swagger, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandler: validationErrorHandler,
	}), nil
}

func validationErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	fields := parseValidationMessage(message)

	resp := map[string]interface{}{
		"error_code": "VALIDATION_ERROR",
		"message":    "Validation error",
		"details":    fields,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(resp)
}

func parseValidationMessage(msg string) map[string]interface{} {
	details := map[string]interface{}{}

	if strings.Contains(msg, "request body") || strings.Contains(msg, "value") {
		details["violations"] = splitViolations(msg)
	} else if strings.Contains(msg, "parameter") {
		details["violations"] = splitViolations(msg)
	} else {
		details["violations"] = []string{msg}
	}

	return details
}

func splitViolations(msg string) []string {
	parts := strings.Split(msg, " | ")
	if len(parts) == 1 {
		parts = strings.Split(msg, "\n")
	}
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

