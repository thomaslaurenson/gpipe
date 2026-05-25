package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	gpipe "github.com/thomaslaurenson/gpipe/internal"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate install scripts and checksums",
	Long: `Generate install.sh, install.ps1, and checksums.txt in the current directory.
Reads configuration from .gpipe.yml and computes SHA256 checksums for each platform binary.

--repo and --version are required. If not provided via flags, they are auto-detected
from the git remote origin URL and the nearest git tag respectively.`,
	RunE: runGenerate,
}

var generateFlags struct {
	repo        string
	version     string
	configPath  string
	binary      string
	installName string
	dryRun      bool
	sign        bool
}

func init() {
	generateCmd.Flags().StringVar(&generateFlags.repo, "repo", "", "GitHub repo in owner/repo format (auto-detected from git remote if not set)")
	generateCmd.Flags().StringVar(&generateFlags.version, "version", "", "Release version tag, e.g. v1.2.3 (auto-detected from git tags if not set)")
	generateCmd.Flags().StringVar(&generateFlags.configPath, "config", ".gpipe.yml", "Path to config file")
	generateCmd.Flags().StringVar(&generateFlags.binary, "binary", "", "Binary name (overrides config)")
	generateCmd.Flags().StringVar(&generateFlags.installName, "install-name", "", "Installed binary name on disk (overrides config)")
	generateCmd.Flags().BoolVar(&generateFlags.dryRun, "dry-run", false, "Generate scripts without requiring all binaries to be present")
	generateCmd.Flags().BoolVar(&generateFlags.sign, "sign", false, "Sign checksums.txt with cosign after generation (overrides config)")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	cfg, err := gpipe.LoadConfig(generateFlags.configPath)
	if err != nil {
		return err
	}

	var signFlag *bool
	if cmd.Flags().Changed("sign") {
		signFlag = &generateFlags.sign
	}

	gpipe.MergeFlags(cfg, gpipe.FlagValues{
		GithubRepo:  generateFlags.repo,
		Version:     generateFlags.version,
		Binary:      generateFlags.binary,
		InstallName: generateFlags.installName,
		Sign:        signFlag,
	})

	// Auto-detect repo if not supplied
	if cfg.GithubRepo == "" {
		detected, err := gpipe.DetectRepo()
		if err != nil {
			return fmt.Errorf("--repo not provided and auto-detection failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "detected repo: %s\n", detected)
		cfg.GithubRepo = detected
	}

	// Auto-detect version if not supplied
	if cfg.Version == "" {
		detected, err := gpipe.DetectVersion()
		if err != nil {
			return fmt.Errorf("--version not provided and auto-detection failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "detected version: %s\n", detected)
		cfg.Version = detected
	}

	mode := gpipe.ModeNormal
	if generateFlags.dryRun {
		mode = gpipe.ModeDryRun
	}

	if errs := gpipe.Validate(cfg, mode); len(errs) > 0 {
		return fmt.Errorf("validation failed:\n%s", joinErrors(errs))
	}

	out, err := gpipe.Generate(cfg, templateFS, mode)
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

	if cfg.Sign {
		if err := gpipe.SignChecksums("checksums.txt"); err != nil {
			return err
		}
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
