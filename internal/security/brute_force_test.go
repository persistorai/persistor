package security_test

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/security"
)

func newTestGuard() (*security.BruteForceGuard, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	return security.NewBruteForceGuard(ctx, log), cancel
}

func TestBruteForce_SuccessfulAuthResetsCount(t *testing.T) {
	guard, cancel := newTestGuard()
	defer cancel()

	guard.RecordFailure("key1")
	guard.RecordFailure("key1")
	guard.ResetKey("key1")

	if guard.IsBlocked("key1") {
		t.Fatal("key should not be blocked after reset")
	}
}

func TestBruteForce_FailureIncrementsAndBlocks(t *testing.T) {
	guard, cancel := newTestGuard()
	defer cancel()

	for range 5 {
		guard.RecordFailure("badkey")
	}

	if !guard.IsBlocked("badkey") {
		t.Fatal("key should be blocked after max failures")
	}
}

func TestBruteForce_NotBlockedBeforeMax(t *testing.T) {
	guard, cancel := newTestGuard()
	defer cancel()

	for range 4 {
		guard.RecordFailure("almostbad")
	}

	if guard.IsBlocked("almostbad") {
		t.Fatal("key should not be blocked before max failures")
	}
}
