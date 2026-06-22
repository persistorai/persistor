package mcpengine

import (
	"context"
	"fmt"
	"strings"

	"github.com/briancolinger/persistor/internal/index"
)

// WriteInput is the memory_write argument shape — one durable note.
type WriteInput struct {
	Path            string `json:"path,omitempty" jsonschema:"Optional .md path used to derive the note id when id is omitted (no absolute paths or ..). The note is stored in Postgres, not as a file."`
	Body            string `json:"body" jsonschema:"The note's markdown prose. Do not include a frontmatter block; the typed fields carry the metadata."`
	ID              string `json:"id,omitempty" jsonschema:"Stable note id. Derived from the path (namespace-prefixed) when omitted."`
	Namespace       string `json:"namespace,omitempty" jsonschema:"Logical bucket for the note (e.g. demo, claude, work, personal). Defaults to 'default'."`
	Kind            string `json:"kind,omitempty" jsonschema:"fact|decision|episode|reference|preference. Default fact."`
	Tier            string `json:"tier,omitempty" jsonschema:"core (always-loaded) or tail (retrieved). Default tail."`
	Title           string `json:"title,omitempty" jsonschema:"Short title. Derived from the first heading when omitted."`
	Supersedes      string `json:"supersedes,omitempty" jsonschema:"Id of the note this CORRECTS. Use only to replace a stale fact, not for a new point in a timeline."`
	ExpectedVersion int    `json:"expected_version,omitempty" jsonschema:"Optimistic-concurrency guard. Omit (or 0) to CREATE a new note; pass the current version (from memory_get) to UPDATE an existing one. A mismatch is rejected as a version conflict."`
}

// WriteOutput is the memory_write result shape.
type WriteOutput struct {
	ID         string `json:"id"`         // the note id written
	Version    int    `json:"version"`    // the note's new version
	Op         string `json:"op"`         // create | update
	Superseded int    `json:"superseded"` // supersession flips this write reconciled
}

// Write creates or updates a single PG-native note under optimistic concurrency,
// then reconciles supersession across the tenant. The tenant comes from the
// verified token, so the write is tenant-isolated with no shared directory.
func (e *Engine) Write(ctx context.Context, in *WriteInput) (WriteOutput, error) {
	if e.readOnly {
		return WriteOutput{}, ErrReadOnly
	}
	if !e.limiter.Allow(e.tenantID) {
		return WriteOutput{}, ErrRateLimited
	}
	if strings.TrimSpace(in.Body) == "" {
		return WriteOutput{}, fmt.Errorf("body is required")
	}
	if !index.ValidKind(in.Kind) {
		return WriteOutput{}, fmt.Errorf("invalid kind %q", in.Kind)
	}
	if !index.ValidTier(in.Tier) {
		return WriteOutput{}, fmt.Errorf("invalid tier %q (want core|tail)", in.Tier)
	}
	id, err := index.DeriveNoteID(in.Namespace, in.Path, in.ID)
	if err != nil {
		return WriteOutput{}, err
	}
	// Reject a self-supersede: a write whose supersedes is the very id it lands on
	// forks no history and would otherwise silently do nothing. The supersedes
	// TARGET-exists check (rejecting a dangling pointer a prompt-injected write
	// could use to fake a correction) now runs inside WriteNote's transaction, so
	// the precondition and the write commit atomically.
	if in.Supersedes != "" && in.Supersedes == id {
		return WriteOutput{}, &index.SelfSupersedeError{ID: id, Path: in.Path}
	}

	res, err := e.store.WriteNote(ctx, e.tenantID, &index.PGNoteInput{
		ID:         id,
		Namespace:  in.Namespace,
		Kind:       in.Kind,
		Tier:       in.Tier,
		Title:      index.DeriveTitle(in.Title, in.Body, id),
		Body:       in.Body,
		Supersedes: in.Supersedes,
		Surface:    e.surface,
	}, in.ExpectedVersion)
	if err != nil {
		return WriteOutput{}, err
	}
	return WriteOutput{ID: res.ID, Version: res.Version, Op: res.Op, Superseded: int(res.Superseded)}, nil
}

// DeleteInput is the memory_delete argument shape.
type DeleteInput struct {
	ID              string `json:"id" jsonschema:"The note id to delete (tombstone)."`
	ExpectedVersion int    `json:"expected_version" jsonschema:"The note's current version (from memory_get). Guards against deleting a note that changed under you."`
}

// MutationOutput is the result shape for delete/restore.
type MutationOutput struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Op      string `json:"op"` // delete | restore
}

// Delete tombstones a note: history is preserved (memory_restore can undo it) but
// it leaves live retrieval. Optimistic via expected_version.
func (e *Engine) Delete(ctx context.Context, in DeleteInput) (MutationOutput, error) {
	if e.readOnly {
		return MutationOutput{}, ErrReadOnly
	}
	if !e.limiter.Allow(e.tenantID) {
		return MutationOutput{}, ErrRateLimited
	}
	if in.ID == "" {
		return MutationOutput{}, fmt.Errorf("id is required")
	}
	res, err := e.store.DeleteNote(ctx, e.tenantID, in.ID, in.ExpectedVersion, e.surface)
	if err != nil {
		return MutationOutput{}, err
	}
	return MutationOutput{ID: res.ID, Version: res.Version, Op: res.Op}, nil
}

// RestoreInput is the memory_restore argument shape.
type RestoreInput struct {
	ID              string `json:"id" jsonschema:"The note id to restore."`
	TargetVersion   int    `json:"target_version,omitempty" jsonschema:"The history version to restore. Omit (or 0) for the most recent non-delete version — the usual undo."`
	ExpectedVersion int    `json:"expected_version" jsonschema:"The note's current version (from memory_get). Guards against restoring over a concurrent change."`
}

// Restore copies a prior version's content forward as a new version — the undo
// for an accidental delete or a bad overwrite.
func (e *Engine) Restore(ctx context.Context, in RestoreInput) (MutationOutput, error) {
	if e.readOnly {
		return MutationOutput{}, ErrReadOnly
	}
	if !e.limiter.Allow(e.tenantID) {
		return MutationOutput{}, ErrRateLimited
	}
	if in.ID == "" {
		return MutationOutput{}, fmt.Errorf("id is required")
	}
	res, err := e.store.RestoreNote(ctx, e.tenantID, in.ID, in.TargetVersion, in.ExpectedVersion, e.surface)
	if err != nil {
		return MutationOutput{}, err
	}
	return MutationOutput{ID: res.ID, Version: res.Version, Op: res.Op}, nil
}
