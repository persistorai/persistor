package graphql

import (
	"context"
	"errors"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/persistorai/persistor/internal/models"
)

// GraphQL error code constants.
const (
	codeNotFound      = "NOT_FOUND"
	codeBadRequest    = "BAD_REQUEST"
	codeInternalError = "INTERNAL_ERROR"
)

// gqlErr maps a service/store error to a user-friendly GraphQL error with
// appropriate extension codes.  It never leaks internal details for unknown
// errors.
func gqlErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, models.ErrNodeNotFound),
		errors.Is(err, models.ErrEdgeNotFound):
		return gqlErrWithCode(ctx, err.Error(), codeNotFound)

	case errors.Is(err, models.ErrMissingID),
		errors.Is(err, models.ErrMissingType),
		errors.Is(err, models.ErrMissingLabel),
		errors.Is(err, models.ErrMissingSource),
		errors.Is(err, models.ErrMissingTarget),
		errors.Is(err, models.ErrMissingRelation),
		errors.Is(err, models.ErrDuplicateKey):
		return gqlErrWithCode(ctx, err.Error(), codeBadRequest)

	case strings.Contains(err.Error(), "exceeds maximum length"):
		return gqlErrWithCode(ctx, err.Error(), codeBadRequest)

	default:
		return gqlErrWithCode(ctx, "internal server error", codeInternalError)
	}
}

// gqlErrWithCode creates a GraphQL error with an extension code on the
// current field path.
func gqlErrWithCode(ctx context.Context, message, code string) error {
	return &gqlerror.Error{
		Message: message,
		Path:    graphql.GetPath(ctx),
		Extensions: map[string]interface{}{
			"code": code,
		},
	}
}
