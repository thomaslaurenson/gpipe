# Changelog

## Unreleased

### Added

- Add windows/arm64 to the release build matrix
- Add an MIT license

### Changed

- Consolidate install script test fixtures into a single generated set

### Fixed

- Harden repo, binary, and asset name validation against shell metacharacter injection
- Anchor the cosign certificate identity pattern so it matches the target repository
- Match checksums and PATH entries exactly instead of by substring
- Fix install.ps1 parameter binding by declaring the param block at script

### Updated

- Update actions/checkout to v7 in all workflows

## 1.1.0 - 2026-05-28

### Added

- Bash testing using BATS
- Powershell testing using Pester

### Updated

- Modularised install scripts to improve testing capability
- General test coverage in gpipe tool

## 1.0.0 - 2026-05-17

### Added

- Initial release
