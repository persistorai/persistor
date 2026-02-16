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
	if flagURL == "http://localhost:3030" && cfg.URL != "" {
		flagURL = cfg.URL
	}
	if flagKey == "" && cfg.APIKey != "" {
		flagKey = cfg.APIKey
	}
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	os.Exit(1)
}
