package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thomaslaurenson/gpipe/internal/config"
	"github.com/thomaslaurenson/gpipe/internal/generator"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate install scripts and checksums",
	Long: `Generate install.sh, install.ps1, and checksums.txt in the current directory.
Reads configuration from .gpipe.yml and computes SHA256 checksums for each platform binary.`,
	RunE: runGenerate,
}

var generateFlags struct {
	repo        string
	version     string
	configPath  string
	binary      string
	installName string
	dryRun      bool
}

func init() {
	generateCmd.Flags().StringVar(&generateFlags.repo, "repo", "", "GitHub repo in owner/repo format")
	generateCmd.Flags().StringVar(&generateFlags.version, "version", "", "Release version tag, e.g. v1.2.3")
	generateCmd.Flags().StringVar(&generateFlags.configPath, "config", ".gpipe.yml", "Path to config file")
	generateCmd.Flags().StringVar(&generateFlags.binary, "binary", "", "Binary name (overrides config)")
	generateCmd.Flags().StringVar(&generateFlags.installName, "install-name", "", "Installed binary name on disk (overrides config)")
	generateCmd.Flags().BoolVar(&generateFlags.dryRun, "dry-run", false, "Generate scripts without requiring all binaries to be present")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(generateFlags.configPath)
	if err != nil {
		return err
	}

	config.MergeFlags(cfg, config.FlagValues{
		Repo:        generateFlags.repo,
		Version:     generateFlags.version,
		Binary:      generateFlags.binary,
		InstallName: generateFlags.installName,
	})

	mode := config.ModeNormal
	if generateFlags.dryRun {
		mode = config.ModeDryRun
	}

	if errs := config.Validate(cfg, mode); len(errs) > 0 {
		return fmt.Errorf("validation failed:\n%s", joinErrors(errs))
	}

	out, err := generator.Generate(cfg, mode)
	if err != nil {
		return err
	}

	if err := os.WriteFile("install.sh", []byte(out.InstallSh), 0o644); err != nil {
		return fmt.Errorf("writing install.sh: %w", err)
	}
	if err := os.WriteFile("install.ps1", []byte(out.InstallPs1), 0o644); err != nil {
		return fmt.Errorf("writing install.ps1: %w", err)
	}
	if err := os.WriteFile("checksums.txt", []byte(out.Checksums), 0o644); err != nil {
		return fmt.Errorf("writing checksums.txt: %w", err)
	}

	fmt.Println("generated install.sh, install.ps1, checksums.txt")
	return nil
}

func joinErrors(errs []error) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = "  - " + e.Error()
	}
	return strings.Join(msgs, "\n")
}
