package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Fixture defines a memory evaluation dataset.
type Fixture struct {
	Name      string     `json:"name"`
	Questions []Question `json:"questions"`
}

// Question defines one benchmark question and its expected hits.
type Question struct {
	Prompt                string   `json:"prompt"`
	Category              string   `json:"category,omitempty"`
	SearchMode            string   `json:"search_mode,omitempty"`
	Limit                 int      `json:"limit,omitempty"`
	ExpectedNodeIDs       []string `json:"expected_node_ids,omitempty"`
	ExpectedLabels        []string `json:"expected_labels,omitempty"`
	PreferredFirstNodeID  string   `json:"preferred_first_node_id,omitempty"`
	PreferredFirstLabel   string   `json:"preferred_first_label,omitempty"`
	Notes                 string   `json:"notes,omitempty"`
	InternalRerankProfile string   `json:"internal_rerank_profile,omitempty"`
}

// Clone returns a deep copy of the fixture for deterministic comparison runs.
func (f *Fixture) Clone() *Fixture {
	if f == nil {
		return nil
	}
	clone := &Fixture{Name: f.Name, Questions: make([]Question, 0, len(f.Questions))}
	for _, q := range f.Questions {
		copied := q
		copied.ExpectedNodeIDs = append([]string(nil), q.ExpectedNodeIDs...)
		copied.ExpectedLabels = append([]string(nil), q.ExpectedLabels...)
		clone.Questions = append(clone.Questions, copied)
	}
	return clone
}

// ApplyPrototypeRerankProfile updates hybrid_rerank questions to use the named profile.
func (f *Fixture) ApplyPrototypeRerankProfile(profile string) {
	profile = strings.TrimSpace(strings.ToLower(profile))
	for i := range f.Questions {
		if strings.TrimSpace(strings.ToLower(f.Questions[i].SearchMode)) == "hybrid_rerank" {
			f.Questions[i].InternalRerankProfile = profile
		}
	}
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

	for i, q := range fixture.Questions {
		if err := validateQuestion(i, q); err != nil {
			return nil, err
		}
	}

	return &fixture, nil
}

func validateQuestion(i int, q Question) error {
	if q.Prompt == "" {
		return fmt.Errorf("question %d: prompt is required", i)
	}
	expected := buildExpectedSet(q)
	if len(expected) == 0 {
		return fmt.Errorf("question %d: at least one expected node id or label is required", i)
	}
	if q.PreferredFirstNodeID != "" {
		if _, ok := expected[normalizeExpected("id", q.PreferredFirstNodeID)]; !ok {
			return fmt.Errorf("question %d: preferred first node id must also be listed in expected_node_ids", i)
		}
	}
	if q.PreferredFirstLabel != "" {
		if _, ok := expected[normalizeExpected("label", q.PreferredFirstLabel)]; !ok {
			return fmt.Errorf("question %d: preferred first label must also be listed in expected_labels", i)
		}
	}
	return nil
}
