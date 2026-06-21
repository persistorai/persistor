package main

import (
	"context"
	"strings"
	"testing"
)

// TestRunPurgeValidation checks the guard rails that run before any DB
// connection: a note id and a valid tenant are required, and the irreversible
// purge is refused without --yes.
func TestRunPurgeValidation(t *testing.T) {
	const validTenant = "c6dc079c-735b-5be1-a2c7-bf1674624697"
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    purgeOpts
		wantSub string
	}{
		{name: "no tenant", opts: purgeOpts{noteID: "x"}, wantSub: "no tenant"},
		{name: "invalid tenant uuid", opts: purgeOpts{noteID: "x", tenantID: "nope"}, wantSub: "invalid tenant"},
		{name: "refuses without --yes", opts: purgeOpts{noteID: "x", tenantID: validTenant}, wantSub: "without --yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runPurge(ctx, &tt.opts)
			if err == nil {
				t.Fatalf("runPurge(%+v) = nil, want error containing %q", tt.opts, tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("runPurge error = %q, want it to contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}
