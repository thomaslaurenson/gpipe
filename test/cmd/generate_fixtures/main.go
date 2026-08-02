// generate_fixtures renders the install-script fixtures used by the bash (bats)
// and PowerShell (Pester) test suites into test/fixtures/, using the real gpipe
// generator so the fixtures always match the current templates rather than
// being hand-maintained.
//
// It produces:
//
//   - install_rendered.sh / install_rendered.ps1 — rendered from a config with
//     every shell completion enabled and pre/post hooks injected from
//     test/fixtures/hooks/. The suites both call individual functions from these
//     files and grep them for the completion and hook sentinels.
//
//   - checksums.txt (and the fake_binary it covers) — used by the checksum
//     verification tests.
//
// Fake binaries (fake_binary_go, fake_binary) are written as needed so checksum
// generation has real files to hash.
//
// This is called by `make generate_test_fixtures` and is a prerequisite of the
// bash and PS test targets.
//
// Usage:
//
//	go run ./test/cmd/generate_fixtures
package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	gpipe "github.com/thomaslaurenson/gpipe/internal"
)

func main() {
	repoRoot, err := repoRoot()
	if err != nil {
		log.Fatalf("could not resolve repo root: %v", err)
	}

	fixtureDir := filepath.Join(repoRoot, "test", "fixtures")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		log.Fatalf("creating fixture dir: %v", err)
	}

	// Write a minimal fake binary so the generator can compute a real checksum.
	fakeBinary := filepath.Join(fixtureDir, "fake_binary_go")
	if err := os.WriteFile(fakeBinary, []byte("fake binary for fixture generation\n"), 0o755); err != nil {
		log.Fatalf("writing fake binary: %v", err)
	}

	platforms := map[string]gpipe.PlatformEntry{
		"linux_amd64":   {Path: fakeBinary, Name: "mytool_linux_amd64"},
		"linux_arm64":   {Path: fakeBinary, Name: "mytool_linux_arm64"},
		"darwin_amd64":  {Path: fakeBinary, Name: "mytool_darwin_amd64"},
		"darwin_arm64":  {Path: fakeBinary, Name: "mytool_darwin_arm64"},
		"windows_amd64": {Path: fakeBinary, Name: "mytool_windows_amd64.exe"},
		"windows_arm64": {Path: fakeBinary, Name: "mytool_windows_arm64.exe"},
	}

	tplFS := os.DirFS(filepath.Join(repoRoot, "templates"))

	if err := generateFull(fixtureDir, repoRoot, platforms, tplFS); err != nil {
		log.Fatalf("generating fixtures: %v", err)
	}

	if err := generateChecksums(fixtureDir, tplFS); err != nil {
		log.Fatalf("generating checksum fixtures: %v", err)
	}

	fmt.Println("fixtures up to date")
}

// generateFull renders the install scripts with all completions and pre/post
// hooks sourced from test/fixtures/hooks/.
func generateFull(fixtureDir, repoRoot string, platforms map[string]gpipe.PlatformEntry, tplFS fs.FS) error {
	hooksDir := filepath.Join(repoRoot, "test", "fixtures", "hooks")

	cfg := &gpipe.Config{
		GithubRepo: "testowner/testrepo",
		Version:    "v1.2.3",
		// Pinned so fixtures do not churn as `git describe` output changes.
		GpipeVersion: "v0.0.0-fixture",
		Binary:       "mytool",
		InstallName:  "mytool",
		Platforms:    platforms,
		Completions: gpipe.Completions{
			Bash:       true,
			Zsh:        true,
			Fish:       true,
			PowerShell: true,
		},
		Hooks: gpipe.Hooks{
			PreSh:   filepath.Join(hooksDir, "pre_install.sh"),
			PostSh:  filepath.Join(hooksDir, "post_install.sh"),
			PrePs1:  filepath.Join(hooksDir, "pre_install.ps1"),
			PostPs1: filepath.Join(hooksDir, "post_install.ps1"),
		},
	}

	out, err := gpipe.Generate(cfg, tplFS, gpipe.ModeNormal)
	if err != nil {
		return err
	}

	return writeFiles(fixtureDir, map[string]string{
		"install_rendered.sh":  out.InstallSh,
		"install_rendered.ps1": out.InstallPs1,
	})
}

// generateChecksums writes a stable checksums.txt fixture for bats tests.
func generateChecksums(fixtureDir string, tplFS fs.FS) error {
	checksumsFixture := filepath.Join(fixtureDir, "fake_binary")
	if err := os.WriteFile(checksumsFixture, []byte("#!/usr/bin/env bash\necho fake-binary\n"), 0o755); err != nil {
		return err
	}

	cfg := &gpipe.Config{
		GithubRepo: "testowner/testrepo",
		Version:    "v1.2.3",
		// Pinned so fixtures do not churn as `git describe` output changes.
		GpipeVersion: "v0.0.0-fixture",
		Binary:       "mytool",
		InstallName:  "mytool",
		Platforms: map[string]gpipe.PlatformEntry{
			"linux_amd64": {Path: checksumsFixture, Name: "fake_binary"},
		},
	}

	out, err := gpipe.Generate(cfg, tplFS, gpipe.ModeNormal)
	if err != nil {
		return err
	}

	checksumsPath := filepath.Join(fixtureDir, "checksums.txt")
	if err := os.WriteFile(checksumsPath, []byte(out.Checksums), 0o644); err != nil {
		return fmt.Errorf("writing checksums.txt: %w", err)
	}
	fmt.Printf("wrote %s\n", checksumsPath)
	return nil
}

func writeFiles(dir string, files map[string]string) error {
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		fmt.Printf("wrote %s\n", path)
	}
	return nil
}

// repoRoot walks up from the working directory looking for go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}
