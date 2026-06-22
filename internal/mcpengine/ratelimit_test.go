package mcpengine_test

import (
	"testing"

	"github.com/briancolinger/persistor/internal/mcpengine"
)

func TestKeyLimiter(t *testing.T) {
	// A disabled limiter (non-positive rate) and a nil limiter allow everything.
	if !mcpengine.NewKeyLimiter(0, 0).Allow("t") {
		t.Error("disabled limiter should allow")
	}
	var nilLimiter *mcpengine.KeyLimiter
	if !nilLimiter.Allow("t") {
		t.Error("nil limiter should allow")
	}

	// Burst 2: the first two events pass, the third is denied (refill is too slow
	// to matter within the test).
	l := mcpengine.NewKeyLimiter(0.001, 2)
	first, second := l.Allow("a"), l.Allow("a")
	if !first || !second {
		t.Fatal("burst of 2 should allow the first two writes")
	}
	if l.Allow("a") {
		t.Error("third write for tenant a should be denied")
	}
	// A different tenant has its own independent bucket.
	if !l.Allow("b") {
		t.Error("tenant b should have its own bucket")
	}
}
