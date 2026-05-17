# gpipe

![Build Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&logo=github) ![Test Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&label=test&logo=github)

![Release Version](https://img.shields.io/github/v/release/thomaslaurenson/gpipe?style=flat&logo=github) ![Release downloads](https://img.shields.io/github/downloads/thomaslaurenson/gpipe/total?label=downloads&logo=github)

![Go Version](https://img.shields.io/github/go-mod/go-version/thomaslaurenson/gpipe?logo=go) ![Code Coverage](https://img.shields.io/badge/Coverage-60%25-blue?logo=go)

Automated, cross-platform, language-agnostic, opionated installer file generation for GitHub binaries

`gpipe` generates `install.sh`, `install.ps1`, and `checksums.txt` from base templates, injecting project-specific configuration and SHA256 checksums at generation time. Designed for **single raw binary distribution** via `curl | bash`. Language-agnostic.

## Quick Start

```yaml
# .gpipe.yml
binary: mycli

platforms:
  linux_amd64:
    path: ./dist/mycli_linux_amd64_v1/mycli
    name: mycli-linux-x86_64
  linux_arm64:
    path: ./dist/mycli_linux_arm64_v8.0/mycli
    name: mycli-linux-aarch64
  darwin_amd64:
    path: ./dist/mycli_darwin_amd64_v1/mycli
    name: mycli-darwin-x86_64
  darwin_arm64:
    path: ./dist/mycli_darwin_arm64_v8.0/mycli
    name: mycli-darwin-aarch64
  windows_amd64:
    path: ./dist/mycli_windows_amd64_v1/mycli.exe
    name: mycli-windows-x86_64.exe
```

```bash
go run github.com/thomaslaurenson/gpipe generate --repo owner/mycli --version v1.2.3
```

Outputs `install.sh`, `install.ps1`, and `checksums.txt` in the current directory.

## `.gpipe.yml` Reference

```yaml
binary: mycli              # required, canonical binary name
install-name: mycli        # optional, name on disk after install (defaults to binary)
repo: owner/mycli          # optional, GitHub repo in owner/repo format; CLI --repo always overrides
version: v1.2.3            # optional, release version tag; CLI --version always overrides
sign: false                # optional, run cosign on generated checksums.txt; CLI --sign always overrides

platforms:                 # required, map of platform to path/name entry
  linux_amd64:
    path: ./dist/mycli_linux_amd64_v1/mycli      # local binary path
    name: mycli-linux-x86_64                      # GitHub release asset name
  linux_arm64:
    path: ./dist/mycli_linux_arm64_v8.0/mycli
    name: mycli-linux-aarch64
  darwin_amd64:
    path: ./dist/mycli_darwin_amd64_v1/mycli
    name: mycli-darwin-x86_64
  darwin_arm64:
    path: ./dist/mycli_darwin_arm64_v8.0/mycli
    name: mycli-darwin-aarch64
  windows_amd64:
    path: ./dist/mycli_windows_amd64_v1/mycli.exe
    name: mycli-windows-x86_64.exe
  windows_arm64:
    path: ./dist/mycli_windows_arm64_v8.0/mycli.exe
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

## Platform Matrix

| OS | Arch | Identifier |
|---|---|---|
| Linux | x86_64 | `linux_amd64` |
| Linux | ARM64 | `linux_arm64` |
| macOS | x86_64 | `darwin_amd64` |
| macOS | ARM64 | `darwin_arm64` |
| Windows | x86_64 | `windows_amd64` |
| Windows | ARM64 | `windows_arm64` |

## Hook Authoring

Hooks are shell snippets injected into the generated install scripts. Place them in a `.gpipe/` directory:

```
.gpipe/
  pre-install.sh
  post-install.sh
  pre-install.ps1
  post-install.ps1
```

- `.sh` hooks are validated with `bash -n` before generation
- Hooks are wrapped in a subshell (bash) or script block (PowerShell) to prevent side-effects
- Empty hook files produce a warning and are skipped

## Shell Completions

Enable per-shell completions in `.gpipe.yml`. The generated script runs `{binary} completion {shell}` after install and writes the output to the appropriate path. If the binary does not support the completion subcommand, the install continues with a warning

## `--validate` and `--dry-run`

```bash
# Validate config and hooks only (no files generated)
go run github.com/thomaslaurenson/gpipe validate

# Full local generation with partial asset support
go run github.com/thomaslaurenson/gpipe generate --dry-run --version v0.0.0-dry-run --repo owner/mycli
```

`--validate` is suitable as a pre-commit hook or CI lint step.

## Asset Naming

The `name` field in each platform entry controls the asset filename used in download URLs. It does not need to match the local file path — this allows tools like GoReleaser (which places binaries in subdirectories) to work without a staging step.

## gpipe-action

See [`gpipe-action`](https://github.com/thomaslaurenson/gpipe-action) for the composite GitHub Action that wraps this tool.
