// persistor-mcp is an MCP server (stdio transport) exposing the memory engine to
// MCP clients such as Claude Code. It connects straight to the local Postgres
// full-text index and serves four tools: memory_search, memory_get,
// memory_write, brief.
//
// Configuration is by environment (shared with the persistor CLI):
//
//	DATABASE_URL          Postgres URL (required)
//	PERSISTOR_TENANT_ID   tenant UUID (required)
//	PERSISTOR_NOTES_DIR   notes repo root (required — watched root + write target)
//	CLAUDE_MEMORY_DIR     Claude auto-memory dir (optional, archived + indexed)
//	PERSISTOR_WRITE_DIR   where memory_write writes (default <PERSISTOR_NOTES_DIR>/memory/atomic)
//
// Register in ~/.claude.json:
//
//	{ "mcpServers": { "persistor": { "command": "persistor-mcp" } } }
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/config"
	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "persistor-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	databaseURL := os.Getenv("DATABASE_URL")
	tenantID := os.Getenv("PERSISTOR_TENANT_ID")
	notesDir := os.Getenv("PERSISTOR_NOTES_DIR")
	if databaseURL == "" || tenantID == "" || notesDir == "" {
		return fmt.Errorf("DATABASE_URL, PERSISTOR_TENANT_ID, and PERSISTOR_NOTES_DIR are required")
	}
	writeDir := os.Getenv("PERSISTOR_WRITE_DIR")
	if writeDir == "" {
		writeDir = DefaultWriteDir(notesDir)
	}

	log := logrus.New()
	log.SetOutput(os.Stderr) // stdout is the MCP transport — keep logs off it
	log.SetLevel(logrus.WarnLevel)

	pool, err := dbpool.NewPool(ctx, databaseURL, 4)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	roots, err := index.BuildRoots(notesDir, os.Getenv("CLAUDE_MEMORY_DIR"), "")
	if err != nil {
		return fmt.Errorf("building roots: %w", err)
	}

	store := index.NewStore(pool, log)
	indexer := index.NewIndexer(store, log, 0)
	engine := NewEngine(store, indexer, tenantID, roots, writeDir)

	server := mcp.NewServer(
		&mcp.Implementation{Name: "persistor", Title: "Persistor Memory", Version: config.Version},
		&mcp.ServerOptions{Instructions: serverInstructions},
	)
	registerTools(server, engine)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serving: %w", err)
	}
	return nil
}
