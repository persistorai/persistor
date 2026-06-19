package eval

import "strings"

// scoreReturned compares the returned results against a question's expectations,
// returning the matched and missed expectation keys. matched+missed always
// partition the question's full expected set.
func scoreReturned(q *Question, returned []ReturnedResult) (matched, missed []string) {
	expectedSet := buildExpectedSet(q)
	seen := make(map[string]bool, len(expectedSet))
	for _, item := range returned {
		for _, key := range expectedKeysForResult(item) {
			if _, ok := expectedSet[key]; ok {
				seen[key] = true
			}
		}
	}

	matched = make([]string, 0, len(expectedSet))
	missed = make([]string, 0, len(expectedSet))
	for key := range expectedSet {
		if seen[key] {
			matched = append(matched, key)
		} else {
			missed = append(missed, key)
		}
	}
	return matched, missed
}

func mapResults(notes []NoteResult) []ReturnedResult {
	results := make([]ReturnedResult, 0, len(notes))
	for i := range notes {
		results = append(results, ReturnedResult{
			ID:    notes[i].ID,
			Title: notes[i].Title,
			Kind:  notes[i].Kind,
		})
	}
	return results
}

func buildExpectedSet(q *Question) map[string]struct{} {
	expected := make(map[string]struct{}, len(q.ExpectedNoteIDs)+len(q.ExpectedLabels))
	for _, id := range q.ExpectedNoteIDs {
		expected[normalizeExpected("id", id)] = struct{}{}
	}
	for _, label := range q.ExpectedLabels {
		expected[normalizeExpected("label", label)] = struct{}{}
	}
	return expected
}

func expectedKeysForResult(result ReturnedResult) []string {
	keys := []string{normalizeExpected("id", result.ID)}
	if result.Title != "" {
		keys = append(keys, normalizeExpected("label", result.Title))
	}
	return keys
}

func normalizeExpected(kind, value string) string {
	return kind + ":" + strings.ToLower(strings.TrimSpace(value))
}

func expectedCount(q *Question) int {
	return len(buildExpectedSet(q))
}

func preferredFirstExpectation(q *Question) string {
	if q.PreferredFirstNoteID != "" {
		return normalizeExpected("id", q.PreferredFirstNoteID)
	}
	if q.PreferredFirstLabel != "" {
		return normalizeExpected("label", q.PreferredFirstLabel)
	}
	return ""
}

func matchesExpectation(returned []ReturnedResult, expected string) bool {
	if expected == "" || len(returned) == 0 {
		return false
	}
	for _, key := range expectedKeysForResult(returned[0]) {
		if key == expected {
			return true
		}
	}
	return false
}

func normalizeCategory(category string) string {
	return strings.TrimSpace(strings.ToLower(category))
}
