package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose configuration and connectivity",
		Long:  "Run diagnostic checks against config, server, and auth",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}
}

type checkResult struct {
	Name   string
	Passed bool
	Detail string
	Hint   string
}

func runDoctor() error {
	fmt.Println("\nPersistor Doctor")
	fmt.Println("================")

	var results []checkResult

	// 1. Config file.
	cfgPath, cfg, cfgErr := doctorLoadConfig()
	if cfgErr != nil {
		results = append(results, checkResult{
			Name: "Config file", Passed: false,
			Detail: cfgPath,
			Hint:   "Run: persistor init",
		})
	} else {
		results = append(results, checkResult{
			Name: "Config file", Passed: true,
			Detail: fmt.Sprintf("found (%s)", cfgPath),
		})
	}

	// Resolve URL and key from flags, env, config (same priority as resolveConfig).
	url, apiKey := doctorResolveSettings(cfg)

	// 2. Server URL.
	if url == "" {
		results = append(results, checkResult{
			Name: "Server URL", Passed: false,
			Hint: "Set --url, PERSISTOR_URL, or run persistor init",
		})
	} else {
		results = append(results, checkResult{
			Name: "Server URL", Passed: true, Detail: url,
		})
	}

	// 3. API key.
	if apiKey == "" {
		results = append(results, checkResult{
			Name: "API key", Passed: false,
			Hint: "Set --api-key, PERSISTOR_API_KEY, or run persistor init",
		})
	} else {
		results = append(results, checkResult{
			Name: "API key", Passed: true, Detail: "configured",
		})
	}

	// 4. Server reachable.
	var serverVersion string
	if url != "" {
		ver, err := doctorCheckHealth(url)
		if err != nil {
			results = append(results, checkResult{
				Name: "Server reachable", Passed: false,
				Detail: url,
				Hint:   fmt.Sprintf("Is the Persistor server running? Try: systemctl status persistor\n   Error: %v", err),
			})
		} else {
			serverVersion = ver
			detail := url
			if ver != "" {
				detail = fmt.Sprintf("v%s", ver)
			}
			results = append(results, checkResult{
				Name: "Server reachable", Passed: true, Detail: detail,
			})
		}
	}

	// 5. Authentication.
	if url != "" && apiKey != "" {
		if err := doctorCheckAuth(url, apiKey); err != nil {
			results = append(results, checkResult{
				Name: "Authentication", Passed: false,
				Hint: fmt.Sprintf("Check your API key. Error: %v", err),
			})
		} else {
			results = append(results, checkResult{
				Name: "Authentication", Passed: true, Detail: "valid",
			})
		}
	}

	// 6. Server version (info only, already captured).
	_ = serverVersion

	// Print results.
	fmt.Println()
	allPassed := true
	for _, r := range results {
		if r.Passed {
			if r.Detail != "" {
				fmt.Printf("✅ %s: %s\n", r.Name, r.Detail)
			} else {
				fmt.Printf("✅ %s\n", r.Name)
			}
		} else {
			allPassed = false
			if r.Detail != "" {
				fmt.Printf("❌ %s: %s\n", r.Name, r.Detail)
			} else {
				fmt.Printf("❌ %s\n", r.Name)
			}
			if r.Hint != "" {
				fmt.Printf("   Hint: %s\n", r.Hint)
			}
		}
	}

	fmt.Println()
	if allPassed {
		fmt.Println("✅ All checks passed!")
	} else {
		fmt.Println("❌ Some checks failed.")
		return fmt.Errorf("doctor found issues")
	}

	return nil
}

func doctorLoadConfig() (string, *profilesFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	cfgPath := filepath.Join(home, ".persistor", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return cfgPath, nil, err
	}
	var cfg profilesFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfgPath, nil, err
	}
	return cfgPath, &cfg, nil
}

func doctorResolveSettings(cfg *profilesFile) (url, apiKey string) {
	// Flags first (use the global flag values).
	url = flagURL
	apiKey = flagKey

	// Env overrides defaults.
	if url == "http://localhost:3030" {
		if v := os.Getenv("PERSISTOR_URL"); v != "" {
			url = v
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("PERSISTOR_API_KEY")
	}

	// Config file fills remaining gaps.
	if cfg != nil {
		profile := cfg.ActiveProfile
		if profile == "" {
			profile = "default"
		}
		if p, ok := cfg.Profiles[profile]; ok {
			if url == "http://localhost:3030" && p.URL != "" {
				url = p.URL
			}
			if apiKey == "" && p.APIKey != "" {
				apiKey = p.APIKey
			}
		}
	}

	return url, apiKey
}

func doctorCheckHealth(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/health", nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var health struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return "", err
	}
	return health.Version, nil
}

func doctorCheckAuth(url, apiKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/stats", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
