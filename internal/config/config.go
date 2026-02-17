// Package config provides environment-driven configuration for the persistor.
package config

import (
	"fmt"
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
	EmbeddingDimensions int
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

	embeddingDims, err := strconv.Atoi(envOrDefault("EMBEDDING_DIMENSIONS", "1024"))
	if err != nil || embeddingDims < 1 || embeddingDims > 4096 {
		return nil, fmt.Errorf("EMBEDDING_DIMENSIONS must be an integer between 1 and 4096")
	}
	cfg.EmbeddingDimensions = embeddingDims

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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
