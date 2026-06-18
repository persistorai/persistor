package harness_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireJQ skips when jq is unavailable (the hooks parse JSON with it). jq is
// present on the dev machine and in the hook runtime; CI without it just skips.
func requireJQ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH")
	}
}

// TestPreCompactNudge: the hook emits valid JSON with a top-level systemMessage
// and does NOT use hookSpecificOutput (rejected for the PreCompact event).
func TestPreCompactNudge(t *testing.T) {
	requireJQ(t)
	out := runHook(t, "hooks/pre-compact-nudge.sh", `{"hook_event_name":"PreCompact","trigger":"auto"}`, nil)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if _, ok := parsed["hookSpecificOutput"]; ok {
		t.Error("PreCompact hook must not use hookSpecificOutput (rejected by Claude Code)")
	}
	var sm string
	if err := json.Unmarshal(parsed["systemMessage"], &sm); err != nil || sm == "" {
		t.Errorf("missing/empty systemMessage: %s", out)
	}
}

// TestSessionEndQueues: the hook appends a transcript record to an isolated queue.
func TestSessionEndQueues(t *testing.T) {
	requireJQ(t)
	queue := filepath.Join(t.TempDir(), "queue.jsonl")
	in := `{"transcript_path":"/tmp/t.jsonl","cwd":"/home/x","session_id":"abc","reason":"clear"}`
	runHook(t, "hooks/session-end.sh", in, []string{"PERSISTOR_QUEUE=" + queue})

	data, err := os.ReadFile(queue)
	if err != nil {
		t.Fatalf("queue not written: %v", err)
	}
	var rec struct {
		TranscriptPath string `json:"transcript_path"`
		SessionID      string `json:"session_id"`
		TS             string `json:"ts"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("queue line not JSON: %v\n%s", err, data)
	}
	if rec.TranscriptPath != "/tmp/t.jsonl" || rec.SessionID != "abc" || rec.TS == "" {
		t.Errorf("queue record wrong: %+v", rec)
	}
}

// runHook executes a hook script with the given stdin and extra env, returning
// stdout. extraEnv entries are appended to the current environment.
func runHook(t *testing.T, script, stdin string, extraEnv []string) []byte {
	t.Helper()
	cmd := exec.Command("bash", script)
	cmd.Stdin = bytes.NewBufferString(stdin)
	cmd.Env = append(os.Environ(), extraEnv...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s: %v\nstderr: %s", script, err, errBuf.String())
	}
	return out.Bytes()
}
