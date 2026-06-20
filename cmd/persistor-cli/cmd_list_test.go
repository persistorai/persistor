package main

import (
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestDisplayID checks the table renderer flags superseded notes and leaves
// live ones untouched.
func TestDisplayID(t *testing.T) {
	tests := []struct {
		name string
		in   index.NoteSummary
		want string
	}{
		{name: "live note unadorned", in: index.NoteSummary{ID: "scout:big-jerry"}, want: "scout:big-jerry"},
		{name: "superseded flagged", in: index.NoteSummary{ID: "scout:old", Superseded: true}, want: "scout:old (superseded)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayID(&tt.in); got != tt.want {
				t.Fatalf("displayID = %q, want %q", got, tt.want)
			}
		})
	}
}
