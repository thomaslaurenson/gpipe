package gpipe_test

import (
	"os"
	"path/filepath"
	"testing"

	gpipe "github.com/thomaslaurenson/gpipe/internal"
)

func TestLoadConfig_FileNotExist(t *testing.T) {
	t.Parallel()
	cfg, err := gpipe.LoadConfig("/nonexistent/.gpipe.yml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for missing file")
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gpipe.yml")
	content := `
binary: mycli
install-name: cli
sign: true
platforms:
  linux_amd64:
    path: ./dist/mycli-linux-x86_64
    name: mycli-linux-x86_64
completions:
  bash: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := gpipe.LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Binary != "mycli" {
		t.Errorf("expected binary=mycli, got %q", cfg.Binary)
	}
	if cfg.InstallName != "cli" {
		t.Errorf("expected install-name=cli, got %q", cfg.InstallName)
	}
	// repo and version must not be read from config file
	if cfg.GithubRepo != "" {
		t.Errorf("repo should not be read from config file, got %q", cfg.GithubRepo)
	}
	if cfg.Version != "" {
		t.Errorf("version should not be read from config file, got %q", cfg.Version)
	}
	if !cfg.Sign {
		t.Error("expected sign=true")
	}
	if cfg.Platforms["linux_amd64"].Path != "./dist/mycli-linux-x86_64" {
		t.Errorf("unexpected platform path: %q", cfg.Platforms["linux_amd64"].Path)
	}
	if cfg.Platforms["linux_amd64"].Name != "mycli-linux-x86_64" {
		t.Errorf("unexpected platform name: %q", cfg.Platforms["linux_amd64"].Name)
	}
	if !cfg.Completions.Bash {
		t.Error("expected completions.bash=true")
	}
}

func TestLoadConfig_RepoVersionIgnoredFromYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gpipe.yml")
	// Even if someone puts repo/version in the file, they must be ignored
	content := `
binary: tool
platforms:
  linux_amd64:
    path: ./dist/tool
    name: tool-linux-x86_64
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := gpipe.LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GithubRepo != "" {
		t.Errorf("repo should not be populated from config file, got %q", cfg.GithubRepo)
	}
	if cfg.Version != "" {
		t.Errorf("version should not be populated from config file, got %q", cfg.Version)
	}
}

func TestLoadConfig_MalformedYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gpipe.yml")
	if err := os.WriteFile(path, []byte("binary: [\ninvalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := gpipe.LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestMergeFlags_OverridesApplied(t *testing.T) {
	t.Parallel()
	signTrue := true
	cfg := &gpipe.Config{Binary: "old", GithubRepo: "old/old"}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{
		GithubRepo:  "new/repo",
		Version:     "v1.0.0",
		Binary:      "newbinary",
		InstallName: "newname",
		Sign:        &signTrue,
	})
	if cfg.GithubRepo != "new/repo" {
		t.Errorf("expected repo=new/repo, got %q", cfg.GithubRepo)
	}
	if cfg.Version != "v1.0.0" {
		t.Errorf("expected version=v1.0.0, got %q", cfg.Version)
	}
	if cfg.Binary != "newbinary" {
		t.Errorf("expected binary=newbinary, got %q", cfg.Binary)
	}
	if cfg.InstallName != "newname" {
		t.Errorf("expected install-name=newname, got %q", cfg.InstallName)
	}
	if !cfg.Sign {
		t.Error("expected sign=true")
	}
}

func TestMergeFlags_EmptyFlagsDoNotOverride(t *testing.T) {
	t.Parallel()
	cfg := &gpipe.Config{Binary: "mycli", GithubRepo: "owner/repo"}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{})
	if cfg.Binary != "mycli" {
		t.Errorf("empty flag should not override binary, got %q", cfg.Binary)
	}
	if cfg.GithubRepo != "owner/repo" {
		t.Errorf("empty flag should not override repo, got %q", cfg.GithubRepo)
	}
}

func TestMergeFlags_InstallNameDefaultsToBinary(t *testing.T) {
	t.Parallel()
	cfg := &gpipe.Config{Binary: "mycli"}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{})
	if cfg.InstallName != "mycli" {
		t.Errorf("install-name should default to binary, got %q", cfg.InstallName)
	}
}

func TestMergeFlags_InstallNameNotOverriddenWhenSet(t *testing.T) {
	t.Parallel()
	cfg := &gpipe.Config{Binary: "mycli", InstallName: "cli"}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{})
	if cfg.InstallName != "cli" {
		t.Errorf("existing install-name should be preserved, got %q", cfg.InstallName)
	}
}

func minimalValidConfig() *gpipe.Config {
	return &gpipe.Config{
		GithubRepo:  "owner/repo",
		Version:     "v1.2.3",
		Binary:      "mycli",
		InstallName: "mycli",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: "/nonexistent/path", Name: "mycli-linux-x86_64"},
		},
	}
}

func TestValidate_MissingRepo(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.GithubRepo = ""
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "missing required field: repo")
}

func TestValidate_BadRepoFormat(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.GithubRepo = "notarepo"
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "invalid repo")
}

func TestValidate_MissingVersion(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Version = ""
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "missing required field: version")
}

func TestValidate_InvalidVersionNormal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "branch name", version: "main"},
		{name: "branch with slash", version: "fix/my-bug"},
		{name: "non-semver string", version: "latest"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := minimalValidConfig()
			cfg.Version = tc.version
			errs := gpipe.Validate(cfg, gpipe.ModeNormal)
			assertContainsError(t, errs, "invalid version")
		})
	}
}

func TestValidate_ValidVersionFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "v-prefixed three-part", version: "v1.2.3"},
		{name: "three-part no prefix", version: "1.2.3"},
		{name: "v-prefixed two-part", version: "v1.2"},
		{name: "two-part no prefix", version: "1.2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := minimalValidConfig()
			cfg.Version = tc.version
			errs := gpipe.Validate(cfg, gpipe.ModeValidate)
			for _, e := range errs {
				if e.Error() == "invalid version" {
					t.Errorf("version %q should be valid but got error: %v", tc.version, e)
				}
			}
		})
	}
}

func TestValidate_DryRunAllowsPlaceholderVersion(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Version = "v0.0.0-dry-run"
	errs := gpipe.Validate(cfg, gpipe.ModeDryRun)
	for _, e := range errs {
		if e.Error() != "" && contains(e.Error(), "invalid version") {
			t.Errorf("dry-run should allow placeholder version, got: %v", e)
		}
	}
}

func TestValidate_UnknownPlatform(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Platforms = map[string]gpipe.PlatformEntry{"solaris_amd64": {Path: "./bin", Name: "mycli-solaris-x86_64"}}
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "unknown platform identifier")
}

func TestValidate_MissingBinary(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Binary = ""
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "missing required config field: binary")
}

func TestValidate_HookWrongExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hookPath := filepath.Join(dir, "hook.ps1")
	os.WriteFile(hookPath, []byte("echo hello"), 0o644)

	cfg := minimalValidConfig()
	cfg.Hooks.PreSh = hookPath
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "expected \".sh\"")
}

func TestValidate_HookFileMissing(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Hooks.PostSh = "/nonexistent/hook.sh"
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "not found")
}

func TestValidate_NormalModeBinaryFileMissing(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Platforms = map[string]gpipe.PlatformEntry{"linux_amd64": {Path: "/nonexistent/binary", Name: "mycli-linux-x86_64"}}
	errs := gpipe.Validate(cfg, gpipe.ModeNormal)
	assertContainsError(t, errs, "not found")
}

func TestValidate_ValidateModeIgnoresMissingBinaryFiles(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Platforms = map[string]gpipe.PlatformEntry{"linux_amd64": {Path: "/nonexistent/binary", Name: "mycli-linux-x86_64"}}
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	for _, e := range errs {
		if contains(e.Error(), "binary file for platform") {
			t.Errorf("validate mode should not check binary file existence, got: %v", e)
		}
	}
}

func TestMergeFlags_SignNilDoesNotOverride(t *testing.T) {
	t.Parallel()
	cfg := &gpipe.Config{Sign: true}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{})
	if !cfg.Sign {
		t.Error("nil Sign flag should not override config sign=true")
	}
}

func TestMergeFlags_SignFalseOverridesTrue(t *testing.T) {
	t.Parallel()
	signFalse := false
	cfg := &gpipe.Config{Sign: true}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{Sign: &signFalse})
	if cfg.Sign {
		t.Error("Sign=false flag should override config sign=true")
	}
}

func TestMergeFlags_CLIOverridesRepoAndVersion(t *testing.T) {
	t.Parallel()
	cfg := &gpipe.Config{GithubRepo: "old/repo", Version: "v1.0.0"}
	gpipe.MergeFlags(cfg, gpipe.FlagValues{GithubRepo: "cli/repo", Version: "v2.0.0"})
	if cfg.GithubRepo != "cli/repo" {
		t.Errorf("CLI --repo should override, got %q", cfg.GithubRepo)
	}
	if cfg.Version != "v2.0.0" {
		t.Errorf("CLI --version should override, got %q", cfg.Version)
	}
}

func TestValidate_DuplicateAssetNames(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Platforms = map[string]gpipe.PlatformEntry{
		"linux_amd64": {Path: "/dist/my-tool-x86_64", Name: "my-tool"},
		"linux_arm64": {Path: "/dist/my-tool-aarch64", Name: "my-tool"},
	}
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	assertContainsError(t, errs, "resolve to the same asset name")
}

func TestValidate_UniqueAssetNamesNoError(t *testing.T) {
	t.Parallel()
	cfg := minimalValidConfig()
	cfg.Platforms = map[string]gpipe.PlatformEntry{
		"linux_amd64": {Path: "/dist/my-tool-x86_64", Name: "my-tool-x86_64"},
		"linux_arm64": {Path: "/dist/my-tool-aarch64", Name: "my-tool-aarch64"},
	}
	errs := gpipe.Validate(cfg, gpipe.ModeValidate)
	for _, e := range errs {
		if contains(e.Error(), "resolve to the same asset name") {
			t.Errorf("unique asset names should not trigger uniqueness error, got: %v", e)
		}
	}
}

func TestParseGitRemote_HTTPS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		remote   string
		expected string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"https://github.com/org/my-tool.git", "org/my-tool"},
	}
	for _, tc := range tests {
		got := gpipe.ParseGitRemote(tc.remote)
		if got != tc.expected {
			t.Errorf("ParseGitRemote(%q) = %q, want %q", tc.remote, got, tc.expected)
		}
	}
}

func TestParseGitRemote_SSH(t *testing.T) {
	t.Parallel()
	tests := []struct {
		remote   string
		expected string
	}{
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"git@github.com:owner/repo", "owner/repo"},
		{"git@github.com:org/my-tool.git", "org/my-tool"},
	}
	for _, tc := range tests {
		got := gpipe.ParseGitRemote(tc.remote)
		if got != tc.expected {
			t.Errorf("ParseGitRemote(%q) = %q, want %q", tc.remote, got, tc.expected)
		}
	}
}

func TestParseGitRemote_Invalid(t *testing.T) {
	t.Parallel()
	got := gpipe.ParseGitRemote("not-a-remote")
	if got != "" {
		t.Errorf("expected empty string for invalid remote, got %q", got)
	}
}

func assertContainsError(t *testing.T, errs []error, substr string) {
	t.Helper()
	for _, e := range errs {
		if contains(e.Error(), substr) {
			return
		}
	}
	t.Errorf("expected an error containing %q, got: %v", substr, errs)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
