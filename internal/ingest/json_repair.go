package ingest

import (
	"encoding/json"
	"strings"
)

// RepairJSON attempts to fix common LLM JSON output issues.
// Handles: markdown code fences, truncated JSON, extraneous text.
func RepairJSON(raw string) string {
	cleaned := stripCodeFences(raw)
	extracted := extractJSONObject(cleaned)

	if extracted == "" {
		return raw
	}

	if json.Valid([]byte(extracted)) {
		return extracted
	}

	repaired := stripTrailingCommas(extracted)
	if json.Valid([]byte(repaired)) {
		return repaired
	}

	repaired = closeBrackets(repaired)
	if json.Valid([]byte(repaired)) {
		return repaired
	}

	repaired = normalizeArrayObjectBoundaries(repaired)
	return repaired
}

// stripCodeFences removes markdown code fences from the string.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)

	// Remove opening fence (```json or ```)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}

	// Remove closing fence.
	s = strings.TrimSuffix(s, "```")

	return strings.TrimSpace(s)
}

// extractJSONObject finds the first '{' and last '}' and extracts that substring.
func extractJSONObject(s string) string {
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")

	if first == -1 {
		return ""
	}

	if last == -1 || last <= first {
		// No closing brace — return from first brace to end for repair
		return s[first:]
	}

	return s[first : last+1]
}

// stripTrailingCommas removes trailing commas before } or ].
func stripTrailingCommas(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != ',' {
			result.WriteRune(runes[i])
			continue
		}

		if isTrailingComma(runes, i) {
			continue // skip this comma
		}

		result.WriteRune(runes[i])
	}

	return result.String()
}

// isTrailingComma checks if the comma at position i is followed only
// by whitespace and then a closing bracket/brace.
func isTrailingComma(runes []rune, i int) bool {
	for j := i + 1; j < len(runes); j++ {
		switch runes[j] {
		case ' ', '\t', '\n', '\r':
			continue
		case '}', ']':
			return true
		default:
			return false
		}
	}

	return false
}

// closeBrackets counts unclosed brackets and appends closing ones.
func normalizeArrayObjectBoundaries(s string) string {
	replacer := strings.NewReplacer(
		"]{", "},{",
		"}[{", ",[{",
		"}{", "},{",
	)
	return replacer.Replace(s)
}

func closeBrackets(s string) string {
	s = stripTrailingCommas(s)

	var stack []rune
	inString := false
	escaped := false

	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}

		if r == '\\' && inString {
			escaped = true
			continue
		}

		if r == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch r {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Append closing brackets in reverse order
	var closer strings.Builder
	closer.WriteString(s)

	for i := len(stack) - 1; i >= 0; i-- {
		closer.WriteRune(stack[i])
	}

	return closer.String()
}
