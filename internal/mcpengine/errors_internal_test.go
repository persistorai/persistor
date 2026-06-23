package mcpengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestOpErrorGenericizesInternal: an arbitrary store/internal error is replaced
// with ErrInternal and its detail (e.g. a leaked column/constraint name) does
// not appear in the client-facing message.
func TestOpErrorGenericizesInternal(t *testing.T) {
	e := &Engine{tenantID: "t1"} // nil logger: sanitization still applies
	leaky := errors.New("pq: column \"secret_internal_col\" does not exist")
	got := e.opError("search", leaky)

	if !errors.Is(got, ErrInternal) {
		t.Fatalf("opError did not wrap ErrInternal: %v", got)
	}
	if strings.Contains(got.Error(), "secret_internal_col") {
		t.Fatalf("internal detail leaked to client message: %q", got.Error())
	}
	if got.Error() != "search: internal error" {
		t.Fatalf("message = %q, want %q", got.Error(), "search: internal error")
	}
}

// TestOpErrorPreservesDomain: client-actionable domain errors pass through
// (still matchable with errors.As, message intact) and are NOT genericized.
func TestOpErrorPreservesDomain(t *testing.T) {
	e := &Engine{tenantID: "t1"}

	domainErrs := []error{
		&index.VersionConflictError{NoteID: "n1", Expected: 2, Actual: 3},
		&index.NoteNotFoundError{NoteID: "n1"},
		&index.SupersedesMissingError{NoteID: "n1", Target: "n2"},
		&index.SelfSupersedeError{ID: "n1"},
	}
	for _, de := range domainErrs {
		got := e.opError("write", de)
		if errors.Is(got, ErrInternal) {
			t.Errorf("domain error genericized: %v", got)
		}
		if !errors.Is(got, de) {
			t.Errorf("domain error not preserved through opError: %v", got)
		}
	}

	// Engine sentinels are client-safe too.
	for _, se := range []error{ErrReadOnly, ErrRateLimited} {
		if got := e.opError("write", se); errors.Is(got, ErrInternal) {
			t.Errorf("sentinel %v genericized: %v", se, got)
		}
	}

	// A nil error stays nil.
	if got := e.opError("write", nil); got != nil {
		t.Errorf("opError(nil) = %v, want nil", got)
	}
}
