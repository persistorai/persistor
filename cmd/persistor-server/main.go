// persistor-server is the remote MCP daemon: it exposes the same memory tools as
// persistor-mcp (memory_search, memory_get, memory_write, brief) over the MCP
// Streamable HTTP transport, so Claude Code / claude.ai / the mobile app can
// connect over the network rather than stdio.
//
// This is the deliberate shift from "Persistor is not a daemon": the remote
// server IS long-running. The stdio server + CLI remain for local use.
//
// Phase P1 binds to the tailnet only and has NO auth — set PERSISTOR_LISTEN_ADDR
// to the Tailscale IP, never a public interface. Auth (P2: static bearer tokens;
// P3: OIDC) lands as middleware in front of newMux. Configuration is by
// environment (shared with persistor-mcp), plus:
//
//	PERSISTOR_LISTEN_ADDR   host:port to bind (default 127.0.0.1:8088; set the
//	                        Tailscale IP to reach it from another device)
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/config"
	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
	"github.com/briancolinger/persistor/internal/mcpengine"
)

const defaultListenAddr = "127.0.0.1:8088"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "persistor-server: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	log := logrus.New()
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.InfoLevel)

	pool, err := dbpool.NewPool(ctx, cfg.databaseURL, 8)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	roots, err := index.BuildRoots(cfg.notesDir, cfg.claudeMemoryDir, "")
	if err != nil {
		return fmt.Errorf("building roots: %w", err)
	}

	store := index.NewStore(pool, log)
	indexer := index.NewIndexer(store, log, 0)
	engine := mcpengine.NewEngine(store, indexer, cfg.tenantID, roots, cfg.writeDir)
	server := mcpengine.NewServer(engine, config.Version)

	httpServer := &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           newMux(server),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.WithField("addr", cfg.listenAddr).Warn("persistor-server listening (tailnet-bound, no auth — do not expose publicly)")
	return serve(ctx, httpServer, log)
}

// serve runs the HTTP server until ctx is cancelled (SIGINT/SIGTERM), then
// drains in-flight requests with a bounded timeout.
func serve(ctx context.Context, srv *http.Server, log *logrus.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}

// serverConfig is the daemon's resolved environment configuration.
type serverConfig struct {
	databaseURL     string
	tenantID        string
	notesDir        string
	claudeMemoryDir string
	writeDir        string
	listenAddr      string
}

func loadConfig() (serverConfig, error) {
	cfg := serverConfig{
		databaseURL:     os.Getenv("DATABASE_URL"),
		tenantID:        os.Getenv("PERSISTOR_TENANT_ID"),
		notesDir:        os.Getenv("PERSISTOR_NOTES_DIR"),
		claudeMemoryDir: os.Getenv("CLAUDE_MEMORY_DIR"),
		writeDir:        os.Getenv("PERSISTOR_WRITE_DIR"),
		listenAddr:      os.Getenv("PERSISTOR_LISTEN_ADDR"),
	}
	if cfg.databaseURL == "" || cfg.tenantID == "" || cfg.notesDir == "" {
		return serverConfig{}, fmt.Errorf("DATABASE_URL, PERSISTOR_TENANT_ID, and PERSISTOR_NOTES_DIR are required")
	}
	if cfg.writeDir == "" {
		cfg.writeDir = mcpengine.DefaultWriteDir(cfg.notesDir)
	}
	if cfg.listenAddr == "" {
		cfg.listenAddr = defaultListenAddr
	}
	return cfg, nil
}
