package models

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	RetrievalOutcomeHelpful   = "helpful"
	RetrievalOutcomeUnhelpful = "unhelpful"
	RetrievalOutcomeMissed    = "missed"

	RetrievalSignalConfirmedRecall  = "confirmed_recall"
	RetrievalSignalIrrelevantResult = "irrelevant_result"
	RetrievalSignalMissedKnownItem  = "missed_known_item"
	RetrievalSignalEmptyResult      = "empty_result"

	DefaultRetrievalFeedbackListLimit = 25
	MaxRetrievalFeedbackListLimit     = 100
)

// RetrievalFeedbackRequest is an explicit, operator-visible feedback event for one retrieval attempt.
// It is intentionally manual and bounded: no automatic firehose logging, no hidden adaptation.
type RetrievalFeedbackRequest struct {
	Query            string   `json:"query"`
	SearchMode       string   `json:"search_mode,omitempty"`
	Intent           string   `json:"intent,omitempty"`
	InternalRerank   string   `json:"internal_rerank,omitempty"`
	RerankProfile    string   `json:"internal_rerank_profile,omitempty"`
	Outcome          string   `json:"outcome"`
	RetrievedNodeIDs []string `json:"retrieved_node_ids,omitempty"`
	SelectedNodeIDs  []string `json:"selected_node_ids,omitempty"`
	ExpectedNodeIDs  []string `json:"expected_node_ids,omitempty"`
	Note             string   `json:"note,omitempty"`
}

func (r RetrievalFeedbackRequest) Normalized() RetrievalFeedbackRequest {
	r.Query = strings.TrimSpace(r.Query)
	r.SearchMode = normalizeLowerToken(r.SearchMode)
	r.Intent = normalizeLowerToken(r.Intent)
	r.InternalRerank = normalizeLowerToken(r.InternalRerank)
	r.RerankProfile = normalizeLowerToken(r.RerankProfile)
	r.Outcome = normalizeRetrievalOutcome(r.Outcome)
	r.Note = strings.TrimSpace(r.Note)
	r.RetrievedNodeIDs = dedupeSortedStrings(r.RetrievedNodeIDs)
	r.SelectedNodeIDs = dedupeSortedStrings(r.SelectedNodeIDs)
	r.ExpectedNodeIDs = dedupeSortedStrings(r.ExpectedNodeIDs)
	return r
}

func (r RetrievalFeedbackRequest) Validate() error {
	if strings.TrimSpace(r.Query) == "" {
		return fmt.Errorf("query is required")
	}
	if len(r.Query) > 500 {
		return fmt.Errorf("query exceeds 500 characters")
	}
	switch normalizeRetrievalOutcome(r.Outcome) {
	case RetrievalOutcomeHelpful, RetrievalOutcomeUnhelpful, RetrievalOutcomeMissed:
	default:
		return fmt.Errorf("outcome must be helpful, unhelpful, or missed")
	}
	if len(r.Note) > 500 {
		return fmt.Errorf("note exceeds 500 characters")
	}
	if len(r.RetrievedNodeIDs) > 20 || len(r.SelectedNodeIDs) > 20 || len(r.ExpectedNodeIDs) > 20 {
		return fmt.Errorf("node id lists must contain at most 20 items")
	}
	return nil
}

// RetrievalFeedbackRecord is the persisted retrieval feedback event.
type RetrievalFeedbackRecord struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id,omitempty"`
	Query            string    `json:"query"`
	NormalizedQuery  string    `json:"normalized_query"`
	SearchMode       string    `json:"search_mode,omitempty"`
	Intent           string    `json:"intent,omitempty"`
	InternalRerank   string    `json:"internal_rerank,omitempty"`
	RerankProfile    string    `json:"internal_rerank_profile,omitempty"`
	Outcome          string    `json:"outcome"`
	Signals          []string  `json:"signals,omitempty"`
	RetrievedNodeIDs []string  `json:"retrieved_node_ids,omitempty"`
	SelectedNodeIDs  []string  `json:"selected_node_ids,omitempty"`
	ExpectedNodeIDs  []string  `json:"expected_node_ids,omitempty"`
	Note             string    `json:"note,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RetrievalFeedbackListOpts controls bounded retrieval feedback reads.
type RetrievalFeedbackListOpts struct {
	Limit int `json:"limit,omitempty"`
}

func (o RetrievalFeedbackListOpts) Normalized() RetrievalFeedbackListOpts {
	if o.Limit <= 0 {
		o.Limit = DefaultRetrievalFeedbackListLimit
	}
	if o.Limit > MaxRetrievalFeedbackListLimit {
		o.Limit = MaxRetrievalFeedbackListLimit
	}
	return o
}

type RetrievalFeedbackSummary struct {
	TotalEvents    int                            `json:"total_events"`
	OutcomeCounts  map[string]int                 `json:"outcome_counts,omitempty"`
	SignalCounts   map[string]int                 `json:"signal_counts,omitempty"`
	RecentEvents   []RetrievalFeedbackRecord      `json:"recent_events,omitempty"`
	QueryBreakdown []RetrievalFeedbackQueryBucket `json:"query_breakdown,omitempty"`
}

type RetrievalFeedbackQueryBucket struct {
	NormalizedQuery string         `json:"normalized_query"`
	ExampleQuery    string         `json:"example_query"`
	SearchMode      string         `json:"search_mode,omitempty"`
	OutcomeCounts   map[string]int `json:"outcome_counts,omitempty"`
	SignalCounts    map[string]int `json:"signal_counts,omitempty"`
	Signals         []string       `json:"signals,omitempty"`
	LastSeenAt      time.Time      `json:"last_seen_at"`
}

func NormalizeRetrievalQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(query))), " ")
}

func DeriveRetrievalSignals(req RetrievalFeedbackRequest) []string {
	req = req.Normalized()
	signals := make([]string, 0, 4)
	containsSelected := intersects(req.RetrievedNodeIDs, req.SelectedNodeIDs)
	containsExpected := intersects(req.RetrievedNodeIDs, req.ExpectedNodeIDs)

	switch req.Outcome {
	case RetrievalOutcomeHelpful:
		if containsSelected || len(req.SelectedNodeIDs) == 0 {
			signals = append(signals, RetrievalSignalConfirmedRecall)
		}
	case RetrievalOutcomeUnhelpful:
		if len(req.RetrievedNodeIDs) > 0 {
			signals = append(signals, RetrievalSignalIrrelevantResult)
		}
	case RetrievalOutcomeMissed:
		if len(req.ExpectedNodeIDs) > 0 && !containsExpected {
			signals = append(signals, RetrievalSignalMissedKnownItem)
		}
	}
	if len(req.RetrievedNodeIDs) == 0 {
		signals = append(signals, RetrievalSignalEmptyResult)
	}
	return dedupeSortedStrings(signals)
}

func normalizeRetrievalOutcome(v string) string {
	switch normalizeLowerToken(v) {
	case RetrievalOutcomeHelpful:
		return RetrievalOutcomeHelpful
	case RetrievalOutcomeUnhelpful:
		return RetrievalOutcomeUnhelpful
	case RetrievalOutcomeMissed:
		return RetrievalOutcomeMissed
	default:
		return ""
	}
}

func normalizeLowerToken(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func dedupeSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func intersects(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}
