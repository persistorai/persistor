package index

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
)

const storeQueryTimeout = 30 * time.Second

// Store is tenant-scoped data access for the index
// (sources/notes/chunks). Every operation runs in a transaction with
// app.tenant_id set so Postgres row-level security isolates tenants.
type Store struct {
	pool *dbpool.Pool
	log  *logrus.Logger
}

// NewStore returns an index store over the given pool.
func NewStore(pool *dbpool.Pool, log *logrus.Logger) *Store {
	return &Store{pool: pool, log: log}
}

// IndexedFile is one file's fully-parsed, ready-to-write index payload.
type IndexedFile struct {
	Root    string
	RelPath string
	SHA256  string
	Note    Note
	Chunks  []string
}

// ListSourceHashes returns path -> sha256 for every indexed source in the
// tenant, the basis for incremental re-index diffing.
func (s *Store) ListSourceHashes(ctx context.Context, tenantID string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	out := make(map[string]string)
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT path, sha256 FROM sources WHERE tenant_id = current_setting('app.tenant_id')::uuid`)
		if err != nil {
			return fmt.Errorf("querying sources: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var path, sha string
			if err := rows.Scan(&path, &sha); err != nil {
				return fmt.Errorf("scanning source: %w", err)
			}
			out[path] = sha
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// IndexFile writes one file's source row, note, and chunks atomically. It is an
// upsert: any prior note/chunks for the same source path are replaced, so
// re-indexing a changed file fully refreshes its index entry.
func (s *Store) IndexFile(ctx context.Context, tenantID string, f *IndexedFile) error {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	return s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sources (tenant_id, path, root, sha256, last_indexed)
			 VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, NOW())
			 ON CONFLICT (tenant_id, path)
			 DO UPDATE SET root = EXCLUDED.root, sha256 = EXCLUDED.sha256, last_indexed = NOW()`,
			f.RelPath, f.Root, f.SHA256); err != nil {
			return fmt.Errorf("upserting source: %w", err)
		}

		if err := deleteNotesForPath(ctx, tx, f.RelPath); err != nil {
			return err
		}

		n := f.Note
		if _, err := tx.Exec(ctx,
			`INSERT INTO notes (id, tenant_id, kind, tier, title, body, source_path, supersedes)
			 VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, NULLIF($7, ''))
			 ON CONFLICT (tenant_id, id)
			 DO UPDATE SET kind = EXCLUDED.kind, tier = EXCLUDED.tier, title = EXCLUDED.title,
			               body = EXCLUDED.body, source_path = EXCLUDED.source_path,
			               supersedes = EXCLUDED.supersedes, superseded = FALSE`,
			n.ID, n.Kind, n.Tier, n.Title, n.Body, n.SourcePath, n.Supersedes); err != nil {
			return fmt.Errorf("upserting note: %w", err)
		}

		if len(f.Chunks) > 0 {
			batch := &pgx.Batch{}
			for ord, text := range f.Chunks {
				batch.Queue(
					`INSERT INTO chunks (note_id, tenant_id, ord, text)
					 VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3)`,
					n.ID, ord, text)
			}
			if err := tx.SendBatch(ctx, batch).Close(); err != nil {
				return fmt.Errorf("inserting chunks: %w", err)
			}
		}
		return nil
	})
}

// DeleteByPath removes a file's source row, note, and chunks — used when a
// watched file disappears.
func (s *Store) DeleteByPath(ctx context.Context, tenantID, relPath string) error {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	return s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		if err := deleteNotesForPath(ctx, tx, relPath); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM sources
			 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND path = $1`, relPath); err != nil {
			return fmt.Errorf("deleting source: %w", err)
		}
		return nil
	})
}

// CountNotes returns the number of indexed notes for the tenant.
func (s *Store) CountNotes(ctx context.Context, tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var n int
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM notes WHERE tenant_id = current_setting('app.tenant_id')::uuid`).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("counting notes: %w", err)
	}
	return n, nil
}

// deleteNotesForPath drops the note(s) and their chunks backed by relPath.
func deleteNotesForPath(ctx context.Context, tx pgx.Tx, relPath string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM chunks
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND note_id IN (
		     SELECT id FROM notes
		     WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source_path = $1)`,
		relPath); err != nil {
		return fmt.Errorf("deleting chunks for path: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM notes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source_path = $1`,
		relPath); err != nil {
		return fmt.Errorf("deleting notes for path: %w", err)
	}
	return nil
}

// inTx runs fn in a read-write transaction with the tenant context set.
func (s *Store) inTx(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer s.rollback(ctx, tx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("setting tenant context: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// inReadTx runs fn in a read-only transaction with the tenant context set.
func (s *Store) inReadTx(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}
	tx, err := s.pool.BeginReadOnly(ctx)
	if err != nil {
		return fmt.Errorf("beginning read transaction: %w", err)
	}
	defer s.rollback(ctx, tx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("setting tenant context: %w", err)
	}
	return fn(tx)
}

// rollback aborts a transaction, ignoring the expected "already committed" case
// and logging any other failure. Safe to defer after a successful commit.
func (s *Store) rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		s.log.WithError(err).Warn("transaction rollback")
	}
}
