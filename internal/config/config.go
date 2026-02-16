// Package config provides environment-driven configuration for the persistor.
package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Secret wraps a sensitive string to prevent accidental logging or marshalling.
type Secret string

// String implements fmt.Stringer, returning a redacted placeholder.
func (s Secret) String() string { return "[REDACTED]" }

// GoString implements fmt.GoStringer, returning a redacted placeholder.
func (s Secret) GoString() string { return "[REDACTED]" }

// MarshalText implements encoding.TextMarshaler, returning a redacted placeholder.
func (s Secret) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }

// Value returns the underlying secret string.
func (s Secret) Value() string { return string(s) }

// Config holds all application configuration values.
type Config struct {
	DatabaseURL        Secret
	Port               string
	ListenHost         string
	CORSOrigins        []string
	OllamaURL          string
	EmbeddingModel     string
	LogLevel           string
	EncryptionProvider string
	EncryptionKey      Secret
	VaultAddr          string
	VaultToken         Secret
	EmbedWorkers       int
	EnablePlayground   bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        Secret(envOrDefault("DATABASE_URL", "")),
		Port:               envOrDefault("PORT", "3030"),
		ListenHost:         envOrDefault("LISTEN_HOST", "127.0.0.1"),
		OllamaURL:          envOrDefault("OLLAMA_URL", "http://localhost:11434"),
		EmbeddingModel:     envOrDefault("EMBEDDING_MODEL", "qwen3-embedding:0.6b"),
		LogLevel:           envOrDefault("LOG_LEVEL", "info"),
		EncryptionProvider: envOrDefault("ENCRYPTION_PROVIDER", "static"),
		EncryptionKey:      Secret(envOrDefault("ENCRYPTION_KEY", "")),
		VaultAddr:          envOrDefault("VAULT_ADDR", "http://127.0.0.1:8200"),
		VaultToken:         Secret(envOrDefault("VAULT_TOKEN", "")),
		EnablePlayground:   envOrDefault("ENABLE_PLAYGROUND", "false") == "true",
	}

	embedWorkers, err := strconv.Atoi(envOrDefault("EMBED_WORKERS", "4"))
	if err != nil || embedWorkers < 1 || embedWorkers > 16 {
		return nil, fmt.Errorf("EMBED_WORKERS must be an integer between 1 and 16")
	}
	cfg.EmbedWorkers = embedWorkers

	origins := envOrDefault("CORS_ORIGINS", "http://localhost:3002")
	cfg.CORSOrigins = strings.Split(origins, ",")

	for i, o := range cfg.CORSOrigins {
		cfg.CORSOrigins[i] = strings.TrimSpace(o)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Addr returns the listen address in host:port format.
func (c *Config) Addr() string {
	return c.ListenHost + ":" + c.Port
}

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

	// Validate LISTEN_HOST is a loopback address to prevent accidental external exposure.
	if c.ListenHost != "127.0.0.1" && c.ListenHost != "::1" && c.ListenHost != "localhost" {
		return fmt.Errorf("LISTEN_HOST must be a loopback address (127.0.0.1, ::1, or localhost), got %q", c.ListenHost)
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
		return fmt.Errorf("OLLAMA_URL must point to localhost (127.0.0.1, ::1, or localhost)")
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
