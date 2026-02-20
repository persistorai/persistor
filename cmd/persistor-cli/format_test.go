package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout replaces os.Stdout with a pipe, calls f, then returns the
// captured output and restores os.Stdout. It is NOT safe for parallel use
// because os.Stdout is a package-level variable.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	f()

	w.Close()
	<-done
	os.Stdout = orig
	r.Close()
	return buf.String()
}

// TestFormatJSON verifies that formatJSON emits indented JSON to stdout.
func TestFormatJSON(t *testing.T) {
	type sample struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	v := sample{ID: "abc-123", Label: "hello world"}

	got := captureStdout(t, func() { formatJSON(v) })

	// Must be valid JSON.
	var out sample
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, got)
	}
	if out.ID != "abc-123" {
		t.Errorf("id: got %q, want %q", out.ID, "abc-123")
	}
	if out.Label != "hello world" {
		t.Errorf("label: got %q, want %q", out.Label, "hello world")
	}
	// Must be indented (contains newlines and spaces).
	if !strings.Contains(got, "\n") {
		t.Errorf("expected indented JSON but got: %s", got)
	}
}

// TestFormatJSONArray verifies an array value is valid JSON.
func TestFormatJSONArray(t *testing.T) {
	items := []map[string]string{{"a": "1"}, {"b": "2"}}
	got := captureStdout(t, func() { formatJSON(items) })

	var out []map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("output is not valid JSON array: %v\noutput: %s", err, got)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 items, got %d", len(out))
	}
}

// TestFormatTable verifies column alignment and separator row.
func TestFormatTable(t *testing.T) {
	headers := []string{"ID", "TYPE", "LABEL"}
	rows := [][]string{
		{"abc-123", "person", "Alice"},
		{"x", "org", "Acme Corporation"},
	}

	got := captureStdout(t, func() { formatTable(headers, rows) })
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	// Expect: header, separator, row, row → 4 lines.
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), got)
	}

	// Header line must contain all header names.
	for _, h := range headers {
		if !strings.Contains(lines[0], h) {
			t.Errorf("header line missing %q: %s", h, lines[0])
		}
	}

	// Separator line must contain only dashes and spaces.
	sep := strings.TrimSpace(lines[1])
	for _, ch := range sep {
		if ch != '-' && ch != ' ' {
			t.Errorf("separator contains unexpected char %q: %s", ch, lines[1])
		}
	}

	// Data rows must contain cell values.
	if !strings.Contains(lines[2], "abc-123") {
		t.Errorf("row 0 missing id: %s", lines[2])
	}
	if !strings.Contains(lines[3], "Acme Corporation") {
		t.Errorf("row 1 missing label: %s", lines[3])
	}
}

// TestFormatTableEmpty verifies that an empty row set still prints headers.
func TestFormatTableEmpty(t *testing.T) {
	headers := []string{"ID", "LABEL"}
	got := captureStdout(t, func() { formatTable(headers, nil) })
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + separator), got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "ID") {
		t.Errorf("header missing: %s", lines[0])
	}
}

// TestFormatTableWidthPadding verifies that columns are padded to the widest
// cell so values align.
func TestFormatTableWidthPadding(t *testing.T) {
	headers := []string{"ID"}
	rows := [][]string{
		{"short"},
		{"a-much-longer-value"},
	}
	got := captureStdout(t, func() { formatTable(headers, rows) })
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// All data lines should be the same length (padded to max width).
	// Line 0 = header, line 1 = separator, lines 2+ = rows.
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}
	if len(lines[2]) != len(lines[3]) {
		t.Errorf("row widths differ: %d vs %d\n%s\n%s", len(lines[2]), len(lines[3]), lines[2], lines[3])
	}
}

// TestOutputJSON verifies output() uses JSON when flagFmt is "json".
func TestOutputJSON(t *testing.T) {
	flagFmt = "json"
	v := map[string]string{"key": "val"}
	got := captureStdout(t, func() { output(v, "quiet-id") })

	var out map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("expected JSON output: %v\noutput: %s", err, got)
	}
	if out["key"] != "val" {
		t.Errorf("got %q, want %q", out["key"], "val")
	}
}

// TestOutputQuiet verifies output() prints the quiet value when flagFmt is "quiet".
func TestOutputQuiet(t *testing.T) {
	flagFmt = "quiet"
	v := map[string]string{"key": "val"}
	got := captureStdout(t, func() { output(v, "my-quiet-id") })
	got = strings.TrimRight(got, "\n")
	if got != "my-quiet-id" {
		t.Errorf("got %q, want %q", got, "my-quiet-id")
	}
}

// TestOutputTableFallback verifies output() falls back to JSON for "table"
// when the caller hasn't handled table rendering itself.
func TestOutputTableFallback(t *testing.T) {
	flagFmt = "table"
	v := map[string]string{"x": "y"}
	got := captureStdout(t, func() { output(v, "") })

	var out map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("expected JSON fallback for table format: %v\noutput: %s", err, got)
	}
}

// TestVersionString verifies the dev build string when commit/buildDate are empty.
func TestVersionString(t *testing.T) {
	origCommit, origDate := commit, buildDate
	commit, buildDate = "", ""
	defer func() { commit, buildDate = origCommit, origDate }()

	s := versionString()
	if !strings.HasSuffix(s, "-dev") {
		t.Errorf("expected -dev suffix for dev build, got %q", s)
	}
	if !strings.Contains(s, version) {
		t.Errorf("version string missing version %q: %s", version, s)
	}
}

// TestVersionStringRelease verifies the full build string when commit and
// buildDate are set.
func TestVersionStringRelease(t *testing.T) {
	origCommit, origDate := commit, buildDate
	commit, buildDate = "abc1234", "2026-01-01"
	defer func() { commit, buildDate = origCommit, origDate }()

	s := versionString()
	if !strings.Contains(s, "abc1234") {
		t.Errorf("expected commit hash in version string, got %q", s)
	}
	if !strings.Contains(s, "2026-01-01") {
		t.Errorf("expected build date in version string, got %q", s)
	}
	if strings.HasSuffix(s, "-dev") {
		t.Errorf("release build should not have -dev suffix, got %q", s)
	}
}
