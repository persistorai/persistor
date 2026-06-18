package index_test

import (
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

func TestSeedQuery_FromCwdAndReadme(t *testing.T) {
	dir := t.TempDir()
	// Place a README so the title contributes terms.
	writeFile(t, dir, "README.md", "# Deer Print Inference\n\nstuff\n")

	// Rename-free: SeedQuery uses filepath.Base(dir); the temp dir's base is
	// random, so just assert the README title terms are present and lowercased.
	seed := index.SeedQuery(dir, "extra topic")
	for _, want := range []string{"deer", "print", "inference", "extra", "topic"} {
		if !strings.Contains(seed, want) {
			t.Errorf("seed %q missing %q", seed, want)
		}
	}
	if seed != strings.ToLower(seed) {
		t.Errorf("seed not lowercased: %q", seed)
	}
}

func TestSeedQuery_EmptyCwdNoPanic(t *testing.T) {
	if got := index.SeedQuery(""); got == "" {
		// empty is acceptable; just must not panic.
		_ = got
	}
}
