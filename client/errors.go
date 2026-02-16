package client

import (
	"encoding/json"
	"fmt"
)

// APIError represents a structured error response from the Persistor API.
type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	RequestID  string `json:"request_id,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("persistor: %d %s: %s (request_id=%s)", e.StatusCode, e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("persistor: %d %s: %s", e.StatusCode, e.Code, e.Message)
}

// IsNotFound returns true if the error is a 404 not found.
func IsNotFound(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 404
	}
	return false
}

// IsConflict returns true if the error is a 409 conflict (duplicate key).
func IsConflict(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 409
	}
	return false
}

// IsRateLimited returns true if the error is a 429 rate limit.
func IsRateLimited(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 429
	}
	return false
}

// parseAPIError attempts to decode a JSON error body; falls back to raw text.
func parseAPIError(statusCode int, body []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode}
	if err := json.Unmarshal(body, apiErr); err != nil || apiErr.Code == "" {
		apiErr.Code = "unknown"
		apiErr.Message = string(body)
	}
	return apiErr
}
