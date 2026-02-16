package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/httputil"
)

// respondError delegates to the shared httputil.RespondError helper.
func respondError(c *gin.Context, code int, errCode, message string) {
	httputil.RespondError(c, code, errCode, message)
}
