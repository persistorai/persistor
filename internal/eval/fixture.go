package eval

import (
	"encoding/json"
	"fmt"
	"os"
)

// Fixture defines a memory evaluation dataset.
type Fixture struct {
	Name      string     `json:"name"`
	Questions []Question `json:"questions"`
}

// Question defines one benchmark question and its expected hits.
type Question struct {
	Prompt               string   `json:"prompt"`
	Category             string   `json:"category,omitempty"`
	Limit                int      `json:"limit,omitempty"`
	ExpectedNoteIDs      []string `json:"expected_note_ids,omitempty"`
	ExpectedLabels       []string `json:"expected_labels,omitempty"`
	PreferredFirstNoteID string   `json:"preferred_first_note_id,omitempty"`
	PreferredFirstLabel  string   `json:"preferred_first_label,omitempty"`
	// ExpectAbstain inverts the expectation: the corpus holds NO answer, so the
	// question passes only when retrieval returns ZERO results. Guards the
	// abstention property (LongMemEval's fifth ability): memory must say
	// "nothing" for absent information, not surface confident-looking noise.
	ExpectAbstain bool   `json:"expect_abstain,omitempty"`
	Notes         string `json:"notes,omitempty"`
}

// LoadFixture reads and validates a fixture from disk.
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}
	return parseFixture(data)
}

func parseFixture(data []byte) (*Fixture, error) {
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("parse fixture: %w", err)
	}

	if fixture.Name == "" {
		return nil, fmt.Errorf("fixture name is required")
	}

	if len(fixture.Questions) == 0 {
		return nil, fmt.Errorf("fixture must contain at least one question")
	}

	for i := range fixture.Questions {
		if err := validateQuestion(i, &fixture.Questions[i]); err != nil {
			return nil, err
		}
	}

	return &fixture, nil
}

func validateQuestion(i int, q *Question) error {
	if q.Prompt == "" {
		return fmt.Errorf("question %d: prompt is required", i)
	}
	expected := buildExpectedSet(q)
	if q.ExpectAbstain {
		if len(expected) != 0 || q.PreferredFirstNoteID != "" || q.PreferredFirstLabel != "" {
			return fmt.Errorf("question %d: expect_abstain cannot be combined with expected ids/labels", i)
		}
		return nil
	}
	if len(expected) == 0 {
		return fmt.Errorf("question %d: at least one expected note id or label is required", i)
	}
	if q.PreferredFirstNoteID != "" {
		if _, ok := expected[normalizeExpected("id", q.PreferredFirstNoteID)]; !ok {
			return fmt.Errorf("question %d: preferred first note id must also be listed in expected_note_ids", i)
		}
	}
	if q.PreferredFirstLabel != "" {
		if _, ok := expected[normalizeExpected("label", q.PreferredFirstLabel)]; !ok {
			return fmt.Errorf("question %d: preferred first label must also be listed in expected_labels", i)
		}
	}
	return nil
}
