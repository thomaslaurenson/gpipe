package gpipe

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// Output holds the three generated file contents.
type Output struct {
	InstallSh  string
	InstallPs1 string
	Checksums  string
}

// templateData is the data passed to install script templates
type templateData struct {
	GithubRepo  string
	Version     string
	Binary      string
	InstallName string
	Completions Completions
	Platforms   []platformEntry
	Hooks       hookContent
}

// platformEntry holds a single platform's ID and resolved asset filename
type platformEntry struct {
	ID        string
	AssetName string
}

// hookContent holds loaded hook file contents (empty string when not configured)
type hookContent struct {
	PreSh   string
	PostSh  string
	PrePs1  string
	PostPs1 string
}

// Generate produces install.sh, install.ps1, and checksums.txt from cfg.
//
// tplFS must be an fs.FS with install.sh and install.ps1 at its root.
// In dry-run mode, missing platform binary files produce warnings instead of errors.
func Generate(cfg *Config, tplFS fs.FS, mode ValidationMode) (*Output, error) {
	shTpl, err := fs.ReadFile(tplFS, "install.sh")
	if err != nil {
		return nil, fmt.Errorf("reading install.sh template: %w", err)
	}
	ps1Tpl, err := fs.ReadFile(tplFS, "install.ps1")
	if err != nil {
		return nil, fmt.Errorf("reading install.ps1 template: %w", err)
	}

	checksums, err := buildChecksums(cfg, mode)
	if err != nil {
		return nil, err
	}

	preShHook, err := loadHook(cfg.Hooks.PreSh, "bash")
	if err != nil {
		return nil, fmt.Errorf("pre-sh hook: %w", err)
	}
	postShHook, err := loadHook(cfg.Hooks.PostSh, "bash")
	if err != nil {
		return nil, fmt.Errorf("post-sh hook: %w", err)
	}
	prePs1Hook, err := loadHook(cfg.Hooks.PrePs1, "ps1")
	if err != nil {
		return nil, fmt.Errorf("pre-ps1 hook: %w", err)
	}
	postPs1Hook, err := loadHook(cfg.Hooks.PostPs1, "ps1")
	if err != nil {
		return nil, fmt.Errorf("post-ps1 hook: %w", err)
	}

	var platforms []platformEntry
	for _, p := range ValidPlatforms {
		entry, ok := cfg.Platforms[p]
		if !ok {
			continue
		}
		platforms = append(platforms, platformEntry{
			ID:        p,
			AssetName: entry.Name,
		})
	}

	data := &templateData{
		GithubRepo:  cfg.GithubRepo,
		Version:     cfg.Version,
		Binary:      cfg.Binary,
		InstallName: cfg.InstallName,
		Completions: cfg.Completions,
		Platforms:   platforms,
		Hooks: hookContent{
			PreSh:   preShHook,
			PostSh:  postShHook,
			PrePs1:  prePs1Hook,
			PostPs1: postPs1Hook,
		},
	}

	sh, err := render(string(shTpl), data)
	if err != nil {
		return nil, fmt.Errorf("rendering install.sh: %w", err)
	}

	ps1, err := render(string(ps1Tpl), data)
	if err != nil {
		return nil, fmt.Errorf("rendering install.ps1: %w", err)
	}

	return &Output{
		InstallSh:  sh,
		InstallPs1: ps1,
		Checksums:  checksums,
	}, nil
}

// render executes a text/template template string against the provided data
func render(tpl string, data *templateData) (string, error) {
	t, err := template.New("").Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

func buildChecksums(cfg *Config, mode ValidationMode) (string, error) {
	var sb strings.Builder
	for _, platform := range ValidPlatforms {
		entry, ok := cfg.Platforms[platform]
		if !ok {
			continue
		}
		f, err := os.Open(entry.Path)
		if err != nil {
			if mode == ModeDryRun {
				fmt.Fprintf(os.Stderr, "warning: skipping checksum for %s: file not found at %s\n", platform, entry.Path)
				continue
			}
			return "", fmt.Errorf("opening binary for platform %s at %s: %w", platform, entry.Path, err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Errorf("hashing binary for platform %s: %w", platform, err)
		}
		f.Close()
		fmt.Fprintf(&sb, "%x  %s\n", h.Sum(nil), entry.Name)
	}
	return sb.String(), nil
}

func loadHook(path, shellType string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading hook file %q: %w", path, err)
	}
	content := string(data)
	if strings.TrimSpace(content) == "" {
		fmt.Fprintf(os.Stderr, "warning: hook file %q is empty, skipping\n", path)
		return "", nil
	}
	if shellType == "bash" {
		if err := validateBashSyntax(path); err != nil {
			return "", err
		}
	}
	return content, nil
}

func validateBashSyntax(path string) error {
	bash, err := exec.LookPath("bash")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: bash not found, skipping syntax check for %q\n", path)
		return nil
	}
	out, err := exec.Command(bash, "-n", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("bash syntax error in hook %q:\n%s", path, bytes.TrimSpace(out))
	}
	return nil
}

// SignChecksums runs `cosign sign-blob --yes` on the given file.
//
// Returns a clear error if cosign is not installed.
func SignChecksums(path string) error {
	cosign, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("cosign not found in PATH: install it from https://docs.sigstore.dev/cosign/system_config/installation/")
	}
	out, err := exec.Command(cosign, "sign-blob", "--yes", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cosign sign-blob failed: %w\n%s", err, bytes.TrimSpace(out))
	}
	return nil
}
