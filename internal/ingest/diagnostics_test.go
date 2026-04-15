package ingest

import (
	"fmt"
	"testing"
	"time"

	"github.com/persistorai/persistor/client"
)

func TestDiagnosticsSnapshot_BuildsOperatorAlerts(t *testing.T) {
	d := newDiagnosticsCollector()
	for i := 0; i < 3; i++ {
		d.recordEntityResolution(resolutionAmbiguous)
	}
	for i := 0; i < 2; i++ {
		d.recordEntityResolution(resolutionNoMatch)
	}
	d.recordUnknownRelations(6)
	d.recordParseFailure()
	d.recordParseFailure()
	d.recordAPIFailure(&client.APIError{StatusCode: 422, Code: "bad_request", Message: "bad input"})
	d.recordAPIFailure(&client.APIError{StatusCode: 503, Code: "unavailable", Message: "try later"})
	d.recordChunkThroughput(1, 400*time.Millisecond, 12, 0, 0)
	d.recordChunkThroughput(2, 400*time.Millisecond, 11, 0, 0)
	d.recordChunkThroughput(3, 3*time.Second, 1, 0, 0)

	snap := d.snapshot()
	if snap.EntityResolutionAmbiguous != 3 || snap.EntityResolutionMisses != 2 {
		t.Fatalf("unexpected resolution counts: %+v", snap)
	}
	if snap.API4xxFailures != 1 || snap.API5xxFailures != 1 {
		t.Fatalf("unexpected api counts: %+v", snap)
	}
	if snap.ParseFailures != 2 || snap.UnknownRelationCount != 6 {
		t.Fatalf("unexpected parse/unknown counts: %+v", snap)
	}
	if len(snap.ThroughputDrops) != 1 {
		t.Fatalf("throughput drops = %d, want 1", len(snap.ThroughputDrops))
	}
	if len(snap.Alerts) == 0 {
		t.Fatal("expected diagnostic alerts")
	}
}

func TestExtractStatusCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{err: fmt.Errorf("LLM API returned status 429: nope"), want: 429},
		{err: fmt.Errorf("ollama returned status 503: nope"), want: 503},
		{err: fmt.Errorf("other error"), want: 0},
	}
	for _, tc := range cases {
		if got := extractStatusCode(tc.err); got != tc.want {
			t.Fatalf("extractStatusCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}
