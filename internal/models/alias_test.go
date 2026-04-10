package models_test

import (
	"testing"

	"github.com/persistorai/persistor/internal/models"
)

func TestNormalizeAlias(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "trim lowercase collapse spaces", input: "  BILL   GATES  ", want: "bill gates"},
		{name: "tabs and newlines", input: "Ada\t\nLovelace", want: "ada lovelace"},
		{name: "empty after trim", input: "   ", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := models.NormalizeAlias(tc.input); got != tc.want {
				t.Fatalf("NormalizeAlias(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCreateAliasRequest_Validate(t *testing.T) {
	confidence := 0.7
	valid := models.CreateAliasRequest{NodeID: "n1", Alias: "Alias", Confidence: &confidence}
	assertNoError(t, valid.Validate())

	assertErrorContains(t, (&models.CreateAliasRequest{}).Validate(), "node_id")
	assertErrorContains(t, (&models.CreateAliasRequest{NodeID: "n1"}).Validate(), "alias is required")
	assertErrorContains(t, (&models.CreateAliasRequest{NodeID: "n1", Alias: "Alias", Confidence: &[]float64{1.2}[0]}).Validate(), "confidence must be between 0 and 1")
}
