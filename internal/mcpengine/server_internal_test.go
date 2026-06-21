package mcpengine

import (
	"encoding/json"
	"testing"
)

// TestASCIISafeJSON: the helper turns valid UTF-8 JSON into pure-ASCII JSON that
// still decodes to the identical value — exercising the warning emoji + U+FE0F
// variation selector (the sequence that broke the claude.ai connector), a
// bidirectional arrow, and an astral-plane rune (surrogate-pair path).
func TestASCIISafeJSON(t *testing.T) {
	type payload struct {
		Body string `json:"body"`
	}
	original := payload{Body: "warn ⚠️ link ↔ dash — deer \U0001F98C end"}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := asciiSafeJSON(raw)

	for i := 0; i < len(got); i++ {
		if got[i] >= 0x80 {
			t.Fatalf("non-ASCII byte 0x%02x at offset %d in %q", got[i], i, got)
		}
	}

	var back payload
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("ascii-safe output is not valid JSON: %v", err)
	}
	if back.Body != original.Body {
		t.Fatalf("round-trip mismatch:\n got  %q\n want %q", back.Body, original.Body)
	}
}
