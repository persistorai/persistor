package models

import (
	"fmt"
	"strings"
)

const maxEmbeddedProperties = 24

var prioritizedEmbeddingKeys = []string{
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

// EmbeddingText returns a richer, deterministic embedding document for batch operations.
func (n *NodeSummary) EmbeddingText() string {
	builder := strings.Builder{}
	builder.WriteString("type: ")
	builder.WriteString(strings.TrimSpace(n.Type))
	builder.WriteString("\n")
	builder.WriteString("label: ")
	builder.WriteString(strings.TrimSpace(n.Label))
	return builder.String()
}

// BuildNodeEmbeddingText returns a richer, deterministic text representation
// suitable for semantic embedding generation.
func BuildNodeEmbeddingText(node *Node) string {
	if node == nil {
		return ""
	}

	builder := strings.Builder{}
	builder.WriteString("type: ")
	builder.WriteString(strings.TrimSpace(node.Type))
	builder.WriteString("\n")
	builder.WriteString("label: ")
	builder.WriteString(strings.TrimSpace(node.Label))

	props := embeddingProperties(node.Properties)
	if len(props) > 0 {
		builder.WriteString("\nproperties:")
		for _, prop := range props {
			builder.WriteString("\n- ")
			builder.WriteString(prop)
		}
	}

	if factText := BuildNodeFactText(node); factText != "" {
		builder.WriteString("\nfacts:\n")
		builder.WriteString(factText)
	}

	return builder.String()
}

func embeddingProperties(properties map[string]any) []string {
	if len(properties) == 0 {
		return nil
	}

	keys := prioritizedKeys(properties, prioritizedEmbeddingKeys, shouldSkipEmbeddingProperty, maxEmbeddedProperties)

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		valueText := normalizeEmbeddingValue(properties[key])
		if valueText == "" {
			continue
		}
		result = append(result, key+": "+valueText)
	}
	return result
}

func shouldSkipEmbeddingProperty(key string) bool {
	return strings.HasPrefix(key, "_")
}

func normalizeEmbeddingValue(value any) string {
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
		return strings.Join(v, ", ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := normalizeEmbeddingValue(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}
