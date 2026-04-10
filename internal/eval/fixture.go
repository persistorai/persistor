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
	Prompt          string   `json:"prompt"`
	SearchMode      string   `json:"search_mode,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	ExpectedNodeIDs []string `json:"expected_node_ids,omitempty"`
	ExpectedLabels  []string `json:"expected_labels,omitempty"`
	Notes           string   `json:"notes,omitempty"`
}

// LoadFixture reads and validates a fixture from disk.
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}

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

	for i, q := range fixture.Questions {
		if q.Prompt == "" {
			return nil, fmt.Errorf("question %d: prompt is required", i)
		}
		if len(q.ExpectedNodeIDs) == 0 && len(q.ExpectedLabels) == 0 {
			return nil, fmt.Errorf("question %d: at least one expected node id or label is required", i)
		}
	}

	return &fixture, nil
}
