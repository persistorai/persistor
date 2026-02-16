package api

import (
	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/httputil"
	"github.com/persistorai/persistor/internal/metrics"
)

// Error code constants for standardized API responses.
const (
	ErrCodeInvalidRequest  = "invalid_request"
	ErrCodeNotFound        = "not_found"
	ErrCodeInternalError   = "internal_error"
	ErrCodeUnauthorized    = "unauthorized"
	ErrCodeRateLimited     = "rate_limited"
	ErrCodeValidationError = "validation_error"
)

// respondError writes a standardized JSON error response, pulling the request
// ID from the Gin context (set by the request ID middleware).
func respondError(c *gin.Context, status int, code, message string) {
	metrics.ErrorsTotal.WithLabelValues(code).Inc()
	httputil.RespondError(c, status, code, message)
}
