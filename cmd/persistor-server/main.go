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
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/config"
	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
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

	store := index.NewStore(pool, log)

	authn, err := buildAuth(ctx, &cfg, pool)
	if err != nil {
		return fmt.Errorf("building authenticator: %w", err)
	}

	httpServer, err := buildHTTPServer(&cfg, store, authn, log, pool.Ping)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{"addr": cfg.listenAddr, "auth_mode": cfg.authMode}).
		Warn("persistor-server listening (tailnet-bound)")
	return serve(ctx, httpServer, log)
}

// buildHTTPServer assembles the daemon's HTTP server: the auth-gated MCP mux
// (plus the OIDC consent page), wrapped in access logging and security headers,
// with timeouts suited to a long-running network service. ready is the /readyz
// DB probe.
func buildHTTPServer(cfg *serverConfig, store *index.Store, authn authBundle, log *logrus.Logger, ready func(context.Context) error) (*http.Server, error) {
	getServer := tenantServer(store, config.Version)
	mux := newMux(getServer, authn.verify, authn.opts, authn.metadata, ready)
	if cfg.authMode == authModeOIDC && cfg.stytchPublicToken != "" {
		consent, err := newConsentHandler(cfg.stytchPublicToken)
		if err != nil {
			return nil, fmt.Errorf("building consent page: %w", err)
		}
		mux.HandleFunc("/authorize", consent)
	}
	handler := requestLogger(log, securityHeaders(mux))
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
	authMode     string // static | oidc
	publicURL    string // externally-visible base URL (resource + metadata)
	oidcIssuer   string
	oidcAudience string
	oidcJWKSURL  string
	// stytchPublicToken, when set in oidc mode, serves the Stytch consent page
	// at /authorize (the OAuth Authorization URL). Publishable, not a secret.
	stytchPublicToken string
}

func loadConfig() (serverConfig, error) {
	cfg := serverConfig{
		databaseURL:       os.Getenv("DATABASE_URL"),
		listenAddr:        os.Getenv("PERSISTOR_LISTEN_ADDR"),
		authMode:          os.Getenv("PERSISTOR_AUTH_MODE"),
		publicURL:         os.Getenv("PERSISTOR_PUBLIC_URL"),
		oidcIssuer:        os.Getenv("PERSISTOR_OIDC_ISSUER"),
		oidcAudience:      os.Getenv("PERSISTOR_OIDC_AUDIENCE"),
		oidcJWKSURL:       os.Getenv("PERSISTOR_OIDC_JWKS_URL"),
		stytchPublicToken: os.Getenv("PERSISTOR_STYTCH_PUBLIC_TOKEN"),
	}
	if cfg.databaseURL == "" {
		return serverConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.listenAddr == "" {
		cfg.listenAddr = defaultListenAddr
	}
	if cfg.authMode == "" {
		cfg.authMode = authModeStatic
	}
	if cfg.publicURL == "" {
		cfg.publicURL = "http://" + cfg.listenAddr
	}
	cfg.publicURL = strings.TrimRight(cfg.publicURL, "/")
	if err := validateAuthConfig(&cfg); err != nil {
		return serverConfig{}, err
	}
	return cfg, nil
}

// validateAuthConfig enforces the requirements of the selected auth mode.
func validateAuthConfig(cfg *serverConfig) error {
	switch cfg.authMode {
	case authModeStatic:
		return nil
	case authModeOIDC:
		if cfg.oidcIssuer == "" || cfg.oidcAudience == "" || cfg.oidcJWKSURL == "" {
			return fmt.Errorf("oidc auth requires PERSISTOR_OIDC_ISSUER, PERSISTOR_OIDC_AUDIENCE, and PERSISTOR_OIDC_JWKS_URL")
		}
		if err := requireHTTPS("PERSISTOR_OIDC_ISSUER", cfg.oidcIssuer); err != nil {
			return err
		}
		if err := requireHTTPS("PERSISTOR_OIDC_JWKS_URL", cfg.oidcJWKSURL); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown PERSISTOR_AUTH_MODE %q (want %q or %q)", cfg.authMode, authModeStatic, authModeOIDC)
	}
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
