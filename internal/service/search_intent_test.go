package service

import "testing"

func TestDetectSearchIntent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  SearchIntent
	}{
		{name: "entity", query: "Who is Big Jerry?", want: SearchIntentEntity},
		{name: "temporal", query: "What happened on Christmas Eve 2025?", want: SearchIntentTemporal},
		{name: "procedural", query: "What is Brian's stance on soft deletes?", want: SearchIntentProcedural},
		{name: "relationship", query: "What is the relationship between Brian and DeerPrint?", want: SearchIntentRelationship},
		{name: "general", query: "DeerPrint memory", want: SearchIntentGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectSearchIntent(tt.query); got != tt.want {
				t.Fatalf("DetectSearchIntent(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
