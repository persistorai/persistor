package models

import (
	"fmt"
	"sort"
	"strings"
)

const maxFactProperties = 32

var prioritizedFactKeys = []string{
	"description",
	"summary",
	"note",
	"details",
	"owner",
	"owned_by",
	"owned by",
	"not_owned_by",
	"not owned by",
	"belongs_to",
	"belongs to",
	"personal_project",
	"purpose",
	"status",
	"stance",
	"preference",
	"policy",
	"matters",
	"important_to",
	"important to",
	"date",
	"when",
	"happened_on",
	"happened on",
	"breakthrough_date",
	"deployment_summary",
	"inference_fix_summary",
}

// BuildNodeFactText renders compact fact-style lines from node properties to improve
// retrieval for natural language questions over preferences, ownership, time, and identity.
func BuildNodeFactText(node *Node) string {
	if node == nil || len(node.Properties) == 0 {
		return ""
	}

	keys := prioritizedKeys(node.Properties, prioritizedFactKeys, shouldSkipFactProperty, maxFactProperties)

	lines := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		value := normalizeFactValue(node.Properties[key])
		if value == "" {
			continue
		}
		lines = append(lines, key+": "+value)
		if sentence := factSentence(node.Label, key, value); sentence != "" {
			lines = append(lines, sentence)
		}
	}

	return strings.Join(lines, "\n")
}

func shouldSkipFactProperty(key string) bool {
	return strings.HasPrefix(key, "_")
}

func normalizeFactValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.Join(v, ", ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := normalizeFactValue(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func factSentence(label, key, value string) string {
	key = strings.TrimSpace(strings.ReplaceAll(key, "_", " "))
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if label == "" || key == "" || value == "" {
		return ""
	}

	switch strings.ToLower(key) {
	case "owner", "owned by", "owned_by":
		return label + " is owned by " + value
	case "not owned by", "not_owned_by":
		return label + " is not owned by " + value
	case "belongs to", "belongs_to", "personal_project":
		return label + " belongs to " + value
	case "priority", "matters", "important to", "important_to":
		return label + " matters because " + value
	case "stance", "preference", "policy":
		return label + " policy: " + value
	case "date", "when", "happened on", "happened_on", "breakthrough_date":
		return label + " happened on " + value
	case "description", "summary", "note", "details", "purpose", "status", "deployment_summary", "inference_fix_summary":
		return label + " " + key + " " + value
	default:
		return label + " " + key + " " + value
	}
}

func prioritizedKeys(properties map[string]any, priority []string, skip func(string) bool, limit int) []string {
	if len(properties) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(properties))
	keys := make([]string, 0, len(properties))
	for _, key := range priority {
		if _, ok := properties[key]; !ok || skip(key) {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	rest := make([]string, 0, len(properties))
	for key := range properties {
		if skip(key) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		rest = append(rest, key)
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return keys
}
