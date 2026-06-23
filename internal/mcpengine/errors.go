package mcpengine

import (
	"errors"
	"fmt"

	"github.com/persistorai/persistor/internal/index"
)

// ErrInternal is the generic error surfaced to a client when a tool operation
// fails for an internal reason (a store/database error). The underlying detail
// is logged server-side and deliberately NOT sent to the caller, so Postgres
// schema/SQL internals never leak across the MCP tool boundary.
var ErrInternal = errors.New("internal error")

// isClientSafe reports whether err carries a message meant for the caller: a
// rate-limit/read-only refusal or a domain conflict (self-supersede, dangling
// supersede target, version conflict, note-not-found). Anything else reaching a
// store boundary is an internal error whose detail must not cross to the client.
//
// Input-validation errors are intentionally absent: the engine returns those
// before any store call, so they never pass through opError.
func isClientSafe(err error) bool {
	if errors.Is(err, ErrReadOnly) || errors.Is(err, ErrRateLimited) {
		return true
	}
	var (
		selfSupersede    *index.SelfSupersedeError
		supersedeMissing *index.SupersedesMissingError
		versionConflict  *index.VersionConflictError
		noteNotFound     *index.NoteNotFoundError
	)
	return errors.As(err, &selfSupersede) ||
		errors.As(err, &supersedeMissing) ||
		errors.As(err, &versionConflict) ||
		errors.As(err, &noteNotFound)
}

// opError sanitizes an error returned from a store call before it reaches the
// MCP client. A client-safe error passes through (wrapped with op for context);
// any other error is logged server-side with the tenant and op, then replaced
// with a generic ErrInternal so database/SQL internals never reach the caller.
func (e *Engine) opError(op string, err error) error {
	if err == nil {
		return nil
	}
	if isClientSafe(err) {
		return fmt.Errorf("%s: %w", op, err)
	}
	if e.log != nil {
		e.log.WithField("tenant", e.tenantID).WithField("op", op).WithError(err).
			Error("memory tool internal error")
	}
	return fmt.Errorf("%s: %w", op, ErrInternal)
}
