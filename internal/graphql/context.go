package graphql

import (
	"context"
	"errors"
)

type contextKey string

const tenantIDKey contextKey = "tenant_id"

// ErrNoTenant is returned when no tenant ID is found in the context.
var ErrNoTenant = errors.New("no tenant ID in context")

// WithTenantID stores the tenant ID in the context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// TenantIDFromContext extracts the tenant ID from the context.
func TenantIDFromContext(ctx context.Context) (string, error) {
	tid, ok := ctx.Value(tenantIDKey).(string)
	if !ok || tid == "" {
		return "", ErrNoTenant
	}
	return tid, nil
}
