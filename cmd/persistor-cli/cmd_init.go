package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// profileConfig holds connection settings for a single profile.
type profileConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

// profilesFile is the top-level config file structure.
type profilesFile struct {
	Profiles      map[string]profileConfig `yaml:"profiles"`
	ActiveProfile string                   `yaml:"active_profile"`
}

func newInitCmd() *cobra.Command {
	var (
		initURL    string
		initAPIKey string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up Persistor CLI configuration",
		Long:  "Interactive setup wizard that creates ~/.persistor/config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			nonInteractive := initURL != "" || initAPIKey != ""
			return runInit(initURL, initAPIKey, nonInteractive)
		},
	}

	cmd.Flags().StringVar(&initURL, "url", "", "Server URL (non-interactive mode)")
	cmd.Flags().StringVar(&initAPIKey, "api-key", "", "API key (non-interactive mode)")
	return cmd
}

func runInit(url, apiKey string, nonInteractive bool) error {
	if !nonInteractive {
		fmt.Println("\n  Persistor Setup")
		fmt.Println("  ───────────────")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("  Server URL [http://localhost:3030]: ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			url = line
		}

		fmt.Print("  API Key: ")
		keyLine, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(keyLine)
	}

	if url == "" {
		url = "http://localhost:3030"
	}

	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Test connection.
	if !nonInteractive {
		fmt.Print("\n  Testing connection... ")
	}

	ver, err := testConnection(url, apiKey)
	if err != nil {
		if !nonInteractive {
			fmt.Println("✗")
		}
		return fmt.Errorf("connection failed: %w", err)
	}

	if !nonInteractive {
		fmt.Printf("✓ Connected (v%s)\n", ver)
	}

	// Write config.
	cfgPath, err := writeConfig(url, apiKey)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if nonInteractive {
		fmt.Printf("Config saved to %s\n", cfgPath)
	} else {
		fmt.Printf("\n  ✓ Config saved to %s\n", cfgPath)
		fmt.Println()
		fmt.Println("  Next steps:")
		fmt.Println("    persistor doctor     # Full diagnostic check")
		fmt.Println("    persistor admin stats # View your graph")
		fmt.Println("    persistor --help     # See all commands")
		fmt.Println()
	}

	return nil
}

func testConnection(url, apiKey string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/health", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	// Parse version from JSON response.
	var health struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return "", err
	}
	if health.Version == "" {
		health.Version = "unknown"
	}
	return health.Version, nil
}

func writeConfig(url, apiKey string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, ".persistor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	cfg := profilesFile{
		Profiles: map[string]profileConfig{
			"default": {URL: url, APIKey: apiKey},
		},
		ActiveProfile: "default",
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return "", err
	}

	return cfgPath, nil
}
