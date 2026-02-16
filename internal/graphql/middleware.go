package graphql

import "github.com/gin-gonic/gin"

// GinContextToTenantMiddleware extracts the tenant_id set by auth middleware
// and stores it in the request context for GraphQL resolvers.
func GinContextToTenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetString("tenant_id")
		if tenantID != "" {
			ctx := WithTenantID(c.Request.Context(), tenantID)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
