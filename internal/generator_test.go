package gpipe_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gpipe "github.com/thomaslaurenson/gpipe/internal"
)

// testTemplateFS points at the root templates/ directory.
// During go test, the working directory is set to the package source directory (internal/),
// so ../templates resolves to the repo-root templates/ folder.
var testTemplateFS = os.DirFS("../templates")

// minimalCfg returns a config pointing at real temp binary files.
func minimalCfg(t *testing.T) (*gpipe.Config, string) {
	t.Helper()
	dir := t.TempDir()

	binPath := filepath.Join(dir, "mycli-linux-x86_64")
	if err := os.WriteFile(binPath, []byte("fake binary content"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &gpipe.Config{
		GithubRepo:  "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: binPath, Name: "mycli-linux-x86_64"},
		},
	}
	return cfg, dir
}

func TestGenerate_NoLeftoverMarkers(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, s := range []string{out.InstallSh, out.InstallPs1} {
		if strings.Contains(s, "{{") || strings.Contains(s, "}}") {
			t.Errorf("output contains leftover markers:\n%s", s)
		}
	}
}

func TestGenerate_ChecksumFormat(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// One line per platform binary, plus install.sh and install.ps1 themselves
	// (added so the scripts can be verified out-of-band the same way the
	// binaries are).
	lines := strings.Split(strings.TrimSpace(out.Checksums), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 checksum lines, got %d: %q", len(lines), lines)
	}
	// Format: "<64 hex chars>  <filename>" (two-space separator is sha256sum format)
	parts := strings.SplitN(lines[0], "  ", 2)
	if len(parts) != 2 {
		t.Fatalf("checksum line not in sha256sum format: %q", lines[0])
	}
	if len(parts[0]) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars: %q", len(parts[0]), parts[0])
	}
	if parts[1] != "mycli-linux-x86_64" {
		t.Errorf("expected filename mycli-linux-x86_64, got %q", parts[1])
	}
	if !strings.Contains(lines[1], "  install.sh") {
		t.Errorf("expected install.sh checksum entry, got: %q", lines[1])
	}
	if !strings.Contains(lines[2], "  install.ps1") {
		t.Errorf("expected install.ps1 checksum entry, got: %q", lines[2])
	}
}

func TestGenerate_MultiplePlatformsChecksumOrder(t *testing.T) {
	dir := t.TempDir()

	// Create fake binaries for multiple platforms
	platforms := map[string]gpipe.PlatformEntry{}
	for _, p := range []struct {
		id   string
		name string
	}{
		{"linux_amd64", "mycli-linux-x86_64"},
		{"linux_arm64", "mycli-linux-aarch64"},
		{"darwin_amd64", "mycli-darwin-x86_64"},
	} {
		binPath := filepath.Join(dir, p.name)
		if err := os.WriteFile(binPath, []byte("fake binary "+p.id), 0o755); err != nil {
			t.Fatal(err)
		}
		platforms[p.id] = gpipe.PlatformEntry{Path: binPath, Name: p.name}
	}

	cfg := &gpipe.Config{
		GithubRepo:  "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms:   platforms,
	}

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 3 platform binaries, then install.sh and install.ps1 themselves.
	lines := strings.Split(strings.TrimSpace(out.Checksums), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 checksum lines, got %d: %q", len(lines), lines)
	}

	// Verify canonical order: linux_amd64, linux_arm64, darwin_amd64
	if !strings.Contains(lines[0], "mycli-linux-x86_64") {
		t.Errorf("expected linux_amd64 first, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "mycli-linux-aarch64") {
		t.Errorf("expected linux_arm64 second, got: %q", lines[1])
	}
	if !strings.Contains(lines[2], "mycli-darwin-x86_64") {
		t.Errorf("expected darwin_amd64 third, got: %q", lines[2])
	}
	if !strings.Contains(lines[3], "  install.sh") {
		t.Errorf("expected install.sh fourth, got: %q", lines[3])
	}
	if !strings.Contains(lines[4], "  install.ps1") {
		t.Errorf("expected install.ps1 fifth, got: %q", lines[4])
	}
}

func TestGenerate_CompletionBlockAbsentWhenDisabled(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, marker := range []string{
		"# gpipe test: bash-completions",
		"# gpipe test: zsh-completions",
		"# gpipe test: fish-completions",
	} {
		if strings.Contains(out.InstallSh, marker) {
			t.Errorf("install.sh should not contain %q when completions are disabled", marker)
		}
	}
	if strings.Contains(out.InstallPs1, "# gpipe test: powershell-completions") {
		t.Error("install.ps1 should not contain powershell-completions marker when disabled")
	}
}

func TestGenerate_CompletionBlockPresentWhenEnabled(t *testing.T) {
	cfg, _ := minimalCfg(t)
	cfg.Completions.Bash = true
	cfg.Completions.Zsh = true
	cfg.Completions.Fish = true

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, marker := range []string{
		"# gpipe test: bash-completions",
		"# gpipe test: zsh-completions",
		"# gpipe test: fish-completions",
	} {
		if !strings.Contains(out.InstallSh, marker) {
			t.Errorf("install.sh missing %q when completions are enabled", marker)
		}
	}
}

func TestGenerate_Ps1CompletionsPresentWhenEnabled(t *testing.T) {
	cfg, _ := minimalCfg(t)
	cfg.Completions.PowerShell = true

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallPs1, "# gpipe test: powershell-completions") {
		t.Error("install.ps1 missing powershell-completions marker when enabled")
	}
	if !strings.Contains(out.InstallPs1, "Install-Completion") {
		t.Error("install.ps1 missing Install-Completion function when enabled")
	}
}

func TestGenerate_ShPostHookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "post-install.sh")
	os.WriteFile(hookPath, []byte("echo 'post hook'\n"), 0o644)
	cfg.Hooks.PostSh = hookPath

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallSh, "# gpipe test: post-install-hook") {
		t.Error("install.sh missing post-install-hook sentinel")
	}
	if !strings.Contains(out.InstallSh, "echo 'post hook'") {
		t.Error("install.sh missing post hook content")
	}
}

func TestGenerate_ShPreHookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "pre-install.sh")
	os.WriteFile(hookPath, []byte("echo 'pre hook'\n"), 0o644)
	cfg.Hooks.PreSh = hookPath

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallSh, "# gpipe test: pre-install-hook") {
		t.Error("install.sh missing pre-install-hook sentinel")
	}
	if !strings.Contains(out.InstallSh, "echo 'pre hook'") {
		t.Error("install.sh missing pre hook content")
	}
}

func TestGenerate_Ps1PostHookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "post-install.ps1")
	os.WriteFile(hookPath, []byte("Write-Host 'post hook'\n"), 0o644)
	cfg.Hooks.PostPs1 = hookPath

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallPs1, "# gpipe test: post-install-hook") {
		t.Error("install.ps1 missing post-install-hook sentinel")
	}
	if !strings.Contains(out.InstallPs1, "Write-Host 'post hook'") {
		t.Error("install.ps1 missing post hook content")
	}
}

func TestGenerate_Ps1PreHookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "pre-install.ps1")
	os.WriteFile(hookPath, []byte("Write-Host 'pre hook'\n"), 0o644)
	cfg.Hooks.PrePs1 = hookPath

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallPs1, "# gpipe test: pre-install-hook") {
		t.Error("install.ps1 missing pre-install-hook sentinel")
	}
	if !strings.Contains(out.InstallPs1, "Write-Host 'pre hook'") {
		t.Error("install.ps1 missing pre hook content")
	}
}

func TestGenerate_NoHookLeavesNoMarker(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, marker := range []string{
		"# gpipe test: pre-install-hook",
		"# gpipe test: post-install-hook",
	} {
		if strings.Contains(out.InstallSh, marker) {
			t.Errorf("install.sh should not contain %q when no hooks are configured", marker)
		}
		if strings.Contains(out.InstallPs1, marker) {
			t.Errorf("install.ps1 should not contain %q when no hooks are configured", marker)
		}
	}
}

func TestGenerate_BashSyntaxErrorInHookFails(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "bad.sh")
	os.WriteFile(hookPath, []byte("if [\n"), 0o644) // intentionally broken bash
	cfg.Hooks.PreSh = hookPath

	_, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err == nil {
		t.Fatal("expected error for bash syntax error in hook, got nil")
	}
	if !strings.Contains(err.Error(), "bash syntax error") {
		t.Errorf("expected 'bash syntax error' in error, got: %v", err)
	}
}

func TestGenerate_Ps1SyntaxErrorInHookFails(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not installed, cannot exercise the ps1 syntax check")
	}

	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "bad.ps1")
	os.WriteFile(hookPath, []byte("if (\n"), 0o644) // intentionally broken PowerShell
	cfg.Hooks.PrePs1 = hookPath

	_, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err == nil {
		t.Fatal("expected error for powershell syntax error in hook, got nil")
	}
	if !strings.Contains(err.Error(), "powershell syntax error") {
		t.Errorf("expected 'powershell syntax error' in error, got: %v", err)
	}
}

func TestGenerate_Ps1SyntaxCheckSkippedWhenPwshMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()
	hookPath := filepath.Join(dir, "hook.ps1")
	os.WriteFile(hookPath, []byte("Write-Host 'hello'\n"), 0o644)

	cfg := &gpipe.Config{
		GithubRepo:  "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: "/nonexistent", Name: "mycli-linux-x86_64"},
		},
		Hooks: gpipe.Hooks{PrePs1: hookPath},
	}

	// Should not error on missing pwsh - syntax check is skipped with a warning
	_, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeDryRun)
	if err != nil {
		t.Fatalf("missing pwsh should skip syntax check, not error: %v", err)
	}
}

func TestGenerate_DryRunSkipsMissingBinary(t *testing.T) {
	cfg := &gpipe.Config{
		GithubRepo:  "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: "/nonexistent/mycli-linux-x86_64", Name: "mycli-linux-x86_64"},
		},
	}

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeDryRun)
	if err != nil {
		t.Fatalf("dry-run should not error on missing binary, got: %v", err)
	}
	if strings.Contains(out.Checksums, "mycli-linux-x86_64") {
		t.Errorf("missing binary should have no checksum entry, got: %q", out.Checksums)
	}
	// install.sh/install.ps1 are rendered in-memory regardless of which
	// platform binaries are present on disk, so their checksum entries are
	// still expected even when every platform binary is missing.
	if !strings.Contains(out.Checksums, "  install.sh") {
		t.Errorf("expected install.sh checksum entry even with missing binaries, got: %q", out.Checksums)
	}
	if !strings.Contains(out.Checksums, "  install.ps1") {
		t.Errorf("expected install.ps1 checksum entry even with missing binaries, got: %q", out.Checksums)
	}
}

func TestGenerate_HeaderPresent(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.InstallSh, "Generated by gpipe") {
		t.Error("install.sh missing generated-by header")
	}
	if !strings.Contains(out.InstallPs1, "Generated by gpipe") {
		t.Error("install.ps1 missing generated-by header")
	}
}

func TestGenerate_GpipeVersionStamped(t *testing.T) {
	cfg, _ := minimalCfg(t) // Version: v1.2.3
	cfg.GpipeVersion = "v1.4.2"

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for name, script := range map[string]string{"install.sh": out.InstallSh, "install.ps1": out.InstallPs1} {
		if !strings.Contains(script, "# Gpipe-Version: v1.4.2") {
			t.Errorf("%s missing Gpipe-Version stamp", name)
		}
		if !strings.Contains(script, "# Release-Version: v1.2.3") {
			t.Errorf("%s missing Release-Version header", name)
		}
	}
}

func TestGenerate_GpipeVersionDefaultsToUnknown(t *testing.T) {
	cfg, _ := minimalCfg(t) // GpipeVersion deliberately unset

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallSh, "# Gpipe-Version: unknown") {
		t.Error("install.sh should stamp 'unknown' when GpipeVersion is unset")
	}
}

// The binary is built with an unprefixed version (goreleaser's .Version, which
// is also what the CLI prints), but the header is a tag reference and must be
// v-prefixed to match the Release-Version above it.
func TestGenerate_GpipeVersionStampIsTagPrefixed(t *testing.T) {
	tests := []struct {
		name  string
		given string
		want  string
	}{
		{"unprefixed release", "1.4.2", "v1.4.2"},
		{"unprefixed snapshot", "1.2.1-dev", "v1.2.1-dev"},
		{"already prefixed", "v1.4.2", "v1.4.2"},
		{"plain go build", "dev", "dev"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := minimalCfg(t)
			cfg.GpipeVersion = tc.given

			out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := "# Gpipe-Version: " + tc.want + "\n"
			if !strings.Contains(out.InstallSh, want) {
				t.Errorf("install.sh: GpipeVersion %q should stamp %q", tc.given, tc.want)
			}
			if !strings.Contains(out.InstallPs1, want) {
				t.Errorf("install.ps1: GpipeVersion %q should stamp %q", tc.given, tc.want)
			}
		})
	}
}

func TestGenerate_CosignIdentityBakedIn(t *testing.T) {
	cfg, _ := minimalCfg(t) // GithubRepo: owner/mycli, Version: v1.2.3
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The same literal must appear in both scripts, unchanged.
	identity := `^https://github\.com/owner/mycli/\.github/workflows/.+@refs/tags/v1\.2\.3$`
	if !strings.Contains(out.InstallSh, identity) {
		t.Errorf("install.sh should contain anchored cosign identity %q", identity)
	}
	if !strings.Contains(out.InstallPs1, identity) {
		t.Errorf("install.ps1 should contain anchored cosign identity %q", identity)
	}
}

func TestGenerate_CosignIdentityEscapesDotsInRepo(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "bin")
	if err := os.WriteFile(binPath, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &gpipe.Config{
		GithubRepo:  "owner/my.tool",
		Version:     "v1.0.0",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: binPath, Name: "mycli-linux-x86_64"},
		},
	}
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The dot in "my.tool" must be regex-escaped (\.); an unescaped dot would
	// make the pattern also match confusable names like "my-tool"/"myXtool".
	escaped := `owner/my\.tool`
	if !strings.Contains(out.InstallSh, escaped) {
		t.Errorf("install.sh cosign identity should escape the dot in repo name, want substring %q in:\n%s", escaped, out.InstallSh)
	}
	if !strings.Contains(out.InstallPs1, escaped) {
		t.Errorf("install.ps1 cosign identity should escape the dot in repo name, want substring %q", escaped)
	}
}

func TestGenerate_CosignIdentityBindsVersionTagRef(t *testing.T) {
	cfg, _ := minimalCfg(t) // Version: v1.2.3
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSuffix := `@refs/tags/v1\.2\.3$`
	if !strings.Contains(out.InstallSh, wantSuffix) {
		t.Errorf("install.sh cosign identity should be bound to the release tag ref, want substring %q", wantSuffix)
	}
	if !strings.Contains(out.InstallPs1, wantSuffix) {
		t.Errorf("install.ps1 cosign identity should be bound to the release tag ref, want substring %q", wantSuffix)
	}
}

func TestGenerate_Ps1FunctionsPresent(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, fn := range []string{
		"function Get-Platform",
		"function Resolve-Asset",
		"function Invoke-DownloadAsset",
		"function Confirm-Signature",
		"function Confirm-Checksum",
		"function Resolve-InstallDir",
		"function Install-Binary",
		"function Update-Path",
		"function Invoke-Installer",
	} {
		if !strings.Contains(out.InstallPs1, fn) {
			t.Errorf("install.ps1 missing expected function: %s", fn)
		}
	}
}

func TestGenerate_Ps1DotSourceGuardPresent(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallPs1, "MyInvocation.InvocationName -ne '.'") {
		t.Error("install.ps1 missing dot-source guard for testability")
	}
}

func TestGenerate_Ps1ScriptScopeConstants(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, constant := range []string{
		"$script:GithubRepo",
		"$script:Version",
		"$script:Binary",
		"$script:InstallName",
	} {
		if !strings.Contains(out.InstallPs1, constant) {
			t.Errorf("install.ps1 missing script-scoped constant: %s", constant)
		}
	}
}

func TestGenerate_ShFunctionsPresent(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, fn := range []string{
		"parse_args()",
		"detect_platform()",
		"resolve_asset()",
		"setup_downloader()",
		"download_assets()",
		"verify_signature()",
		"verify_checksum()",
		"_try_install()",
		"install_binary()",
		"manage_path()",
	} {
		if !strings.Contains(out.InstallSh, fn) {
			t.Errorf("install.sh missing expected function: %s", fn)
		}
	}
}

func TestGenerate_ShBashSourceGuardPresent(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallSh, `BASH_SOURCE[0]}" == "${0}"`) {
		t.Error("install.sh missing BASH_SOURCE guard for testability")
	}
}

func TestSignChecksums_SignFalseNoError(t *testing.T) {
	cfg, _ := minimalCfg(t)
	cfg.Sign = false
	_, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("sign=false should not require cosign, got error: %v", err)
	}
}

func TestSignChecksums_CosignMissingReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := gpipe.SignChecksums("/some/checksums.txt")
	if err == nil {
		t.Fatal("expected error when cosign is not in PATH, got nil")
	}
	if !strings.Contains(err.Error(), "cosign not found") {
		t.Errorf("expected 'cosign not found' in error, got: %v", err)
	}
}

func TestSignChecksums_BashSyntaxCheckSkippedWhenBashMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()
	hookPath := filepath.Join(dir, "hook.sh")
	// Valid bash syntax - should not error even without bash available
	os.WriteFile(hookPath, []byte("echo hello\n"), 0o644)

	cfg := &gpipe.Config{
		GithubRepo:  "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: "/nonexistent", Name: "mycli-linux-x86_64"},
		},
		Hooks: gpipe.Hooks{PreSh: hookPath},
	}

	// Should not error on missing bash - syntax check is skipped with a warning
	_, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeDryRun)
	if err != nil {
		t.Fatalf("missing bash should skip syntax check, not error: %v", err)
	}
}
