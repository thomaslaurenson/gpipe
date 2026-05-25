package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	gpipe "github.com/thomaslaurenson/gpipe/internal"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate config and hooks without generating files",
	Long: `Validate .gpipe.yml structure, platform identifiers, hook file existence,
hook syntax, and version format. No files are generated.

--repo and --version are optional for validate. If not provided, auto-detection
from git is attempted. Validation proceeds even if detection fails for these fields.

Suitable as a pre-commit hook or CI lint step.`,
	RunE: runValidate,
}

var validateFlags struct {
	configPath string
	repo       string
	version    string
}

func init() {
	validateCmd.Flags().StringVar(&validateFlags.configPath, "config", ".gpipe.yml", "Path to config file")
	validateCmd.Flags().StringVar(&validateFlags.repo, "repo", "", "GitHub repo in owner/repo format (optional, auto-detected if not set)")
	validateCmd.Flags().StringVar(&validateFlags.version, "version", "", "Version to validate format of (optional, auto-detected if not set)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	cfg, err := gpipe.LoadConfig(validateFlags.configPath)
	if err != nil {
		return err
	}

	gpipe.MergeFlags(cfg, gpipe.FlagValues{
		GithubRepo: validateFlags.repo,
		Version:    validateFlags.version,
	})

	// Attempt auto-detection for repo and version if not supplied.
	// Validation mode: detection failures are warnings, not hard errors,
	// since validate is primarily for checking config file structure.
	if cfg.GithubRepo == "" {
		if detected, err := gpipe.DetectRepo(); err == nil {
			fmt.Fprintf(os.Stderr, "detected repo: %s\n", detected)
			cfg.GithubRepo = detected
		}
	}
	if cfg.Version == "" {
		if detected, err := gpipe.DetectVersion(); err == nil {
			fmt.Fprintf(os.Stderr, "detected version: %s\n", detected)
			cfg.Version = detected
		}
	}

	if errs := gpipe.Validate(cfg, gpipe.ModeValidate); len(errs) > 0 {
		return fmt.Errorf("validation failed:\n%s", joinErrors(errs))
	}

	fmt.Println("config is valid")
	return nil
}
