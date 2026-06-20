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

	"github.com/persistorai/persistor/internal/config"
	"github.com/persistorai/persistor/internal/db"
	"github.com/persistorai/persistor/internal/db/migrations"
	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
	"github.com/persistorai/persistor/internal/mcpengine"
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
	if databaseURL == "" || tenantID == "" {
		return fmt.Errorf("DATABASE_URL and PERSISTOR_TENANT_ID are required")
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

	store := index.NewStore(pool, log)
	engine := mcpengine.NewEngine(store, tenantID, mcpengine.WithSurface("local-stdio"))

	server := mcpengine.NewServer(engine, config.Version)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serving: %w", err)
	}
	return nil
}
