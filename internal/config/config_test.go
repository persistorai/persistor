package config_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/config"
)

func validKey() string {
	return hex.EncodeToString(make([]byte, 32))
}

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb")
	t.Setenv("ENCRYPTION_PROVIDER", "static")
	t.Setenv("ENCRYPTION_KEY", validKey())
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
}

func TestLoad_ValidConfig(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != "3030" {
		t.Errorf("expected default port 3030, got %s", cfg.Port)
	}

	if cfg.ListenHost != "127.0.0.1" {
		t.Errorf("expected default listen host 127.0.0.1, got %s", cfg.ListenHost)
	}

	if cfg.EmbedWorkers != 4 {
		t.Errorf("expected default embed workers 4, got %d", cfg.EmbedWorkers)
	}

	if cfg.DBMaxConns != 21 {
		t.Errorf("expected default DB_MAX_CONNS 21, got %d", cfg.DBMaxConns)
	}

	if cfg.Addr() != "127.0.0.1:3030" {
		t.Errorf("expected addr 127.0.0.1:3030, got %s", cfg.Addr())
	}

	if cfg.MetricsPort != "9091" {
		t.Errorf("expected default metrics port 9091, got %s", cfg.MetricsPort)
	}

	if cfg.MetricsAddr() != "127.0.0.1:9091" {
		t.Errorf("expected metrics addr 127.0.0.1:9091, got %s", cfg.MetricsAddr())
	}
}

func TestLoad_Defaults(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("unexpected OllamaURL default: %s", cfg.OllamaURL)
	}

	if cfg.EmbeddingModel != "qwen3-embedding:0.6b" {
		t.Errorf("unexpected EmbeddingModel default: %s", cfg.EmbeddingModel)
	}

	if cfg.EmbeddingDimensions != 1024 {
		t.Errorf("unexpected EmbeddingDimensions default: %d", cfg.EmbeddingDimensions)
	}

	if cfg.EnablePlayground {
		t.Error("expected EnablePlayground=false by default")
	}
}

func TestLoad_ErrorCases(t *testing.T) {
	tests := []struct {
		name         string
		envOverrides map[string]string
		envClear     []string
		wantErr      string
	}{
		{
			name:     "missing DATABASE_URL",
			envClear: []string{"DATABASE_URL"},
			wantErr:  "DATABASE_URL is required",
		},
		{
			name:         "invalid PORT zero",
			envOverrides: map[string]string{"PORT": "0"},
			wantErr:      "PORT must be between 1 and 65535",
		},
		{
			name:         "invalid PORT too high",
			envOverrides: map[string]string{"PORT": "99999"},
			wantErr:      "PORT must be between 1 and 65535",
		},
		{
			name:         "invalid PORT non-numeric",
			envOverrides: map[string]string{"PORT": "abc"},
			wantErr:      "PORT must be a valid integer",
		},
		{
			name:         "invalid LISTEN_HOST",
			envOverrides: map[string]string{"LISTEN_HOST": "192.168.1.1"},
			wantErr:      "LISTEN_HOST must be a loopback address or 0.0.0.0/:: for containers",
		},
		{
			name:         "CORS wildcard",
			envOverrides: map[string]string{"CORS_ORIGINS": "*"},
			wantErr:      "CORS_ORIGINS must not contain wildcard",
		},
		{
			name:         "CORS invalid origin",
			envOverrides: map[string]string{"CORS_ORIGINS": "not-a-url"},
			wantErr:      "CORS_ORIGINS contains invalid origin",
		},
		{
			name:         "vault provider without token",
			envOverrides: map[string]string{"ENCRYPTION_PROVIDER": "vault"},
			envClear:     []string{"ENCRYPTION_KEY", "VAULT_TOKEN"},
			wantErr:      "VAULT_TOKEN is required",
		},
		{
			name:         "static provider without key",
			envOverrides: map[string]string{"ENCRYPTION_PROVIDER": "static"},
			envClear:     []string{"ENCRYPTION_KEY"},
			wantErr:      "ENCRYPTION_KEY is required",
		},
		{
			name:         "encryption key wrong length",
			envOverrides: map[string]string{"ENCRYPTION_KEY": "aabbccdd"},
			wantErr:      "ENCRYPTION_KEY must be 64 hex characters",
		},
		{
			name:         "embedding dimensions zero",
			envOverrides: map[string]string{"EMBEDDING_DIMENSIONS": "0"},
			wantErr:      "EMBEDDING_DIMENSIONS must be an integer between 1 and 4096",
		},
		{
			name:         "embedding dimensions too high",
			envOverrides: map[string]string{"EMBEDDING_DIMENSIONS": "5000"},
			wantErr:      "EMBEDDING_DIMENSIONS must be an integer between 1 and 4096",
		},
		{
			name:         "embedding dimensions non-numeric",
			envOverrides: map[string]string{"EMBEDDING_DIMENSIONS": "abc"},
			wantErr:      "EMBEDDING_DIMENSIONS must be an integer between 1 and 4096",
		},
		{
			name:         "embed workers zero",
			envOverrides: map[string]string{"EMBED_WORKERS": "0"},
			wantErr:      "EMBED_WORKERS must be an integer between 1 and 16",
		},
		{
			name:         "embed workers too high",
			envOverrides: map[string]string{"EMBED_WORKERS": "17"},
			wantErr:      "EMBED_WORKERS must be an integer between 1 and 16",
		},
		{
			name:         "embed workers non-numeric",
			envOverrides: map[string]string{"EMBED_WORKERS": "abc"},
			wantErr:      "EMBED_WORKERS must be an integer between 1 and 16",
		},
		{
			name:         "db max conns too low",
			envOverrides: map[string]string{"DB_MAX_CONNS": "1"},
			wantErr:      "DB_MAX_CONNS must be an integer between 2 and 200",
		},
		{
			name:         "db max conns too high",
			envOverrides: map[string]string{"DB_MAX_CONNS": "201"},
			wantErr:      "DB_MAX_CONNS must be an integer between 2 and 200",
		},
		{
			name:         "db max conns non-numeric",
			envOverrides: map[string]string{"DB_MAX_CONNS": "abc"},
			wantErr:      "DB_MAX_CONNS must be an integer between 2 and 200",
		},
		{
			name:         "invalid METRICS_PORT zero",
			envOverrides: map[string]string{"METRICS_PORT": "0"},
			wantErr:      "METRICS_PORT must be between 1 and 65535",
		},
		{
			name:         "invalid METRICS_PORT too high",
			envOverrides: map[string]string{"METRICS_PORT": "99999"},
			wantErr:      "METRICS_PORT must be between 1 and 65535",
		},
		{
			name:         "invalid METRICS_PORT non-numeric",
			envOverrides: map[string]string{"METRICS_PORT": "abc"},
			wantErr:      "METRICS_PORT must be a valid integer",
		},
		{
			name:         "METRICS_PORT same as PORT",
			envOverrides: map[string]string{"METRICS_PORT": "3030"},
			wantErr:      "METRICS_PORT must differ from PORT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setValidEnv(t)
			for _, k := range tc.envClear {
				t.Setenv(k, "")
			}
			for k, v := range tc.envOverrides {
				t.Setenv(k, v)
			}

			_, err := config.Load()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestLoad_OllamaRemote(t *testing.T) {
	t.Run("remote URL rejected without flag", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("OLLAMA_URL", "http://ollama.internal:11434")

		_, err := config.Load()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "OLLAMA_ALLOW_REMOTE") {
			t.Errorf("expected error to mention OLLAMA_ALLOW_REMOTE, got %q", err.Error())
		}
	})

	t.Run("remote URL allowed with flag", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("OLLAMA_URL", "http://ollama.internal:11434")
		t.Setenv("OLLAMA_ALLOW_REMOTE", "true")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !cfg.OllamaAllowRemote {
			t.Error("expected OllamaAllowRemote=true")
		}
		if cfg.OllamaURL != "http://ollama.internal:11434" {
			t.Errorf("unexpected OllamaURL: %s", cfg.OllamaURL)
		}
	})

	t.Run("localhost URL still works without flag", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("OLLAMA_URL", "http://127.0.0.1:11434")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.OllamaURL != "http://127.0.0.1:11434" {
			t.Errorf("unexpected OllamaURL: %s", cfg.OllamaURL)
		}
	})
}
