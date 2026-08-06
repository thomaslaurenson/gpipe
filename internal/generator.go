package gpipe

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
)

// Sentinel errors for the failure modes a caller can act on. Each is wrapped
// with %w at the point of failure, so callers match with errors.Is rather than
// on message text.
var (
	// ErrBashSyntax reports a hook rejected by bash -n.
	ErrBashSyntax = errors.New("bash syntax error in hook")
	// ErrPs1Syntax reports a hook rejected by the PowerShell parser.
	ErrPs1Syntax = errors.New("powershell syntax error in hook")
	// ErrCosignNotFound reports that cosign is not installed.
	ErrCosignNotFound = errors.New("cosign not found in PATH")
	// ErrCosignSign reports that cosign ran but failed to sign.
	ErrCosignSign = errors.New("cosign sign-blob failed")
)

// Output holds the three generated file contents.
type Output struct {
	InstallSh  string
	InstallPs1 string
	Checksums  string
}

// templateData is the data passed to install script templates
type templateData struct {
	GithubRepo string
	// Version is the released project version; GpipeVersion is the gpipe that
	// rendered the script. Unrelated, and stamped separately.
	Version      string
	GpipeVersion string
	Binary       string
	// ShPlatforms and Ps1Platforms are disjoint subsets of the configured
	// platforms, split by OS so install.sh never carries an unreachable
	// windows_* case arm (and vice versa for install.ps1).
	ShPlatforms  []platformEntry
	Ps1Platforms []platformEntry
	Hooks        hookContent
	// CosignCertIdentity is the assembled --certificate-identity-regexp. Its
	// QuoteMeta backslashes survive both bash double-quoting and PowerShell
	// single-quoting, so one string is interpolated into both templates
	// unchanged.
	CosignCertIdentity string
}

// cosignCertIdentity builds the --certificate-identity-regexp value the
// generated installers pass to `cosign verify-blob`.
//
// The pattern is anchored (^...$) to this exact repository and requires the
// signing workflow run to have been triggered by pushing this exact version
// tag (@refs/tags/VERSION). That closes two gaps a looser pattern leaves
// open: a signature from a different repository whose name differs from
// this one only by a regex metacharacter (e.g. "my.tool" vs "myXtool")
// cannot verify, and a checksums.txt signed for a different release cannot
// be replayed against this version (anti-rollback). The workflow filename
// segment is left open (.+) because it is not knowable at generation time
// for arbitrary consumers of gpipe.
//
// GithubRepo and Version are both constrained by Validate (repoPattern /
// semverPattern) before Generate ever runs, so QuoteMeta only ever has
// literal dots to escape in practice here. It is applied unconditionally
// rather than relying on that invariant, since regex-injection safety
// should not depend on a caller having already validated its input.
func cosignCertIdentity(githubRepo, version string) string {
	return fmt.Sprintf(
		`^https://github\.com/%s/\.github/workflows/.+@refs/tags/%s$`,
		regexp.QuoteMeta(githubRepo),
		regexp.QuoteMeta(version),
	)
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

// stampVersion normalises a gpipe version for the Gpipe-Version header.
//
// The build stamps cmd.Version from goreleaser's .Version, which is the release
// tag with the v stripped, matching the unprefixed form the CLI prints. The
// header is a tag reference though, sitting directly above a v-prefixed
// Release-Version, so the v is added back here.
//
// Only versions starting with a digit are prefixed, leaving non-tag values
// ("dev" from a plain go build, "unknown" below, or the already-prefixed output
// of git describe) as they are.
func stampVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	if c := version[0]; c >= '0' && c <= '9' {
		return "v" + version
	}
	return version
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

	var shPlatforms, ps1Platforms []platformEntry
	for _, p := range ValidPlatforms {
		entry, ok := cfg.Platforms[p]
		if !ok {
			continue
		}
		pe := platformEntry{ID: p, AssetName: entry.Name}
		if strings.HasPrefix(p, "windows_") {
			ps1Platforms = append(ps1Platforms, pe)
		} else {
			shPlatforms = append(shPlatforms, pe)
		}
	}

	gpipeVersion := stampVersion(cfg.GpipeVersion)

	data := &templateData{
		GithubRepo:         cfg.GithubRepo,
		Version:            cfg.Version,
		GpipeVersion:       gpipeVersion,
		Binary:             cfg.Binary,
		ShPlatforms:        shPlatforms,
		Ps1Platforms:       ps1Platforms,
		CosignCertIdentity: cosignCertIdentity(cfg.GithubRepo, cfg.Version),
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

	// After rendering, so the scripts are themselves covered by checksums.txt
	// and can be verified before being run.
	checksums, err := buildChecksums(cfg, mode, sh, ps1)
	if err != nil {
		return nil, err
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

// buildChecksums hashes each configured platform binary, followed by the
// already-rendered install.sh and install.ps1 content, so both scripts can
// be verified out-of-band the same way the binaries are.
func buildChecksums(cfg *Config, mode ValidationMode, installSh, installPs1 string) (string, error) {
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
	fmt.Fprintf(&sb, "%x  install.sh\n", sha256.Sum256([]byte(installSh)))
	fmt.Fprintf(&sb, "%x  install.ps1\n", sha256.Sum256([]byte(installPs1)))
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
	switch shellType {
	case "bash":
		if err := validateBashSyntax(path); err != nil {
			return "", err
		}
	case "ps1":
		if err := validatePs1Syntax(path); err != nil {
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
		return fmt.Errorf("%w %q:\n%s", ErrBashSyntax, path, bytes.TrimSpace(out))
	}
	return nil
}

// ps1ParseCheckScript parses (but does not execute) the file named by the
// GPIPE_HOOK_PATH environment variable using the PowerShell language parser,
// the pwsh equivalent of `bash -n`. The path travels via an environment
// variable rather than being interpolated into the script text so no
// PowerShell quoting of the path is needed here at all.
const ps1ParseCheckScript = `
$parseErrors = $null
[System.Management.Automation.Language.Parser]::ParseFile($env:GPIPE_HOOK_PATH, [ref]$null, [ref]$parseErrors) | Out-Null
if ($parseErrors.Count -gt 0) {
    $parseErrors | ForEach-Object { Write-Output $_.ToString() }
    exit 1
}
exit 0
`

// validatePs1Syntax mirrors validateBashSyntax for PowerShell hooks: it
// catches a broken .ps1 hook before it ends up in a published installer.
// Skipped with a warning (not an error) when pwsh is unavailable, the same
// fallback validateBashSyntax uses when bash is missing.
func validatePs1Syntax(path string) error {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: pwsh not found, skipping syntax check for %q\n", path)
		return nil
	}
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", ps1ParseCheckScript)
	cmd.Env = append(os.Environ(), "GPIPE_HOOK_PATH="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w %q:\n%s", ErrPs1Syntax, path, bytes.TrimSpace(out))
	}
	return nil
}

// SignChecksums runs `cosign sign-blob --yes` on the given file.
//
// Returns a clear error if cosign is not installed.
func SignChecksums(path string) error {
	cosign, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("%w: install it from https://docs.sigstore.dev/cosign/system_config/installation/", ErrCosignNotFound)
	}
	bundle := path + ".sigstore.json"
	out, err := exec.Command(cosign, "sign-blob", "--yes", "--bundle="+bundle, path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %v\n%s", ErrCosignSign, err, bytes.TrimSpace(out))
	}
	return nil
}
