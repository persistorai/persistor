package ingest

import "strings"

var factPropertyAliases = map[string]string{
	"email_given_date":      "email_assigned_date",
	"voice":                 "voice_name",
	"voice_selected":        "voice_selection_timing",
	"voice_chosen_date":     "email_assigned_date",
	"name_chosen_date":      "born_on",
	"role_description":      "role",
	"retired":               "retired_date",
	"smoking_started":       "smoking_start_age",
	"weight_lb":             "current_weight",
	"location":              "residence",
}

func normalizeFactProperty(key string) string {
	trimmed := strings.TrimSpace(strings.ToLower(key))
	if trimmed == "" {
		return ""
	}
	if canonical, ok := factPropertyAliases[trimmed]; ok {
		return canonical
	}
	return trimmed
}
