package gpipe_test

import (
	"os"
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

	lines := strings.Split(strings.TrimSpace(out.Checksums), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 checksum line, got %d", len(lines))
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

	lines := strings.Split(strings.TrimSpace(out.Checksums), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 checksum lines, got %d", len(lines))
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
}

func TestGenerate_CompletionBlockAbsentWhenDisabled(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, phrase := range []string{"completion bash", "completion zsh", "completion fish"} {
		if strings.Contains(out.InstallSh, phrase) {
			t.Errorf("install.sh should not contain %q when completion is disabled", phrase)
		}
	}
	if strings.Contains(out.InstallPs1, "completion powershell") {
		t.Error("install.ps1 should not contain powershell completion block when disabled")
	}
}

func TestGenerate_CompletionBlockPresentWhenEnabled(t *testing.T) {
	cfg, _ := minimalCfg(t)
	cfg.Completions.Bash = true
	cfg.Completions.Zsh = true

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallSh, "completion bash") {
		t.Error("install.sh should contain bash completion block when enabled")
	}
	if !strings.Contains(out.InstallSh, "completion zsh") {
		t.Error("install.sh should contain zsh completion block when enabled")
	}
}

func TestGenerate_ShHookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "post-install.sh")
	os.WriteFile(hookPath, []byte("echo 'post hook'\n"), 0o644)
	cfg.Hooks.PostSh = hookPath

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallSh, "# --- gpipe: post-install hook ---") {
		t.Error("missing post-install hook header comment")
	}
	if !strings.Contains(out.InstallSh, "echo 'post hook'") {
		t.Error("hook content not present in output")
	}
	if !strings.Contains(out.InstallSh, "# --- gpipe: end post-install hook ---") {
		t.Error("missing post-install hook footer comment")
	}
}

func TestGenerate_Ps1HookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "post-install.ps1")
	os.WriteFile(hookPath, []byte("Write-Host 'post hook'\n"), 0o644)
	cfg.Hooks.PostPs1 = hookPath

	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.InstallPs1, "# --- gpipe: post-install hook ---") {
		t.Error("ps1: missing post-install hook header comment")
	}
	if !strings.Contains(out.InstallPs1, "Write-Host 'post hook'") {
		t.Error("ps1: hook content not present in output")
	}
	if !strings.Contains(out.InstallPs1, "# --- gpipe: end post-install hook ---") {
		t.Error("ps1: missing post-install hook footer comment")
	}
}

func TestGenerate_NoHookLeavesNoMarker(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out.InstallSh, "PRE_INSTALL_HOOK") || strings.Contains(out.InstallSh, "POST_INSTALL_HOOK") {
		t.Error("hook markers should be removed entirely when no hooks are provided")
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
	if strings.TrimSpace(out.Checksums) != "" {
		t.Errorf("expected empty checksums for missing binary in dry-run, got: %q", out.Checksums)
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

func TestGenerate_CosignIdentityBakedIn(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := gpipe.Generate(cfg, testTemplateFS, gpipe.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedIdentity := "https://github.com/owner/mycli/.github/workflows/.*"
	if !strings.Contains(out.InstallSh, expectedIdentity) {
		t.Errorf("install.sh should contain baked-in cosign identity %q", expectedIdentity)
	}
	if !strings.Contains(out.InstallPs1, expectedIdentity) {
		t.Errorf("install.ps1 should contain baked-in cosign identity %q", expectedIdentity)
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
