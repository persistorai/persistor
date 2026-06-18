package eval

// The eval harness scores retrieval by matching returned result ids/labels
// against a fixture's expectations. These local types are the minimal result
// shape it needs, so the package stands alone.

// Node is a minimal retrieved result: the fields the evaluator matches on.
type Node struct {
	ID    string
	Type  string
	Label string
}

// SearchOptions tunes a retrieval call. Retrieval is full-text only, so Limit is
// the only knob.
type SearchOptions struct {
	Limit int
}
