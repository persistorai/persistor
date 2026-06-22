// persistor-server is the MCP daemon: it exposes the memory tools
// (memory_search, memory_get, memory_write, memory_delete, memory_restore,
// brief) over the MCP Streamable HTTP transport. It is the ONLY way a client
// reaches Persistor's memory — there is no stdio binary. Local and remote clients
// alike are normal MCP clients of a (local or remote) persistor-server.
//
// Auth is OIDC, always: clients obtain a token through the browser OAuth flow and
// the daemon validates it against the IdP's JWKS, deriving a stable tenant from
// iss|sub so the same identity maps to the same tenant on every device.
//
// Configuration is by environment:
//
//	DATABASE_URL            Postgres URL (required)
//	PERSISTOR_LISTEN_ADDR   host:port to bind (default 127.0.0.1:8088; set the
//	                        Tailscale IP to reach it from another device)
//	PERSISTOR_OIDC_ISSUER / _AUDIENCE / _JWKS_URL   OIDC validation (required)
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

	"github.com/persistorai/persistor/internal/config"
	"github.com/persistorai/persistor/internal/db"
	"github.com/persistorai/persistor/internal/db/migrations"
	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
	"github.com/persistorai/persistor/internal/mcpengine"
)

const defaultListenAddr = "127.0.0.1:8088"

// Per-tenant rate limits at the MCP boundary. Writes (memory_write/delete/
// restore) get a tighter cap that blunts a runaway or prompt-injected agent;
// reads (search/get/list/namespaces/brief) get a looser cap that still bounds
// unbounded FTS/assembly per tenant. Both are generous for real interactive or
// import-via-MCP workloads.
const (
	writeRatePerSecond = 5
	writeRateBurst     = 20
	readRatePerSecond  = 30
	readRateBurst      = 60
)

// maxMCPBodyBytes caps the /mcp request body. 4 MiB fits a max-size note (the DB
// bounds the body at 1 MiB) plus JSON-RPC framing, while stopping a single
// authenticated tenant from exhausting memory with one giant request.
const maxMCPBodyBytes = 4 << 20

// Per-client-IP rate limit for the open /register DCR proxy: it relays to the
// IdP unauthenticated, so cap how fast any one IP can drive registrations.
const (
	registerRatePerSecond = 1
	registerRateBurst     = 5
)

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
	level, err := logrus.ParseLevel(cfg.logLevel)
	if err != nil {
		return fmt.Errorf("invalid PERSISTOR_LOG_LEVEL %q: %w", cfg.logLevel, err)
	}
	log.SetLevel(level)

	pool, err := dbpool.NewPool(ctx, cfg.databaseURL, cfg.dbMaxConns)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := applySchema(ctx, &cfg, pool, log); err != nil {
		return err
	}

	store := index.NewStore(pool, log)

	authn, err := buildAuth(ctx, &cfg, pool)
	if err != nil {
		return fmt.Errorf("building authenticator: %w", err)
	}

	httpServer, err := buildHTTPServer(&cfg, store, authn, log, pool.Ping, pool.Stat)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{"addr": cfg.listenAddr, "auth": "oidc"}).
		Warn("persistor-server listening (tailnet-bound)")
	return serve(ctx, httpServer, log)
}

// applySchema brings the database schema into the state the daemon needs.
//
// With auto-migrate on (PERSISTOR_AUTO_MIGRATE!=false, the default — the
// single-role self-host posture) the daemon applies migrations at boot, which
// requires it to connect as the schema owner.
//
// With auto-migrate off (the production posture) migrations are an explicit,
// separately-run `persistor migrate` step as the schema-owning migrator, and the
// daemon connects as a non-owner least-privilege app role. It cannot run DDL, so
// it instead (1) refuses to serve a schema with pending migrations and (2)
// asserts it is not the table owner, enforcing the role split that protects the
// append-only audit log.
func applySchema(ctx context.Context, cfg *serverConfig, pool *dbpool.Pool, log *logrus.Logger) error {
	if cfg.autoMigrate {
		if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
			return fmt.Errorf("applying migrations: %w", err)
		}
		return nil
	}

	pending, err := db.MigrationsPending(ctx, pool, migrations.FS)
	if err != nil {
		return fmt.Errorf("checking schema version: %w", err)
	}
	if pending {
		return fmt.Errorf("database has pending migrations and PERSISTOR_AUTO_MIGRATE is off; " +
			"run `persistor migrate` as the schema-owning migrator role before starting the daemon")
	}
	if err := pool.AssertNonOwner(ctx); err != nil {
		return err
	}
	return nil
}

// buildHTTPServer assembles the daemon's HTTP server: the auth-gated MCP mux
// (plus the OIDC consent page), wrapped in access logging and security headers,
// with timeouts suited to a long-running network service. ready is the /readyz
// DB probe.
func buildHTTPServer(cfg *serverConfig, store *index.Store, authn authBundle, log *logrus.Logger, ready func(context.Context) error, poolStat func() dbpool.Stat) (*http.Server, error) {
	writeLimiter := mcpengine.NewKeyLimiter(writeRatePerSecond, writeRateBurst)
	readLimiter := mcpengine.NewKeyLimiter(readRatePerSecond, readRateBurst)
	getServer := tenantServer(store, writeLimiter, readLimiter, config.Version)
	mux := newMux(getServer, authn.verify, authn.opts, authn.metadata, ready)
	m := newMetrics(poolStat)
	mux.HandleFunc("/metrics", m.serveHTTP)
	if cfg.stytchPublicToken != "" {
		consent, err := newConsentHandler(cfg.stytchPublicToken)
		if err != nil {
			return nil, fmt.Errorf("building consent page: %w", err)
		}
		mux.HandleFunc("/authorize", consent)
		// Authorization-server facade for MCP clients that perform OAuth discovery
		// at the resource origin instead of following the cross-origin
		// authorization_servers pointer (claude.ai web). Only meaningful alongside
		// the consent page, which is the facade's authorization_endpoint. The
		// same-origin /register proxy bridges DCR to the IdP.
		asMeta := newAuthServerMetadata(cfg.publicURL, cfg.oidcIssuer, cfg.oidcJWKSURL)
		mux.HandleFunc("/.well-known/oauth-authorization-server", asMeta.serveHTTP)
		mux.HandleFunc("/.well-known/openid-configuration", asMeta.serveHTTP)
		regProxy := newRegisterProxy(
			&http.Client{Timeout: 10 * time.Second},
			stytchEndpoint(cfg.oidcIssuer, "/v1/oauth2/register"),
		)
		regLimiter := mcpengine.NewKeyLimiter(registerRatePerSecond, registerRateBurst)
		mux.Handle("/register", perIPLimit(regLimiter, regProxy))
	}
	handler := observe(log, m, securityHeaders(contentLengthBuffer(mux)))
	return &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: MCP Streamable HTTP responses can be long-lived
		// (SSE-style streaming); a write deadline would truncate them.
	}, nil
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
