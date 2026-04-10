package models

import (
	"fmt"
	"strings"
)

const maxSearchTextProperties = 40

var prioritizedSearchKeys = []string{
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
	"date",
	"when",
	"happened_on",
	"happened on",
	"breakthrough_date",
	"deployment_summary",
	"inference_fix_summary",
}

// BuildNodeSearchText builds a deterministic text blob for full-text indexing.
func BuildNodeSearchText(node *Node) string {
	if node == nil {
		return ""
	}

	builder := strings.Builder{}
	appendSearchLine(&builder, node.Label)
	appendSearchLine(&builder, node.Type)

	for _, value := range searchPropertyValues(node.Properties) {
		appendSearchLine(&builder, value)
	}
	appendSearchLine(&builder, BuildNodeFactText(node))

	return strings.TrimSpace(builder.String())
}

// BuildNodeSummarySearchText builds full-text index input for lightweight node summaries.
func BuildNodeSummarySearchText(node *NodeSummary) string {
	if node == nil {
		return ""
	}

	builder := strings.Builder{}
	appendSearchLine(&builder, node.Label)
	appendSearchLine(&builder, node.Type)
	return strings.TrimSpace(builder.String())
}

func appendSearchLine(builder *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if builder.Len() > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString(value)
}

func searchPropertyValues(properties map[string]any) []string {
	if len(properties) == 0 {
		return nil
	}

	keys := prioritizedKeys(properties, prioritizedSearchKeys, shouldSkipSearchProperty, maxSearchTextProperties)

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		text := normalizeSearchValue(properties[key])
		if text == "" {
			continue
		}
		values = append(values, text)
	}
	return values
}

func shouldSkipSearchProperty(key string) bool {
	return strings.HasPrefix(key, "_")
}

func normalizeSearchValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", v)
	case []string:
		return strings.Join(v, " ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := normalizeSearchValue(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}
