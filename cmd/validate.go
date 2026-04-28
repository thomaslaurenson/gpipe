package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thomaslaurenson/gpipe/internal/config"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate config and hooks without generating files",
	Long: `Validate .gpipe.yml structure, platform identifiers, hook file existence,
hook syntax, and version format. No files are generated.

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
	validateCmd.Flags().StringVar(&validateFlags.repo, "repo", "", "GitHub repo in owner/repo format (optional override)")
	validateCmd.Flags().StringVar(&validateFlags.version, "version", "", "Version to validate format of (optional)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(validateFlags.configPath)
	if err != nil {
		return err
	}

	config.MergeFlags(cfg, config.FlagValues{
		Repo:    validateFlags.repo,
		Version: validateFlags.version,
	})

	if errs := config.Validate(cfg, config.ModeValidate); len(errs) > 0 {
		return fmt.Errorf("validation failed:\n%s", joinErrors(errs))
	}

	fmt.Println("config is valid")
	return nil
}
