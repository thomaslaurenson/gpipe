// Package gpipe provides configuration loading, validation, and install script generation for gpipe.
package gpipe

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationMode controls how strict validation is.
type ValidationMode int

const (
	ModeNormal ValidationMode = iota
	ModeValidate
	ModeDryRun
)

// ValidPlatforms lists all supported platform identifiers in canonical order.
// The order here controls the order platforms appear in generated scripts.
var ValidPlatforms = []string{
	"linux_amd64",
	"linux_arm64",
	"darwin_amd64",
	"darwin_arm64",
	"windows_amd64",
	"windows_arm64",
}

// semverPattern matches v1.2.3, 1.2.3, v1.2, 1.2
var semverPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?$`)

// semverRelaxedPattern also allows placeholder values like v0.0.0-dry-run
var semverRelaxedPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?(-[a-zA-Z0-9._-]+)?$`)

// repoPattern matches owner/repo
var repoPattern = regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)

// Hooks holds optional hook file paths.
type Hooks struct {
	PreSh   string `yaml:"pre-sh"`
	PostSh  string `yaml:"post-sh"`
	PrePs1  string `yaml:"pre-ps1"`
	PostPs1 string `yaml:"post-ps1"`
}

// Completions holds per-shell completion flags.
type Completions struct {
	Bash       bool `yaml:"bash"`
	Zsh        bool `yaml:"zsh"`
	Fish       bool `yaml:"fish"`
	PowerShell bool `yaml:"powershell"`
}

// PlatformEntry holds the local binary path and the GitHub release asset name for a platform.
type PlatformEntry struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
}

// Config holds the merged configuration from .gpipe.yml and CLI flags.
// Note: GithubRepo and Version are runtime-only inputs supplied via CLI flags,
// never read from the config file.
type Config struct {
	Binary      string                   `yaml:"binary"`
	InstallName string                   `yaml:"install-name"`
	Platforms   map[string]PlatformEntry `yaml:"platforms"`
	Hooks       Hooks                    `yaml:"hooks"`
	Completions Completions              `yaml:"completions"`
	Sign        bool                     `yaml:"sign"`
	// Runtime-only: not read from config file, always supplied via flags or auto-detected.
	GithubRepo string
	Version    string
}

// FlagValues holds CLI flag overrides.
type FlagValues struct {
	GithubRepo  string
	Version     string
	Binary      string
	InstallName string
	Sign        *bool
}

// LoadConfig reads and parses a .gpipe.yml file.
//
// Returns an empty Config (not nil) if the file does not exist.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	return &cfg, nil
}

// MergeFlags applies CLI flag overrides on top of a loaded config.
//
// install-name defaults to binary if neither is set via flag or config.
func MergeFlags(cfg *Config, flags FlagValues) {
	if flags.GithubRepo != "" {
		cfg.GithubRepo = flags.GithubRepo
	}
	if flags.Version != "" {
		cfg.Version = flags.Version
	}
	if flags.Binary != "" {
		cfg.Binary = flags.Binary
	}
	if flags.InstallName != "" {
		cfg.InstallName = flags.InstallName
	}
	if flags.Sign != nil {
		cfg.Sign = *flags.Sign
	}
	if cfg.InstallName == "" {
		cfg.InstallName = cfg.Binary
	}
}

// DetectRepo attempts to determine the GitHub repo from the git remote URL.
//
// Parses the origin remote URL and extracts the owner/repo portion from
// both HTTPS and SSH remote formats. Requires git to be available in PATH.
func DetectRepo() (string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found in PATH: cannot auto-detect repo")
	}

	out, err := exec.Command(git, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin failed: %w\n%s", err, bytes.TrimSpace(out))
	}

	remote := strings.TrimSpace(string(out))
	repo := parseGitRemote(remote)
	if repo == "" {
		return "", fmt.Errorf("could not parse owner/repo from git remote URL: %q", remote)
	}

	if !repoPattern.MatchString(repo) {
		return "", fmt.Errorf("detected repo %q does not match expected owner/repo format", repo)
	}

	return repo, nil
}

// parseGitRemote extracts owner/repo from HTTPS or SSH remote URLs.
//
// Handles:
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo
//   - git@github.com:owner/repo.git
//   - git@github.com:owner/repo
func parseGitRemote(remote string) string {
	// Strip trailing .git
	remote = strings.TrimSuffix(remote, ".git")

	// SSH format: git@github.com:owner/repo
	if strings.Contains(remote, "@") && strings.Contains(remote, ":") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// HTTPS format: https://github.com/owner/repo
	if strings.Contains(remote, "://") {
		parts := strings.SplitN(remote, "://", 2)
		if len(parts) == 2 {
			// Strip host, keep path
			path := strings.SplitN(parts[1], "/", 2)
			if len(path) == 2 {
				return path[1]
			}
		}
	}

	return ""
}

// DetectVersion attempts to determine the current version from git tags.
//
// Uses git describe to find the nearest tag. Appends -dev if the working
// tree is dirty or the commit is not exactly on a tag. Requires git in PATH.
func DetectVersion() (string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found in PATH: cannot auto-detect version")
	}

	// Try exact tag match first
	out, err := exec.Command(git, "describe", "--tags", "--exact-match", "HEAD").CombinedOutput()
	if err == nil {
		version := strings.TrimSpace(string(out))
		if semverPattern.MatchString(version) {
			return version, nil
		}
	}

	// Not on an exact tag: find nearest tag and append -dev
	out, err = exec.Command(git, "describe", "--tags", "--abbrev=0").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git describe failed: no tags found. Create a tag or pass --version explicitly")
	}

	base := strings.TrimSpace(string(out))
	if !semverRelaxedPattern.MatchString(base) {
		return "", fmt.Errorf("detected tag %q is not a semantic version: pass --version explicitly", base)
	}

	version := base + "-dev"
	return version, nil
}

// Validate checks the config for correctness and returns a slice of errors.
//
// Returns nil if all checks pass.
func Validate(cfg *Config, mode ValidationMode) []error {
	var errs []error

	if cfg.GithubRepo == "" {
		errs = append(errs, errors.New("missing required field: repo (pass --repo or ensure git remote origin is set)"))
	} else if !repoPattern.MatchString(cfg.GithubRepo) {
		errs = append(errs, fmt.Errorf("invalid repo %q: expected owner/repo format", cfg.GithubRepo))
	}

	if cfg.Version == "" {
		errs = append(errs, errors.New("missing required field: version (pass --version or ensure git tags are set)"))
	} else {
		switch mode {
		case ModeDryRun:
			if !semverRelaxedPattern.MatchString(cfg.Version) {
				errs = append(errs, fmt.Errorf("invalid version %q: expected semantic version like v1.2.3 or 1.2.3", cfg.Version))
			}
		default:
			if !semverPattern.MatchString(cfg.Version) {
				errs = append(errs, fmt.Errorf("invalid version %q: expected semantic version like v1.2.3 or 1.2.3", cfg.Version))
			}
		}
	}

	if cfg.Binary == "" {
		errs = append(errs, errors.New("missing required config field: binary"))
	}

	if len(cfg.Platforms) == 0 {
		errs = append(errs, errors.New("missing required config field: platforms"))
	} else {
		for platform := range cfg.Platforms {
			if !isValidPlatform(platform) {
				errs = append(errs, fmt.Errorf("unknown platform identifier %q: valid values are %s",
					platform, strings.Join(ValidPlatforms, ", ")))
			}
		}
		// Asset name uniqueness: two platforms must not resolve to the same filename
		seen := make(map[string]string) // assetName -> first platform ID
		for _, p := range ValidPlatforms {
			entry, ok := cfg.Platforms[p]
			if !ok {
				continue
			}
			if first, dup := seen[entry.Name]; dup {
				errs = append(errs, fmt.Errorf("platforms %q and %q resolve to the same asset name %q: asset names must be unique", first, p, entry.Name))
			} else {
				seen[entry.Name] = p
			}
		}
	}

	if err := validateHookFile(cfg.Hooks.PreSh, "pre-sh", ".sh"); err != nil {
		errs = append(errs, err)
	}
	if err := validateHookFile(cfg.Hooks.PostSh, "post-sh", ".sh"); err != nil {
		errs = append(errs, err)
	}
	if err := validateHookFile(cfg.Hooks.PrePs1, "pre-ps1", ".ps1"); err != nil {
		errs = append(errs, err)
	}
	if err := validateHookFile(cfg.Hooks.PostPs1, "post-ps1", ".ps1"); err != nil {
		errs = append(errs, err)
	}

	// In normal mode, verify binary files exist on disk
	if mode == ModeNormal {
		for platform, entry := range cfg.Platforms {
			if entry.Path == "" {
				errs = append(errs, fmt.Errorf("platform %q has empty binary path", platform))
				continue
			}
			if _, err := os.Stat(entry.Path); err != nil {
				errs = append(errs, fmt.Errorf("binary file for platform %q not found at %q", platform, entry.Path))
			}
		}
	}

	return errs
}

func validateHookFile(path, key, expectedExt string) error {
	if path == "" {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != expectedExt {
		return fmt.Errorf("hook %q has extension %q but expected %q", key, ext, expectedExt)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("hook file for %q not found at %q", key, path)
	}
	return nil
}

func isValidPlatform(platform string) bool {
	for _, v := range ValidPlatforms {
		if v == platform {
			return true
		}
	}
	return false
}

// ParseGitRemote is an exported wrapper around parseGitRemote for testing.
func ParseGitRemote(remote string) string {
	return parseGitRemote(remote)
}
