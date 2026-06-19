package eval

// The eval harness scores retrieval by matching returned result ids/titles
// against a fixture's expectations. These local types are the minimal result
// shape it needs, so the package stands alone.

// NoteResult is a minimal retrieved note: the fields the evaluator matches on.
type NoteResult struct {
	ID    string
	Kind  string
	Title string
}

// SearchOptions tunes a retrieval call. Retrieval is full-text only, so Limit is
// the only knob.
type SearchOptions struct {
	Limit int
}
