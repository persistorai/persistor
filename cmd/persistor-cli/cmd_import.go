package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// importOpts holds the resolved inputs for `persistor import`.
type importOpts struct {
	databaseURL string
	tenantID    string
	namespace   string
}

// newImportCmd builds `persistor import <path>`: bulk-load a directory (or single
// file) of .md notes into a tenant's memory. It is source-agnostic — the same
// command imports Scout's notes, Claude's auto-memory, or any markdown corpus —
// and idempotent: re-running it updates notes in place rather than failing. It
// talks to Postgres directly (operator tool, no bearer auth).
func newImportCmd() *cobra.Command {
	o := importOpts{}
	cmd := &cobra.Command{
		Use:   "import <path>",
		Short: "Bulk-import .md notes into a tenant/namespace",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runImport(cmd.Context(), &o, args[0]) },
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.namespace, "namespace", "default", "Namespace to import the notes into (e.g. scout, claude)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func runImport(ctx context.Context, o *importOpts, path string) error {
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	if _, err := uuid.Parse(o.tenantID); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", o.tenantID, err)
	}
	if o.namespace == "" {
		o.namespace = "default"
	}
	if err := index.ValidateNamespace(o.namespace); err != nil {
		return err
	}
	databaseURL := o.databaseURL
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}

	files, err := collectMarkdown(path)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no .md files found under %q", path)
	}

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, databaseURL, 4)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	store := index.NewStore(pool, log)

	created, updated, err := importAll(ctx, store, o.tenantID, o.namespace, files)
	if err != nil {
		return err
	}

	// Reconcile supersession once across the whole tenant after the bulk write.
	if _, err := store.ReconcileSupersessions(ctx, o.tenantID); err != nil {
		return fmt.Errorf("reconciling supersessions: %w", err)
	}

	fmt.Fprintf(os.Stderr, "imported %d note(s) into namespace %q (%d created, %d updated)\n",
		len(files), o.namespace, created, updated)
	return nil
}

// importAll writes every file's parsed note into the tenant/namespace, returning
// how many were created vs updated. It is idempotent: an existing note is updated
// at its current version rather than colliding with a create.
func importAll(ctx context.Context, store *index.Store, tenantID, namespace string, files []importFile) (created, updated int, err error) {
	for _, f := range files {
		content, err := os.ReadFile(f.abs)
		if err != nil {
			return 0, 0, fmt.Errorf("reading %q: %w", f.abs, err)
		}
		// ParseNote derives the id (frontmatter id, else <namespace>:<slug(rel)>),
		// kind/tier, title, and supersedes — the same parse the file indexer used.
		n := index.ParseNote(namespace, f.rel, string(content), false)
		// A frontmatter id: bypasses DeriveNoteID, so enforce the same charset/
		// length rules here that the MCP write path applies.
		if err := index.ValidateNoteID(n.ID); err != nil {
			return 0, 0, fmt.Errorf("importing %q: %w", f.rel, err)
		}

		expected := 0
		st, found, err := store.NoteState(ctx, tenantID, n.ID)
		if err != nil {
			return 0, 0, fmt.Errorf("checking %q: %w", n.ID, err)
		}
		if found {
			expected = st.Version
		}
		res, err := store.WriteNote(ctx, tenantID, &index.PGNoteInput{
			ID: n.ID, Namespace: namespace, Kind: n.Kind, Tier: n.Tier,
			Title: n.Title, Body: n.Body, Supersedes: n.Supersedes, Surface: "import",
		}, expected)
		if err != nil {
			return 0, 0, fmt.Errorf("importing %q: %w", n.ID, err)
		}
		if res.Op == "create" {
			created++
		} else {
			updated++
		}
	}
	return created, updated, nil
}

// importFile is one markdown file to import: its absolute path and the relative
// path used to derive a stable note id.
type importFile struct {
	abs string
	rel string
}

// collectMarkdown returns the .md files under path. A single .md file is imported
// directly (rel = base name); a directory is walked recursively (rel = path
// relative to the directory, so ids stay stable across runs).
func collectMarkdown(path string) ([]importFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.IsDir() {
		if !strings.HasSuffix(path, ".md") {
			return nil, fmt.Errorf("%q is not a .md file", path)
		}
		return []importFile{{abs: path, rel: filepath.Base(path)}}, nil
	}

	var out []importFile
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		out = append(out, importFile{abs: p, rel: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %q: %w", path, err)
	}
	return out, nil
}
