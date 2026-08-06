# gpipe

![Build Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&logo=github) ![Test Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&label=test&logo=github)

![Release Version](https://img.shields.io/github/v/release/thomaslaurenson/gpipe?style=flat&logo=github) ![Release Downloads](https://img.shields.io/github/downloads/thomaslaurenson/gpipe/total?label=downloads&logo=github)

![Go Version](https://img.shields.io/github/go-mod/go-version/thomaslaurenson/gpipe?logo=go) ![Code Coverage](https://img.shields.io/badge/Coverage-75.1%25-blue?logo=go)

Generates `install.sh`, `install.ps1`, and `checksums.txt` for GitHub binary releases, so a project can be installed with `curl | bash`. Language-agnostic: it only needs built binaries on disk, whatever produced them.

The generated installers detect the platform, download the release asset, verify its cosign signature and SHA-256, and install the binary.

## Quick start

Add `.gpipe.yml` to the repo root, listing each platform's built binary and its release asset name:

```yaml
binary: mycli

platforms:
  linux_amd64:
    path: ./dist/mycli-linux-x86_64
    name: mycli-linux-x86_64
  darwin_arm64:
    path: ./dist/mycli-darwin-aarch64
    name: mycli-darwin-aarch64
  windows_amd64:
    path: ./dist/mycli-windows-x86_64.exe
    name: mycli-windows-x86_64.exe
```

Then build, run gpipe, and create the release:

```yaml
name: Release

on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-24.04
    permissions:
      contents: write
      id-token: write  # only needed for cosign_sign
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod

      - name: Build binaries
        uses: goreleaser/goreleaser-action@v7
        with:
          version: "~> v2"
          args: build --clean
        env:
          GORELEASER_CURRENT_TAG: ${{ github.ref_name }}

      - uses: thomaslaurenson/gpipe@v1
        with:
          cosign_sign: true

      - name: Create release
        run: |
          gh release create "${{ github.ref_name }}" \
            dist/mycli-* \
            install.sh install.ps1 \
            checksums.txt checksums.txt.sigstore.json
        env:
          GH_TOKEN: ${{ github.token }}
```

The pattern is **build -> gpipe -> release** for any toolchain; only the build step changes for CMake, Cargo, or plain `make`.

For Go projects, this `.goreleaser.yml` puts named binaries in the dist root so the paths map straight into `.gpipe.yml`:

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

## Configuration

`.gpipe.yml` has three keys. `path` is the local build output, `name` is the release asset filename:

```yaml
binary: mycli              # required: binary name, also the name written to disk

platforms:                 # required: at least one
  linux_amd64:             # linux_amd64, linux_arm64, darwin_amd64,
    path: ./dist/mycli-linux-x86_64    # darwin_arm64, windows_amd64, windows_arm64
    name: mycli-linux-x86_64

hooks:                     # optional
  pre-sh:   .gpipe/pre-install.sh    # before download in install.sh
  post-sh:  .gpipe/post-install.sh   # after install in install.sh
  pre-ps1:  .gpipe/pre-install.ps1   # before download in install.ps1
  post-ps1: .gpipe/post-install.ps1  # after install in install.ps1
```

Each generated script carries only the platforms it can install, so `install.sh` never gets a `windows_*` branch. `repo` and `version` are runtime-only and come from flags or git, never from this file. Unknown keys are a parse error rather than being ignored.

## Action inputs

| Input | Default | Description |
|---|---|---|
| `version` | `${{ github.ref_name }}` | Release version tag |
| `repo` | `${{ github.repository }}` | `owner/repo` |
| `config` | `.gpipe.yml` | Config path relative to repo root |
| `cosign_sign` | `false` | Sign `checksums.txt`; needs `id-token: write` |

The action builds gpipe from its own checkout, so `@v1` gets the latest v1.x and `@v1.4.0` pins exactly. It needs Go on the runner, which GitHub-hosted runners provide.

## CLI

```text
gpipe generate  --repo <owner/repo> --version <vX.Y.Z> [--config <path>] [--sign] [--dry-run]
gpipe validate  [--repo <owner/repo>] [--version <vX.Y.Z>] [--config <path>]
gpipe version
```

Install it with `curl -fsSL https://github.com/thomaslaurenson/gpipe/releases/latest/download/install.sh | bash`, or grab a binary from the [releases page](https://github.com/thomaslaurenson/gpipe/releases).

`--repo` and `--version` fall back to the git remote and `git describe --tags`, printing what they detected. `validate` checks the config without generating; `--dry-run` generates without needing every binary present.

`checksums.txt` covers `install.sh` and `install.ps1` themselves and is cosign-signed, so an installer can be verified before it is run. The generated `install.sh` carries the exact `cosign verify-blob` invocation, identity regexp included, in its `verify_signature` function.

## Hooks

Hook files are injected verbatim into the generated scripts. Available variables:

| Variable | Available in | Description |
|---|---|---|
| `GITHUB_REPO` / `$script:GithubRepo` | pre, post | `owner/repo` |
| `VERSION` / `$script:Version` | pre, post | release version tag |
| `BINARY` / `$script:Binary` | pre, post | binary name |
| `INSTALL_DIR` / `$installDir` | post | where the binary was installed |

Two things to know when writing one:

- Bash hooks run in a subshell inheriting `set -euo pipefail`, and PowerShell hooks inside `& { }` with `$ErrorActionPreference = 'Stop'`. Anything optional needs guarding, or a failure aborts the install after the binary is already placed. Use `exit 0` to leave the hook without stopping the installer.
- The `ok`, `info`, `warn`, and `error` helpers are in scope, so hook output matches the installer's `[LEVEL]` formatting.

Syntax is checked before generation with `bash -n` and the PowerShell parser, the latter only when `pwsh` is available.

## Local development

```bash
make build   # build to bin/gpipe
make ci      # fmt, vet, lint, and the full test suite
make help    # everything else
```

Tests are Go plus [bats-core](https://github.com/bats-core/bats-core) for `install.sh` and Pester for `install.ps1`. bats is a submodule, so a fresh clone needs `git submodule update --init --recursive`. The PowerShell targets need `pwsh` with Pester 5+ and PSScriptAnalyzer.
