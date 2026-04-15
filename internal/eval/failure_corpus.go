package eval

import (
	"encoding/json"
	"fmt"
	"os"
)

// FailureCorpus captures a replayable set of known-bad retrievals or edge cases.
// It intentionally reuses Question semantics so corpus entries can be run through
// the normal evaluator without a separate execution path.
type FailureCorpus struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Cases       []FailureCase `json:"cases"`
}

// FailureCase records one known bad retrieval or edge-case query.
type FailureCase struct {
	ID                    string   `json:"id,omitempty"`
	Prompt                string   `json:"prompt"`
	Category              string   `json:"category,omitempty"`
	SearchMode            string   `json:"search_mode,omitempty"`
	Limit                 int      `json:"limit,omitempty"`
	ExpectedNodeIDs       []string `json:"expected_node_ids,omitempty"`
	ExpectedLabels        []string `json:"expected_labels,omitempty"`
	PreferredFirstNodeID  string   `json:"preferred_first_node_id,omitempty"`
	PreferredFirstLabel   string   `json:"preferred_first_label,omitempty"`
	FailureMode           string   `json:"failure_mode,omitempty"`
	KnownBadLabels        []string `json:"known_bad_labels,omitempty"`
	KnownBadNodeIDs       []string `json:"known_bad_node_ids,omitempty"`
	Notes                 string   `json:"notes,omitempty"`
	InternalRerankProfile string   `json:"internal_rerank_profile,omitempty"`
}

// ToFixture converts the replay corpus into a normal eval fixture.
func (c *FailureCorpus) ToFixture() *Fixture {
	if c == nil {
		return nil
	}
	fixture := &Fixture{Name: c.Name, Questions: make([]Question, 0, len(c.Cases))}
	for _, item := range c.Cases {
		fixture.Questions = append(fixture.Questions, Question{
			Prompt:                item.Prompt,
			Category:              item.Category,
			SearchMode:            item.SearchMode,
			Limit:                 item.Limit,
			ExpectedNodeIDs:       append([]string(nil), item.ExpectedNodeIDs...),
			ExpectedLabels:        append([]string(nil), item.ExpectedLabels...),
			PreferredFirstNodeID:  item.PreferredFirstNodeID,
			PreferredFirstLabel:   item.PreferredFirstLabel,
			Notes:                 item.Notes,
			InternalRerankProfile: item.InternalRerankProfile,
		})
	}
	return fixture
}

// LoadFixtureOrFailureCorpus reads either a standard fixture or a failure corpus.
func LoadFixtureOrFailureCorpus(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}
	if fixture, err := parseFixture(data); err == nil {
		return fixture, nil
	}
	corpus, err := parseFailureCorpus(data)
	if err != nil {
		return nil, err
	}
	return corpus.ToFixture(), nil
}

func parseFailureCorpus(data []byte) (*FailureCorpus, error) {
	var corpus FailureCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, fmt.Errorf("parse fixture: %w", err)
	}
	if corpus.Name == "" {
		return nil, fmt.Errorf("fixture name is required")
	}
	if len(corpus.Cases) == 0 {
		return nil, fmt.Errorf("failure corpus must contain at least one case")
	}
	for i, c := range corpus.Cases {
		q := Question{
			Prompt:               c.Prompt,
			ExpectedNodeIDs:      c.ExpectedNodeIDs,
			ExpectedLabels:       c.ExpectedLabels,
			PreferredFirstNodeID: c.PreferredFirstNodeID,
			PreferredFirstLabel:  c.PreferredFirstLabel,
		}
		if err := validateQuestion(i, q); err != nil {
			return nil, err
		}
	}
	return &corpus, nil
}
