# gpipe

![Build Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat) ![Test Status](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/gpipe/tag.yml?style=flat&label=test)

![Release Version](https://img.shields.io/github/v/release/thomaslaurenson/gpipe?style=flat) ![Release Downloads](https://img.shields.io/github/downloads/thomaslaurenson/gpipe/total?label=downloads)

![Go Version](https://img.shields.io/github/go-mod/go-version/thomaslaurenson/gpipe) ![Code Coverage](https://img.shields.io/badge/coverage-60%25-blue)

Automated, cross-platform, language-agnostic, opionated installer file generation for GitHub binaries

`gpipe` generates `install.sh`, `install.ps1`, and `checksums.txt` from base templates, injecting project-specific configuration and SHA256 checksums at generation time. Designed for **single raw binary distribution** via `curl | bash`. Language-agnostic.

## Quick Start

```yaml
# .gpipe.yml
binary: mycli

platforms:
  linux_amd64: ./dist/mycli-linux-x86_64
  linux_arm64: ./dist/mycli-linux-aarch64
  darwin_amd64: ./dist/mycli-macos-x86_64
  darwin_arm64: ./dist/mycli-macos-arm64
  windows_amd64: ./dist/mycli-windows-x86_64.exe
  windows_arm64: ./dist/mycli-windows-aarch64.exe
```

```bash
go run github.com/thomaslaurenson/gpipe --repo owner/mycli --version v1.2.3
```

Outputs `install.sh`, `install.ps1`, and `checksums.txt` in the current directory.

## `.gpipe.yml` Reference

```yaml
binary: mycli              # required, canonical binary name
install-name: mycli        # optional, name on disk after install (defaults to binary)

platforms:                 # required, map of platform to local binary path
  linux_amd64:   ./dist/mycli-linux-x86_64
  linux_arm64:   ./dist/mycli-linux-aarch64
  darwin_amd64:  ./dist/mycli-macos-x86_64
  darwin_arm64:  ./dist/mycli-macos-arm64
  windows_amd64: ./dist/mycli-windows-x86_64.exe
  windows_arm64: ./dist/mycli-windows-aarch64.exe

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
go run github.com/thomaslaurenson/gpipe --validate

# Full local generation with partial asset support
go run github.com/thomaslaurenson/gpipe --dry-run --version v0.0.0-dry-run --repo owner/mycli
```

`--validate` is suitable as a pre-commit hook or CI lint step.

## Asset Naming

gpipe does not impose any naming convention on binary files. The asset name used in download URLs is derived from the filename of the mapped path in `.gpipe.yml`.

## gpipe-action

See [`gpipe-action`](https://github.com/thomaslaurenson/gpipe-action) for the composite GitHub Action that wraps this tool.
