package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/persistorai/persistor/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	version   = "dev"
	apiClient *client.Client
	flagURL   string
	flagKey   string
	flagFmt   string
)

type configFile struct {
	// Flat format (legacy)
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
	// Profile format
	Profiles      map[string]configProfile `yaml:"profiles"`
	ActiveProfile string                   `yaml:"active_profile"`
}

type configProfile struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

func main() {
	rootCmd := &cobra.Command{
		Use:     "persistor",
		Short:   "Persistor CLI — knowledge graph memory for AI agents",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			resolveConfig()
			var opts []client.Option
			if flagKey != "" {
				opts = append(opts, client.WithAPIKey(flagKey))
			}
			apiClient = client.New(flagURL, opts...)
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&flagURL, "url", "http://localhost:3030", "Persistor server URL (env: PERSISTOR_URL)")
	rootCmd.PersistentFlags().StringVar(&flagKey, "api-key", "", "API key (env: PERSISTOR_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&flagFmt, "format", "json", "Output format: json|table|quiet")

	initCmd := newInitCmd()
	initCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {} // skip client setup
	doctorCmd := newDoctorCmd()
	doctorCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {} // skip client setup

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(newNodeCmd())
	rootCmd.AddCommand(newEdgeCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newGraphCmd())
	rootCmd.AddCommand(newSalienceCmd())
	rootCmd.AddCommand(newAdminCmd())
	rootCmd.AddCommand(newAuditCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveConfig() {
	// Flag takes precedence, then env, then config file.
	if flagURL == "http://localhost:3030" {
		if v := os.Getenv("PERSISTOR_URL"); v != "" {
			flagURL = v
		}
	}
	if flagKey == "" {
		flagKey = os.Getenv("PERSISTOR_API_KEY")
	}

	// Try config file for any remaining defaults.
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	cfgPath := filepath.Join(home, ".persistor", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	var cfg configFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return
	}
	// Resolve from profiles if available, fall back to flat format
	resolvedURL := cfg.URL
	resolvedKey := cfg.APIKey
	if cfg.Profiles != nil {
		profileName := cfg.ActiveProfile
		if profileName == "" {
			profileName = "default"
		}
		if p, ok := cfg.Profiles[profileName]; ok {
			if p.URL != "" {
				resolvedURL = p.URL
			}
			if p.APIKey != "" {
				resolvedKey = p.APIKey
			}
		}
	}
	if flagURL == "http://localhost:3030" && resolvedURL != "" {
		flagURL = resolvedURL
	}
	if flagKey == "" && resolvedKey != "" {
		flagKey = resolvedKey
	}
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	os.Exit(1)
}
