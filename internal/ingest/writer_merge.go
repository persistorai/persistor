package ingest

import (
	"fmt"
	"time"
)

// mergeProperties merges new properties into existing ones, tracking conflicts.
func mergeProperties(existing, incoming map[string]any, source string) map[string]any {
	result := make(map[string]any, len(incoming)+1)
	history := extractHistory(existing)

	for key, newVal := range incoming {
		if key == "_property_history" || key == "_ingested_from" || key == "_ingested_at" {
			continue
		}
		oldVal, exists := existing[key]
		if !exists || valuesEqual(oldVal, newVal) {
			result[key] = newVal
			continue
		}
		result[key] = newVal
		history = appendHistory(history, key, oldVal, newVal, source)
	}

	if len(history) > 0 {
		result["_property_history"] = history
	}

	return result
}

// extractHistory pulls existing _property_history from node properties.
func extractHistory(props map[string]any) []map[string]any {
	raw, ok := props["_property_history"]
	if !ok {
		return nil
	}

	slice, ok := raw.([]map[string]any)
	if ok {
		return slice
	}

	// Handle the case where it's deserialized as []any.
	iface, ok := raw.([]any)
	if !ok {
		return nil
	}

	return convertHistory(iface)
}

// convertHistory converts []any to []map[string]any.
func convertHistory(iface []any) []map[string]any {
	result := make([]map[string]any, 0, len(iface))

	for _, item := range iface {
		entry, ok := item.(map[string]any)
		if ok {
			result = append(result, entry)
		}
	}

	return result
}

// appendHistory adds a conflict record to the property history.
func appendHistory(
	history []map[string]any,
	key string,
	oldVal, newVal any,
	source string,
) []map[string]any {
	entry := map[string]any{
		"property":   key,
		"old_value":  oldVal,
		"new_value":  newVal,
		"changed_at": time.Now().UTC().Format(time.RFC3339),
		"source":     source,
	}
	return append(history, entry)
}

// valuesEqual checks if two property values are considered equal.
func valuesEqual(a, b any) bool {
	// Simple string comparison handles the common case.
	aStr, aOK := a.(string)
	bStr, bOK := b.(string)
	if aOK && bOK {
		return aStr == bStr
	}

	// Fall back to formatted comparison.
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
