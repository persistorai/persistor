package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/config"
)

// Build-time variables set via ldflags.
var (
	commit    = ""
	buildDate = ""
)

// flagFmt is the shared output format (json|table) for commands that print
// structured results.
var flagFmt string

func versionString() string {
	if commit != "" && buildDate != "" {
		return fmt.Sprintf("persistor version %s (commit: %s, built: %s)", config.Version, commit, buildDate)
	}
	return fmt.Sprintf("persistor version %s-dev", config.Version)
}

func main() {
	rootCmd := &cobra.Command{
		Use:          "persistor",
		Short:        "Persistor — memory for AI agents",
		Version:      versionString(),
		SilenceUsage: true,
	}
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().StringVar(&flagFmt, "format", "json", "Output format: json|table")

	rootCmd.AddCommand(newReindexCmd())
	rootCmd.AddCommand(newEvalCmd())
	rootCmd.AddCommand(newBriefCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newConsolidateCmd())
	rootCmd.AddCommand(newKeyCmd())
	rootCmd.AddCommand(newAdminCmd())
	rootCmd.AddCommand(newExportCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
