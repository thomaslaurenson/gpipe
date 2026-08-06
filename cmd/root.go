// Package cmd implements the gpipe command line interface.
package cmd

import (
	"io/fs"

	"github.com/spf13/cobra"
)

// templateFS holds the embedded templates, provided via Execute
var templateFS fs.FS

// SilenceErrors and SilenceUsage both matter here: without them cobra prints
// the error and a full usage dump, and main.go then prints the same error
// again. Reporting is the entry point's job alone.
var rootCmd = &cobra.Command{
	Use:   "gpipe",
	Short: "Install script generator for GitHub releases",
	Long: `gpipe generates install.sh, install.ps1, and checksums.txt from base templates,
injecting project-specific configuration and SHA256 checksums at generation time.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	Version:       Version,
}

// Execute runs the root command.
func Execute(tplFS fs.FS) error {
	templateFS = tplFS
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
}
