package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationMode controls how strict validation is.
type ValidationMode int

const (
	ModeNormal   ValidationMode = iota
	ModeValidate ValidationMode = iota
	ModeDryRun   ValidationMode = iota
)

// ValidPlatforms lists all supported platform identifiers in canonical order.
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

// Config holds the merged configuration from .gpipe.yml and CLI flags.
type Config struct {
	Binary      string            `yaml:"binary"`
	InstallName string            `yaml:"install-name"`
	Platforms   map[string]string `yaml:"platforms"`
	Hooks       Hooks             `yaml:"hooks"`
	Completions Completions       `yaml:"completions"`

	// Runtime fields set via CLI flags, not present in YAML.
	Repo    string `yaml:"-"`
	Version string `yaml:"-"`
}

// FlagValues holds CLI flag overrides.
type FlagValues struct {
	Repo        string
	Version     string
	Binary      string
	InstallName string
}

// LoadConfig reads and parses a .gpipe.yml file.
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
// install-name defaults to binary if neither is set via flag or config.
func MergeFlags(cfg *Config, flags FlagValues) {
	if flags.Repo != "" {
		cfg.Repo = flags.Repo
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
	if cfg.InstallName == "" {
		cfg.InstallName = cfg.Binary
	}
}

// Validate checks the config for correctness and returns a slice of errors.
// Returns nil if all checks pass.
func Validate(cfg *Config, mode ValidationMode) []error {
	var errs []error

	if cfg.Repo == "" {
		errs = append(errs, errors.New("missing required field: repo"))
	} else if !repoPattern.MatchString(cfg.Repo) {
		errs = append(errs, fmt.Errorf("invalid repo %q: expected owner/repo format", cfg.Repo))
	}

	if cfg.Version == "" {
		errs = append(errs, errors.New("missing required field: version"))
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

	// In normal mode, verify binary files exist on disk.
	if mode == ModeNormal {
		for platform, path := range cfg.Platforms {
			if path == "" {
				errs = append(errs, fmt.Errorf("platform %q has empty binary path", platform))
				continue
			}
			if _, err := os.Stat(path); err != nil {
				errs = append(errs, fmt.Errorf("binary file for platform %q not found at %q", platform, path))
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
