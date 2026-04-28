# Changelog

## v0.1.0 - 2026-04-28

### Added

- Generate install.sh and install.ps1 from base templates for six target platforms
- Compute SHA256 checksums and write checksums.txt in sha256sum format
- Read project configuration from .gpipe.yml with CLI flag overrides
- Hook injection support for pre/post install scripts in bash and PowerShell
- Opt-in shell completion support for bash, zsh, fish, and PowerShell
- Validate subcommand for config and hook validation without generating files
- Dry-run mode for local generation with partial asset support
- Silent fallback from system install to user-local install on permission failure
- Automatic PATH repair after user-local installs on Linux, macOS, and Windows
