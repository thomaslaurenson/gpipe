# gpipe

![Build Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&logo=github) ![Test Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&label=test&logo=github)

![Release Version](https://img.shields.io/github/v/release/thomaslaurenson/gpipe?style=flat&logo=github) ![Release downloads](https://img.shields.io/github/downloads/thomaslaurenson/gpipe/total?label=downloads&logo=github)

![Go Version](https://img.shields.io/github/go-mod/go-version/thomaslaurenson/gpipe?logo=go) ![Code Coverage](https://img.shields.io/badge/Coverage-60%25-blue?logo=go)

Automated, cross-platform, language-agnostic installer script generation for GitHub binary releases.

`gpipe` generates `install.sh`, `install.ps1`, and `checksums.txt` from embedded templates, injecting project-specific configuration and SHA256 checksums at generation time. Designed for **single raw binary distribution** via `curl | bash`. Language-agnostic — works with Go, C++, Rust, or any toolchain that produces binaries.

## Quick Start

```yaml
# .gpipe.yml
binary: mycli

platforms:
  linux_amd64:
    path: ./dist/mycli-linux-x86_64
    name: mycli-linux-x86_64
  linux_arm64:
    path: ./dist/mycli-linux-aarch64
    name: mycli-linux-aarch64
  darwin_amd64:
    path: ./dist/mycli-darwin-x86_64
    name: mycli-darwin-x86_64
  darwin_arm64:
    path: ./dist/mycli-darwin-aarch64
    name: mycli-darwin-aarch64
  windows_amd64:
    path: ./dist/mycli-windows-x86_64.exe
    name: mycli-windows-x86_64.exe
```

```bash
gpipe generate --repo owner/mycli --version v1.2.3
```

Outputs `install.sh`, `install.ps1`, and `checksums.txt` in the current directory.

## Installation

```bash
curl -fsSL https://github.com/thomaslaurenson/gpipe/releases/latest/download/install.sh | bash
```

Or download a binary directly from the [releases page](https://github.com/thomaslaurenson/gpipe/releases).

## `.gpipe.yml` Reference

```yaml
binary: mycli              # required: canonical binary name used in the generated scripts
install-name: mycli        # optional: name written to disk after install (defaults to binary)
sign: false                # optional: run cosign keyless signing on generated checksums.txt

platforms:                 # required: map of platform identifier to path/name entry
  linux_amd64:
    path: ./dist/mycli-linux-x86_64      # local path to the built binary
    name: mycli-linux-x86_64             # GitHub release asset filename
  linux_arm64:
    path: ./dist/mycli-linux-aarch64
    name: mycli-linux-aarch64
  darwin_amd64:
    path: ./dist/mycli-darwin-x86_64
    name: mycli-darwin-x86_64
  darwin_arm64:
    path: ./dist/mycli-darwin-aarch64
    name: mycli-darwin-aarch64
  windows_amd64:
    path: ./dist/mycli-windows-x86_64.exe
    name: mycli-windows-x86_64.exe
  windows_arm64:
    path: ./dist/mycli-windows-aarch64.exe
    name: mycli-windows-aarch64.exe

hooks:
  pre-sh:   .gpipe/pre-install.sh    # injected before download in install.sh
  post-sh:  .gpipe/post-install.sh   # injected after install in install.sh
  pre-ps1:  .gpipe/pre-install.ps1   # injected before download in install.ps1
  post-ps1: .gpipe/post-install.ps1  # injected after install in install.ps1

completions:
  bash:        false   # default
  zsh:         false   # default
  fish:        false   # default
  powershell:  false   # default
```

> **Note:** `repo` and `version` are runtime-only inputs. They are always supplied via `--repo` and `--version` flags (or auto-detected from git). They are never read from `.gpipe.yml`.

## CLI Reference

```
gpipe generate   --repo <owner/repo> --version <vX.Y.Z> [--sign] [--dry-run]
gpipe validate   [--repo <owner/repo>] [--version <vX.Y.Z>]
gpipe platforms  # list supported platform identifiers
gpipe version    # print gpipe version
```

`--repo` and `--version` are required for `generate`. If not supplied as flags, they are auto-detected from the git remote origin URL and the nearest git tag respectively. Detection prints what was found so you can verify it.

## Platform Matrix

| OS      | Arch  | Identifier      |
|---------|-------|-----------------|
| Linux   | x86_64 | `linux_amd64`  |
| Linux   | ARM64  | `linux_arm64`  |
| macOS   | x86_64 | `darwin_amd64` |
| macOS   | ARM64  | `darwin_arm64` |
| Windows | x86_64 | `windows_amd64`|
| Windows | ARM64  | `windows_arm64`|

## Release Workflow Examples

gpipe fits into any build pipeline. The pattern is always: **build → gpipe → release**.

### GoReleaser (Go projects)

Use `goreleaser build` (not `release`) to produce binaries without touching GitHub, then let gpipe and `gh` handle the rest.

```yaml
- name: Build binaries
  uses: goreleaser/goreleaser-action@v7
  with:
    version: "~> v2"
    args: build --clean
  env:
    GORELEASER_CURRENT_TAG: ${{ github.ref_name }}

- uses: thomaslaurenson/gpipe-action@v1
  with:
    cosign_sign: true

- name: Create release
  run: |
    gh release create "${{ github.ref_name }}" \
      dist/mycli-* install.sh install.ps1 \
      checksums.txt checksums.txt.sigstore.json
  env:
    GH_TOKEN: ${{ github.token }}
```

Recommended `.goreleaser.yml` for this pattern — named binaries in the dist root, no subdirectories:

```yaml
builds:
  - env:
      - CGO_ENABLED=0
    binary: >-
      mycli-{{ .Os }}-{{ if eq .Arch "amd64" }}x86_64{{ else if eq .Arch "arm64" }}aarch64{{ else }}{{ .Arch }}{{ end }}
    no_unique_dist_dir: true
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
```

This produces `dist/mycli-linux-x86_64`, `dist/mycli-darwin-aarch64`, `dist/mycli-windows-x86_64.exe`, etc. — paths that map directly to `.gpipe.yml` without translation.

### CMake (C++ projects)

```yaml
- name: Build binaries
  run: |
    cmake -B build -DCMAKE_BUILD_TYPE=Release
    cmake --build build --config Release

- uses: thomaslaurenson/gpipe-action@v1
  with:
    cosign_sign: true

- name: Create release
  run: |
    gh release create "${{ github.ref_name }}" \
      build/mycli-* install.sh install.ps1 \
      checksums.txt checksums.txt.sigstore.json
  env:
    GH_TOKEN: ${{ github.token }}
```

The `.gpipe.yml` `path` entries point to wherever CMake places your built binaries. A common pattern is staging them into a consistent output directory as a post-build step.

## Hook Authoring

Hooks are shell snippets injected verbatim into the generated install scripts. Place them in a `.gpipe/` directory:

```
.gpipe/
  pre-install.sh
  post-install.sh
  pre-install.ps1
  post-install.ps1
```

**Available variables in hooks:**

| Variable | Available in | Description |
|---|---|---|
| `GITHUB_REPO` / `$GithubRepo` | pre + post | `owner/repo` |
| `VERSION` / `$Version` | pre + post | release version tag |
| `BINARY` / `$Binary` | pre + post | canonical binary name |
| `INSTALL_NAME` / `$InstallName` | pre + post | name written to disk |
| `INSTALL_DIR` / `$InstallDir` | post only | directory binary was installed to |

**Important notes for hook authors:**
- Bash hooks are validated with `bash -n` before generation
- Bash hooks run in a subshell — `set -euo pipefail` is inherited. Any non-zero exit aborts the install
- PowerShell hooks run inside `& { }` — `$ErrorActionPreference = "Stop"` is active. Any terminating error aborts the install
- Pre-install hooks run before the temp directory is created — do not assume a temp dir is available
- Hooks are wrapped in delimited comment blocks in the generated output for easy identification

## Shell Completions

Enable per-shell completions in `.gpipe.yml`. The generated script runs `{binary} completion {shell}` after install and writes the output to the appropriate location. If the binary does not support the completion subcommand the install continues with a warning.

## Local Testing

```bash
# Validate config and hooks without generating files
gpipe validate

# Full generation without requiring all binaries to be present
gpipe generate --dry-run

# Verify goreleaser output structure before pushing a tag
goreleaser build --snapshot --clean
ls dist/
```

If `--repo` and `--version` are not supplied, they are auto-detected from the local git state:
- `repo` is parsed from `git remote get-url origin`
- `version` is derived from `git describe --tags`, with `-dev` appended if not on an exact tag

Detection output is always printed so you can confirm the values before scripts are generated.

## gpipe-action

See [`gpipe-action`](https://github.com/thomaslaurenson/gpipe-action) for the composite GitHub Action that wraps this tool for use in release workflows.
