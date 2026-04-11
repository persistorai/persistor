package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// AuthScopeContextKey stores the caller scope in Gin context.
const AuthScopeContextKey = "auth_scope"

// AuthScope defines the privilege level attached to an API key.
type AuthScope string

const (
	ScopeReadWrite AuthScope = "read_write"
	ScopeAdmin     AuthScope = "admin"
)

// AuthPrincipal is the authenticated identity derived from an API key.
type AuthPrincipal struct {
	TenantID string
	Scope    AuthScope
}

func (s AuthScope) allows(required AuthScope) bool {
	if required == ScopeReadWrite {
		return true
	}

	return s == ScopeAdmin
}

// RequireScope blocks requests whose authenticated API key lacks the required scope.
func RequireScope(required AuthScope, log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, _ := c.Get(AuthScopeContextKey)
		actual, _ := scope.(AuthScope)
		if actual == "" {
			actual = ScopeReadWrite
		}

		if actual.allows(required) {
			c.Next()
			return
		}

		log.WithFields(logrus.Fields{
			"path":       c.Request.URL.Path,
			"method":     c.Request.Method,
			"tenant_id":  c.GetString("tenant_id"),
			"auth_scope": actual,
			"required":   required,
		}).Warn("authorization failed: insufficient api key scope")

		respondError(c, http.StatusForbidden, "forbidden", "insufficient api key scope")
		c.Abort()
	}
}
