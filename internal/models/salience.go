package models

import "fmt"

// SupersedeRequest represents a request to supersede one node with another.
type SupersedeRequest struct {
	OldID string `json:"old_id"`
	NewID string `json:"new_id"`
}

// Validate checks that both old_id and new_id are present and within length limits.
func (r *SupersedeRequest) Validate() error {
	if r.OldID == "" || r.NewID == "" {
		return fmt.Errorf("old_id and new_id are required")
	}

	if r.OldID == r.NewID {
		return fmt.Errorf("old_id and new_id must be different")
	}

	if len(r.OldID) > 255 {
		return ErrFieldTooLong("old_id", 255)
	}

	if len(r.NewID) > 255 {
		return ErrFieldTooLong("new_id", 255)
	}

	return nil
}
