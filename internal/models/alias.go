package models

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Alias represents an alternate name persisted for a node.
type Alias struct {
	ID              uuid.UUID `json:"id"`
	TenantID        uuid.UUID `json:"-"`
	NodeID          string    `json:"node_id"`
	Alias           string    `json:"alias"`
	NormalizedAlias string    `json:"normalized_alias"`
	AliasType       string    `json:"alias_type,omitempty"`
	Confidence      float64   `json:"confidence"`
	Source          string    `json:"source,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateAliasRequest is the payload for creating a persisted alias.
type CreateAliasRequest struct {
	NodeID     string   `json:"node_id"`
	Alias      string   `json:"alias"`
	AliasType  string   `json:"alias_type,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Source     string   `json:"source,omitempty"`
}

// AliasListOpts controls alias listing queries.
type AliasListOpts struct {
	NodeID          string
	NormalizedAlias string
	AliasType       string
	Limit           int
	Offset          int
}

// Validate checks CreateAliasRequest fields and fills defaults.
func (r *CreateAliasRequest) Validate() error {
	if r.NodeID == "" {
		return fmt.Errorf("node_id: %w", ErrMissingID)
	}

	if len(r.NodeID) > 255 {
		return ErrFieldTooLong("node_id", 255)
	}

	if strings.TrimSpace(r.Alias) == "" {
		return fmt.Errorf("alias is required")
	}

	if len(r.Alias) > 1000 {
		return ErrFieldTooLong("alias", 1000)
	}

	if len(r.AliasType) > 100 {
		return ErrFieldTooLong("alias_type", 100)
	}

	if len(r.Source) > 255 {
		return ErrFieldTooLong("source", 255)
	}

	if r.Confidence == nil {
		defaultConfidence := 1.0
		r.Confidence = &defaultConfidence
	}

	if *r.Confidence < 0 || *r.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	return nil
}

// NormalizeAlias converts aliases into a deterministic, index-friendly form.
func NormalizeAlias(alias string) string {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(alias))
	prevSpace := false

	for _, r := range alias {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			b.WriteByte(' ')
			prevSpace = true
			continue
		}

		b.WriteRune(r)
		prevSpace = false
	}

	return b.String()
}
