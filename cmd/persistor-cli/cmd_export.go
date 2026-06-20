package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

// exportOpts holds the resolved inputs for `persistor export`.
type exportOpts struct {
	databaseURL string
	tenantID    string
	outDir      string
}

// newExportCmd builds `persistor export`: write every current note for a tenant
// as a .md file (frontmatter + body). The export is portable markdown you can
// git-track on a trusted box and re-import; it is the Model 1 backup story (the
// at-rest bytes live in a Postgres row, but stay plain prose).
func newExportCmd() *cobra.Command {
	o := exportOpts{}
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a tenant's notes to .md files",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runExport(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.outDir, "out", "", "Output directory (created if absent)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func runExport(ctx context.Context, o *exportOpts) error {
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	if _, err := uuid.Parse(o.tenantID); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", o.tenantID, err)
	}
	if o.outDir == "" {
		return fmt.Errorf("no output directory (set --out)")
	}

	databaseURL := o.databaseURL
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, databaseURL, 2)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	store := index.NewStore(pool, log)
	notes, err := store.ExportNotes(ctx, o.tenantID)
	if err != nil {
		return err
	}

	for i := range notes {
		n := &notes[i]
		rel, err := exportRelPath(n)
		if err != nil {
			return err
		}
		content, err := index.RenderNote(n)
		if err != nil {
			return fmt.Errorf("rendering %q: %w", n.ID, err)
		}
		dest := filepath.Join(o.outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return fmt.Errorf("creating dir for %q: %w", dest, err)
		}
		// 0600: an export is a backup of private memory; keep it owner-only.
		if err := os.WriteFile(dest, []byte(content), 0o600); err != nil {
			return fmt.Errorf("writing %q: %w", dest, err)
		}
	}
	fmt.Fprintf(os.Stderr, "exported %d note(s) to %s\n", len(notes), o.outDir)
	return nil
}

// exportRelPath picks a safe, relative .md path for a note: its source_path when
// present and well-behaved, else a filename derived from the id. It rejects any
// path that would escape the output directory (absolute or containing "..").
func exportRelPath(n *index.Note) (string, error) {
	rel := n.SourcePath
	if rel == "" {
		rel = strings.ReplaceAll(strings.ReplaceAll(n.ID, ":", "-"), "/", "-") + ".md"
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe note path %q (from id %q)", rel, n.ID)
	}
	if !strings.HasSuffix(clean, ".md") {
		clean += ".md"
	}
	return clean, nil
}
