// Package httputil provides shared HTTP response helpers.
package httputil

import "github.com/gin-gonic/gin"

// RespondError writes a standardized JSON error response and aborts the request.
func RespondError(c *gin.Context, status int, code, message string) {
	var requestID string
	if rid, exists := c.Get("request_id"); exists {
		if s, ok := rid.(string); ok {
			requestID = s
		}
	}

	resp := map[string]string{
		"code":    code,
		"message": message,
	}

	if requestID != "" {
		resp["request_id"] = requestID
	}

	c.AbortWithStatusJSON(status, resp)
}
