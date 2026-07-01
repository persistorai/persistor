package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

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
	// logLevel is the logrus level name (default "info").
	logLevel string
	// dbMaxConns sizes the connection pool (default defaultDBMaxConns).
	dbMaxConns int32
	// trustedOrigins are browser origins allowed to make cross-origin requests to
	// /mcp past the CSRF/DNS-rebinding guard. Browser MCP clients (claude.ai web)
	// send Sec-Fetch-Site: cross-site, which the guard otherwise rejects; a trusted
	// origin is exempted. Non-browser clients (Claude Code) are unaffected either
	// way. Defaults to the Claude web app.
	trustedOrigins []string
	// originSecret, when set, locks the origin to the Cloudflare edge: every
	// route except /healthz requires a matching X-Origin-Secret header (injected
	// by a CF Transform Rule on proxied traffic), and the per-IP limiters switch
	// to keying on CF-Connecting-IP. Leave unset for self-host/tailnet binds.
	originSecret string
	// dcrEnabled mounts the unauthenticated /register DCR relay and advertises
	// registration_endpoint in the AS metadata facade (default true — claude.ai
	// web connectors register their OAuth client through it). Set
	// PERSISTOR_DCR_ENABLED=false to shut the relay off if it is being abused;
	// already-registered clients keep working, new connector setups fail.
	dcrEnabled bool
}

// defaultDBMaxConns is the pool size when PERSISTOR_DB_MAX_CONNS is unset.
const defaultDBMaxConns = 8

// defaultTrustedOrigin is the browser origin trusted for cross-origin /mcp
// requests when PERSISTOR_TRUSTED_ORIGINS is unset: the Claude web app.
const defaultTrustedOrigin = "https://claude.ai"

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
		autoMigrate:    !strings.EqualFold(os.Getenv("PERSISTOR_AUTO_MIGRATE"), "false"),
		logLevel:       os.Getenv("PERSISTOR_LOG_LEVEL"),
		trustedOrigins: parseTrustedOrigins(os.Getenv("PERSISTOR_TRUSTED_ORIGINS")),
		originSecret:   os.Getenv("PERSISTOR_ORIGIN_SECRET"),
		// Default on: DCR is how browser MCP clients onboard. Only an explicit
		// "false" turns the relay off (abuse response).
		dcrEnabled: !strings.EqualFold(os.Getenv("PERSISTOR_DCR_ENABLED"), "false"),
	}
	if cfg.logLevel == "" {
		cfg.logLevel = "info"
	}
	maxConns, err := parseMaxConns(os.Getenv("PERSISTOR_DB_MAX_CONNS"))
	if err != nil {
		return serverConfig{}, err
	}
	cfg.dbMaxConns = maxConns
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

// publicHTTPS reports whether the externally-visible URL is HTTPS, gating
// HSTS (set only on public TLS deployments, not the http tailnet bind).
func (c *serverConfig) publicHTTPS() bool {
	return strings.HasPrefix(c.publicURL, "https://")
}

// parseTrustedOrigins resolves the comma-separated browser origins allowed to
// make cross-origin /mcp requests, defaulting to the Claude web app when unset.
// Whitespace around entries is trimmed and empties are dropped.
func parseTrustedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{defaultTrustedOrigin}
	}
	var origins []string
	for o := range strings.SplitSeq(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// parseMaxConns resolves the pool size from PERSISTOR_DB_MAX_CONNS, defaulting to
// defaultDBMaxConns when unset and rejecting a non-positive or non-numeric value.
func parseMaxConns(raw string) (int32, error) {
	if raw == "" {
		return defaultDBMaxConns, nil
	}
	// maxDBMaxConns bounds the value so a fat-fingered setting can't request an
	// absurd pool. ParseInt with bitSize 32 guarantees the result fits int32.
	const maxDBMaxConns = 1000
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n <= 0 || n > maxDBMaxConns {
		return 0, fmt.Errorf("PERSISTOR_DB_MAX_CONNS must be an integer in 1..%d (got %q)", maxDBMaxConns, raw)
	}
	return int32(n), nil
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
// attacker could serve forged signing keys and mint accepted tokens. Plaintext
// loopback is allowed so tests can run a local JWKS server. The host is parsed
// and compared exactly — a prefix match would accept
// http://127.0.0.1.attacker.tld as "loopback". (PERSISTOR_PUBLIC_URL is not run
// through this: the tailnet deployment legitimately advertises http://<tailnet-ip>,
// with WireGuard providing transport encryption.)
func requireHTTPS(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL (%q): %w", name, raw, err)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("%s must use https:// (got %q)", name, raw)
}

// isLoopbackHost reports whether host is exactly a loopback name/address.
func isLoopbackHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}
