// Package gpipe provides configuration loading, validation, and install script generation for gpipe.
package gpipe

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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

// semverPattern matches semantic version tags: v1.2.3, 1.2.3, v1.2, 1.2,
// with an optional prerelease/build suffix such as v1.2.3-rc.1 or the
// -dev suffix DetectVersion appends when HEAD is not exactly on a tag.
// Applied uniformly across all validation modes: the suffix charset
// ([a-zA-Z0-9._-]) contains no shell metacharacters, so accepting it does
// not weaken the injection defence these values are validated against
// before being interpolated into the generated install.sh / install.ps1.
var semverPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?(-[a-zA-Z0-9._-]+)?$`)

// repoPattern matches owner/repo.
//
// The character classes deliberately match GitHub's own allowed owner/repo
// characters (letters, digits, and '-' for owners; plus '.' and '_' for repos)
// and, crucially, exclude every shell metacharacter. Because these values are
// interpolated into the generated install.sh / install.ps1, a permissive
// pattern would allow command injection into the emitted scripts.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

// namePattern matches shell-safe identifiers used for the binary name and
// per-platform asset filenames. It permits only letters, digits, '.', '_'
// and '-' so that these values cannot break out of the quoted strings they
// are injected into within the generated scripts.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Hooks holds optional hook file paths.
type Hooks struct {
	PreSh   string `yaml:"pre-sh"`
	PostSh  string `yaml:"post-sh"`
	PrePs1  string `yaml:"pre-ps1"`
	PostPs1 string `yaml:"post-ps1"`
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
	Binary    string                   `yaml:"binary"`
	Platforms map[string]PlatformEntry `yaml:"platforms"`
	Hooks     Hooks                    `yaml:"hooks"`
	// Runtime-only: not read from config file, always supplied via flags or auto-detected.
	GithubRepo string
	Version    string
	// GpipeVersion is the gpipe version doing the generating, stamped into the
	// generated scripts. Defaults to "unknown" when unset.
	GpipeVersion string
}

// FlagValues holds CLI flag overrides.
type FlagValues struct {
	GithubRepo string
	Version    string
}

// LoadConfig reads and parses a .gpipe.yml file.
//
// Returns an empty Config (not nil) if the file does not exist.
//
// Decoding is strict (KnownFields): an unrecognised key such as a typo'd
// "post_sh" (underscore instead of hyphen) is a parse error rather than
// being silently ignored.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty (or comment/whitespace-only) file decodes as io.EOF rather
	// than populating cfg; treat that the same as "no fields set", matching
	// the previous yaml.Unmarshal behaviour instead of surfacing an error.
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	return &cfg, nil
}

// MergeFlags applies CLI flag overrides on top of a loaded config.
func MergeFlags(cfg *Config, flags FlagValues) {
	if flags.GithubRepo != "" {
		cfg.GithubRepo = flags.GithubRepo
	}
	if flags.Version != "" {
		cfg.Version = flags.Version
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

// githubHostPattern matches a git remote host that is exactly github.com
// (case-insensitive, as DNS host comparisons conventionally are). gpipe's
// generated scripts always download from https://github.com/..., so a
// remote pointing at GitLab, Bitbucket, a self-hosted GitHub Enterprise
// instance, or any other host must not silently resolve to a
// plausible-looking but wrong owner/repo.
var githubHostPattern = regexp.MustCompile(`(?i)^github\.com$`)

// parseGitRemote extracts owner/repo from a github.com remote URL.
//
// Handles:
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo
//   - ssh://git@github.com/owner/repo.git
//   - ssh://git@github.com/owner/repo
//   - git@github.com:owner/repo.git
//   - git@github.com:owner/repo
//
// Returns "" if the URL cannot be parsed, or if it parses but the host is
// not github.com.
func parseGitRemote(remote string) string {
	remote = strings.TrimSuffix(remote, ".git")

	// scheme://[user@]host[:port]/owner/repo covers both ssh:// and
	// http(s):// remotes, which share the same scheme://host/path shape.
	if strings.Contains(remote, "://") {
		parts := strings.SplitN(remote, "://", 2)
		if len(parts) != 2 {
			return ""
		}
		rest := parts[1]
		if at := strings.LastIndex(rest, "@"); at != -1 {
			rest = rest[at+1:]
		}
		hostAndPath := strings.SplitN(rest, "/", 2)
		if len(hostAndPath) != 2 {
			return ""
		}
		host := strings.SplitN(hostAndPath[0], ":", 2)[0] // strip a port
		if !githubHostPattern.MatchString(host) {
			return ""
		}
		return hostAndPath[1]
	}

	// SCP-like short syntax (no scheme): [user@]host:owner/repo
	if strings.Contains(remote, "@") && strings.Contains(remote, ":") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return ""
		}
		userAndHost := strings.SplitN(parts[0], "@", 2)
		host := userAndHost[len(userAndHost)-1]
		if !githubHostPattern.MatchString(host) {
			return ""
		}
		return parts[1]
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
	if !semverPattern.MatchString(base) {
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
	} else if !semverPattern.MatchString(cfg.Version) {
		errs = append(errs, fmt.Errorf("invalid version %q: expected semantic version like v1.2.3 or 1.2.3", cfg.Version))
	}

	if cfg.Binary == "" {
		errs = append(errs, errors.New("missing required config field: binary"))
	} else if !namePattern.MatchString(cfg.Binary) {
		errs = append(errs, fmt.Errorf("invalid binary %q: allowed characters are letters, digits, '.', '_' and '-'", cfg.Binary))
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
			if entry.Name == "" {
				errs = append(errs, fmt.Errorf("platform %q has empty asset name", p))
			} else if !namePattern.MatchString(entry.Name) {
				errs = append(errs, fmt.Errorf("invalid asset name %q for platform %q: allowed characters are letters, digits, '.', '_' and '-'", entry.Name, p))
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
