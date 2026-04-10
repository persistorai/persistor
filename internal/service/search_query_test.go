package service

import "testing"

func TestBuildSearchQueryVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		contains []string
	}{
		{
			name:     "entity question strips lead phrase",
			query:    "Who is Big Jerry?",
			contains: []string{"who is big jerry", "big jerry"},
		},
		{
			name:     "temporal question extracts event and year",
			query:    "What happened on Christmas Eve 2025?",
			contains: []string{"what happened on christmas eve 2025", "christmas eve 2025", "2025", "christmas"},
		},
		{
			name:     "procedural question strips stance phrasing",
			query:    "What is Brian's stance on soft deletes?",
			contains: []string{"what is brian's stance on soft deletes", "brian's soft deletes", "deletes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := BuildSearchQueryVariants(tt.query)
			for _, want := range tt.contains {
				if !containsString(got, want) {
					t.Fatalf("BuildSearchQueryVariants(%q) missing %q in %#v", tt.query, want, got)
				}
			}
		})
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
