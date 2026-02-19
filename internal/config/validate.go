package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (c *Config) validate() error {
	if err := c.validateDatabase(); err != nil {
		return err
	}

	if err := c.validateNetwork(); err != nil {
		return err
	}

	if err := c.validateOllama(); err != nil {
		return err
	}

	if err := c.validateCORS(); err != nil {
		return err
	}

	if err := c.validateEncryption(); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateDatabase() error {
	if c.DatabaseURL.Value() == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	dbURL, err := url.Parse(c.DatabaseURL.Value())
	if err != nil {
		return fmt.Errorf("DATABASE_URL is not a valid URL: %w", err)
	}

	if dbURL.Scheme != "postgres" && dbURL.Scheme != "postgresql" {
		return fmt.Errorf("DATABASE_URL scheme must be postgres:// or postgresql://")
	}

	if dbURL.Hostname() == "" {
		return fmt.Errorf("DATABASE_URL must include a host")
	}

	dbHost := dbURL.Hostname()
	if dbHost != "localhost" && dbHost != "127.0.0.1" && dbHost != "::1" {
		sslmode := dbURL.Query().Get("sslmode")
		if sslmode == "disable" {
			return fmt.Errorf("DATABASE_URL sslmode=disable is not allowed for non-local host %q", dbHost)
		}
	}

	return nil
}

func (c *Config) validateNetwork() error {
	port, err := strconv.Atoi(c.Port)
	if err != nil {
		return fmt.Errorf("PORT must be a valid integer: %w", err)
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535")
	}

	// Validate LISTEN_HOST is a known-safe address. Allow loopback addresses for
	// local deployments and 0.0.0.0/:: for containerized deployments where the
	// network boundary is enforced externally (e.g. Docker, Kubernetes).
	validHosts := map[string]bool{
		"127.0.0.1": true,
		"::1":       true,
		"localhost": true,
		"0.0.0.0":   true,
		"::":        true,
	}
	if !validHosts[c.ListenHost] {
		return fmt.Errorf("LISTEN_HOST must be a loopback address or 0.0.0.0/:: for containers (got %q)", c.ListenHost)
	}

	metricsPort, err := strconv.Atoi(c.MetricsPort)
	if err != nil {
		return fmt.Errorf("METRICS_PORT must be a valid integer: %w", err)
	}

	if metricsPort < 1 || metricsPort > 65535 {
		return fmt.Errorf("METRICS_PORT must be between 1 and 65535")
	}

	if metricsPort == port {
		return fmt.Errorf("METRICS_PORT must differ from PORT")
	}

	return nil
}

func (c *Config) validateOllama() error {
	ollamaURL, err := url.ParseRequestURI(c.OllamaURL)
	if err != nil {
		return fmt.Errorf("OLLAMA_URL is not a valid URL: %w", err)
	}

	ollamaHost := ollamaURL.Hostname()
	if ollamaHost != "localhost" && ollamaHost != "127.0.0.1" && ollamaHost != "::1" {
		if !c.OllamaAllowRemote {
			return fmt.Errorf("OLLAMA_URL must point to localhost (set OLLAMA_ALLOW_REMOTE=true for distributed deployments)")
		}
	}

	return nil
}

func (c *Config) validateCORS() error {
	for _, origin := range c.CORSOrigins {
		if origin == "*" {
			return fmt.Errorf("CORS_ORIGINS must not contain wildcard '*'")
		}
		if strings.ContainsAny(origin, "*?[]") {
			return fmt.Errorf("CORS_ORIGINS must not contain glob characters (*?[]), got %q", origin)
		}
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("CORS_ORIGINS contains invalid origin %q (must have scheme and host)", origin)
		}
	}

	return nil
}

func (c *Config) validateEncryption() error {
	switch c.EncryptionProvider {
	case "static":
		if c.EncryptionKey.Value() == "" {
			return fmt.Errorf("ENCRYPTION_KEY is required when ENCRYPTION_PROVIDER is static")
		}

		keyBytes, err := hex.DecodeString(c.EncryptionKey.Value())
		if err != nil {
			return fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
		}

		if len(keyBytes) != 32 {
			return fmt.Errorf("ENCRYPTION_KEY must be 64 hex characters (32 bytes), got %d chars", len(c.EncryptionKey.Value()))
		}
	case "vault":
		if c.VaultToken.Value() == "" {
			return fmt.Errorf("VAULT_TOKEN is required when ENCRYPTION_PROVIDER is vault")
		}

		if !isLocalhost(c.VaultAddr) && !strings.HasPrefix(c.VaultAddr, "https://") {
			return fmt.Errorf("VAULT_ADDR must use HTTPS for non-localhost connections")
		}
	default:
		return fmt.Errorf("ENCRYPTION_PROVIDER must be 'static' or 'vault', got %q", c.EncryptionProvider)
	}

	return nil
}

// isLocalhost returns true if the given address points to a loopback address.
func isLocalhost(addr string) bool {
	u, err := url.Parse(addr)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
