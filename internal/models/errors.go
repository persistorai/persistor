package models

import (
	"errors"
	"fmt"
)

// Sentinel errors for validation.
var (
	ErrMissingID       = errors.New("id is required")
	ErrMissingType     = errors.New("type is required")
	ErrMissingLabel    = errors.New("label is required")
	ErrMissingSource   = errors.New("source is required")
	ErrMissingTarget   = errors.New("target is required")
	ErrMissingRelation = errors.New("relation is required")
)

// Sentinel errors for entity lookups.
var (
	ErrNodeNotFound = errors.New("node not found")
	ErrEdgeNotFound = errors.New("edge not found")
)

// ErrDuplicateKey indicates a unique constraint violation (maps to HTTP 409 Conflict).
var ErrDuplicateKey = errors.New("duplicate key")

// ErrFieldTooLong returns an error indicating a field exceeds its maximum length.
func ErrFieldTooLong(field string, maxLen int) error {
	return fmt.Errorf("%s exceeds maximum length of %d", field, maxLen)
}
