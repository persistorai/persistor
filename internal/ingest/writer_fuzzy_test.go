package ingest

import "testing"

func TestFuzzyMatchLabel_ExactAfterNormalization(t *testing.T) {
	score := fuzzyMatchLabel("Alltel Wireless", "Alltel Wireless")
	if score != 1.0 {
		t.Errorf("exact match: expected 1.0, got %f", score)
	}
}

func TestFuzzyMatchLabel_NormalizedExact(t *testing.T) {
	score := fuzzyMatchLabel("Alltel, Inc.", "Alltel")
	if score != 1.0 {
		t.Errorf("normalized exact: expected 1.0, got %f", score)
	}
}

func TestFuzzyMatchLabel_ContainsMatch(t *testing.T) {
	score := fuzzyMatchLabel("Alltel Wireless", "Alltel")
	if score < 0.8 {
		t.Errorf("contains match: expected >= 0.8, got %f", score)
	}
}

func TestFuzzyMatchLabel_ReverseContains(t *testing.T) {
	score := fuzzyMatchLabel("Alltel", "Alltel Wireless")
	if score < 0.8 {
		t.Errorf("reverse contains: expected >= 0.8, got %f", score)
	}
}

func TestFuzzyMatchLabel_NoMatch(t *testing.T) {
	score := fuzzyMatchLabel("Alltel", "Verizon")
	if score != 0.0 {
		t.Errorf("no match: expected 0.0, got %f", score)
	}
}

func TestFuzzyMatchLabel_CaseInsensitive(t *testing.T) {
	score := fuzzyMatchLabel("ALLTEL", "alltel")
	if score != 1.0 {
		t.Errorf("case insensitive: expected 1.0, got %f", score)
	}
}

func TestFuzzyMatchLabel_StripsSuffixes(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		minScore float64
	}{
		{"inc suffix", "Acme, Inc.", "Acme", 1.0},
		{"llc suffix", "Acme LLC", "Acme", 1.0},
		{"corp suffix", "Acme Corp.", "Acme", 1.0},
		{"ltd suffix", "Acme, Ltd.", "Acme", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := fuzzyMatchLabel(tt.a, tt.b)
			if score < tt.minScore {
				t.Errorf("expected >= %f, got %f", tt.minScore, score)
			}
		})
	}
}

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Alltel Wireless  ", "alltel"},
		{"Acme, Inc.", "acme"},
		{"Foo LLC", "foo"},
		{"Simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLabel(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeLabel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
