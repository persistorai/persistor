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
	"strings"
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

// Per-tenant write rate limit at the MCP boundary: a sustained writesPerSecond
// with a burst, applied to memory_write/delete/restore. Generous enough for any
// real interactive or import-via-MCP workload, low enough to blunt a runaway or
// prompt-injected agent.
const (
	writeRatePerSecond = 5
	writeRateBurst     = 20
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
	log.SetLevel(logrus.InfoLevel)

	pool, err := dbpool.NewPool(ctx, cfg.databaseURL, 8)
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

	httpServer, err := buildHTTPServer(&cfg, store, authn, log, pool.Ping)
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
func buildHTTPServer(cfg *serverConfig, store *index.Store, authn authBundle, log *logrus.Logger, ready func(context.Context) error) (*http.Server, error) {
	limiter := mcpengine.NewWriteLimiter(writeRatePerSecond, writeRateBurst)
	getServer := tenantServer(store, limiter, config.Version)
	mux := newMux(getServer, authn.verify, authn.opts, authn.metadata, ready)
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
		mux.HandleFunc("/register", regProxy)
	}
	handler := requestLogger(log, securityHeaders(contentLengthBuffer(mux)))
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

// serverConfig is the daemon's resolved environment configuration. There is no
// tenant here: the serving tenant is resolved per request from the bearer token.
type serverConfig struct {
	databaseURL  string
	listenAddr   string
	publicURL    string // externally-visible base URL (resource + metadata)
	oidcIssuer   string
	oidcAudience string
	oidcJWKSURL  string
	// stytchPublicToken, when set, serves the Stytch consent page at /authorize
	// (the OAuth Authorization URL). Publishable, not a secret.
	stytchPublicToken string
	// autoMigrate applies migrations at boot (default true, the single-role
	// self-host posture). Set PERSISTOR_AUTO_MIGRATE=false in production, where
	// migrations run separately as the owner and the daemon is a non-owner role.
	autoMigrate bool
}

func loadConfig() (serverConfig, error) {
	cfg := serverConfig{
		databaseURL:       os.Getenv("DATABASE_URL"),
		listenAddr:        os.Getenv("PERSISTOR_LISTEN_ADDR"),
		publicURL:         os.Getenv("PERSISTOR_PUBLIC_URL"),
		oidcIssuer:        os.Getenv("PERSISTOR_OIDC_ISSUER"),
		oidcAudience:      os.Getenv("PERSISTOR_OIDC_AUDIENCE"),
		oidcJWKSURL:       os.Getenv("PERSISTOR_OIDC_JWKS_URL"),
		stytchPublicToken: os.Getenv("PERSISTOR_STYTCH_PUBLIC_TOKEN"),
		// Default on: preserves the single-box self-host behavior. Only an
		// explicit "false" turns it off (the production split-role posture).
		autoMigrate: !strings.EqualFold(os.Getenv("PERSISTOR_AUTO_MIGRATE"), "false"),
	}
	if cfg.databaseURL == "" {
		return serverConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.listenAddr == "" {
		cfg.listenAddr = defaultListenAddr
	}
	if cfg.publicURL == "" {
		cfg.publicURL = "http://" + cfg.listenAddr
	}
	cfg.publicURL = strings.TrimRight(cfg.publicURL, "/")
	if err := validateOIDCConfig(&cfg); err != nil {
		return serverConfig{}, err
	}
	return cfg, nil
}

// validateOIDCConfig enforces that the OIDC requirements are met. OIDC is the
// only auth path, so the issuer, audience, and JWKS URL are mandatory and the
// issuer/JWKS must be HTTPS (or loopback for tests).
func validateOIDCConfig(cfg *serverConfig) error {
	if cfg.oidcIssuer == "" || cfg.oidcAudience == "" || cfg.oidcJWKSURL == "" {
		return fmt.Errorf("oidc auth requires PERSISTOR_OIDC_ISSUER, PERSISTOR_OIDC_AUDIENCE, and PERSISTOR_OIDC_JWKS_URL")
	}
	if err := requireHTTPS("PERSISTOR_OIDC_ISSUER", cfg.oidcIssuer); err != nil {
		return err
	}
	return requireHTTPS("PERSISTOR_OIDC_JWKS_URL", cfg.oidcJWKSURL)
}

// requireHTTPS rejects a non-HTTPS issuer/JWKS URL: over plain HTTP an on-path
// attacker could serve forged signing keys and mint accepted tokens. Loopback
// HTTP is allowed so tests can run a local JWKS server.
func requireHTTPS(name, raw string) error {
	if strings.HasPrefix(raw, "https://") ||
		strings.HasPrefix(raw, "http://127.0.0.1") ||
		strings.HasPrefix(raw, "http://localhost") ||
		strings.HasPrefix(raw, "http://[::1]") {
		return nil
	}
	return fmt.Errorf("%s must use https:// (got %q)", name, raw)
}
