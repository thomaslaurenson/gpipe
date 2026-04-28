package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "gpipe",
	Short: "Install script generator for GitHub releases",
	Long: `gpipe generates install.sh, install.ps1, and checksums.txt from base templates,
injecting project-specific configuration and SHA256 checksums at generation time.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
}
