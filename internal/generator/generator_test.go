package generator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomaslaurenson/gpipe/internal/config"
	"github.com/thomaslaurenson/gpipe/internal/generator"
)

// minimalCfg returns a config pointing at real temp binary files.
func minimalCfg(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()

	binPath := filepath.Join(dir, "mycli-linux-x86_64")
	if err := os.WriteFile(binPath, []byte("fake binary content"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Repo:        "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms:   map[string]string{"linux_amd64": binPath},
	}
	return cfg, dir
}

func TestGenerate_NoLeftoverMarkers(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := generator.Generate(cfg, config.ModeNormal)
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
	out, err := generator.Generate(cfg, config.ModeNormal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.Checksums), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 checksum line, got %d", len(lines))
	}
	// Format: "<64 hex chars> <filename>"
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

func TestGenerate_CompletionBlockAbsentWhenDisabled(t *testing.T) {
	cfg, _ := minimalCfg(t)
	// All completions default false, no blocks should appear
	out, err := generator.Generate(cfg, config.ModeNormal)
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

	out, err := generator.Generate(cfg, config.ModeNormal)
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

func TestGenerate_HookInjectedAndWrapped(t *testing.T) {
	cfg, dir := minimalCfg(t)

	hookPath := filepath.Join(dir, "post-install.sh")
	os.WriteFile(hookPath, []byte("echo 'post hook'\n"), 0o644)
	cfg.Hooks.PostSh = hookPath

	out, err := generator.Generate(cfg, config.ModeNormal)
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

func TestGenerate_NoHookLeavesNoMarker(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := generator.Generate(cfg, config.ModeNormal)
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

	_, err := generator.Generate(cfg, config.ModeNormal)
	if err == nil {
		t.Fatal("expected error for bash syntax error in hook, got nil")
	}
	if !strings.Contains(err.Error(), "bash syntax error") {
		t.Errorf("expected 'bash syntax error' in error, got: %v", err)
	}
}

func TestGenerate_DryRunSkipsMissingBinary(t *testing.T) {
	cfg := &config.Config{
		Repo:        "owner/mycli",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]string{
			"linux_amd64": "/nonexistent/mycli-linux-x86_64",
		},
	}

	out, err := generator.Generate(cfg, config.ModeDryRun)
	if err != nil {
		t.Fatalf("dry-run should not error on missing binary, got: %v", err)
	}
	// Checksums should be empty since no binary was found.
	if strings.TrimSpace(out.Checksums) != "" {
		t.Errorf("expected empty checksums for missing binary in dry-run, got: %q", out.Checksums)
	}
}

func TestGenerate_HeaderPresent(t *testing.T) {
	cfg, _ := minimalCfg(t)
	out, err := generator.Generate(cfg, config.ModeNormal)
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
