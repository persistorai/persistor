package mcpengine

import (
	"testing"
	"time"
)

// TestParseTimeBound locks the since/until argument grammar: RFC3339, bare
// dates (until = end of day), empty = unbounded, garbage rejected.
func TestParseTimeBound(t *testing.T) {
	if got, err := parseTimeBound("since", "", false); err != nil || got != nil {
		t.Errorf("empty = (%v, %v), want (nil, nil)", got, err)
	}

	rfc, err := parseTimeBound("since", "2026-07-01T12:30:00Z", false)
	if err != nil || rfc == nil || !rfc.Equal(time.Date(2026, 7, 1, 12, 30, 0, 0, time.UTC)) {
		t.Errorf("rfc3339 = (%v, %v)", rfc, err)
	}

	day, err := parseTimeBound("since", "2026-07-01", false)
	if err != nil || day == nil || !day.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bare date start-of-day = (%v, %v)", day, err)
	}

	end, err := parseTimeBound("until", "2026-07-01", true)
	if err != nil || end == nil || !end.After(time.Date(2026, 7, 1, 23, 59, 59, 0, time.UTC)) ||
		!end.Before(time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bare date end-of-day = (%v, %v), want inside the final second of Jul 1", end, err)
	}

	if _, err := parseTimeBound("since", "last tuesday", false); err == nil {
		t.Error("garbage accepted; want an error telling the model the expected formats")
	}
}
