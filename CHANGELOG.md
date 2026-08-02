# Changelog

## 1.2.1 - 2026-08-02

### Added

- Stamp the generating gpipe version into both install scripts
- Notify gpipe-action on release so it can bump its pinned gpipe version

### Changed

- Rename the generated Version header to Release-Version

## 1.2.0 - 2026-07-29

### Added

- Validate PowerShell hooks before generation, matching the existing bash hook check
- Include the install scripts in checksums.txt so they can be verified before being run

### Changed

- Bind the cosign certificate identity to the release tag so signatures cannot be replayed
- Reject unknown keys in .gpipe.yml and non-github.com remotes, accept prerelease versions
- Limit each generated script to the platforms it can actually install
- Give every line of installer output a level label, and hide cosign and download chatter unless it fails

### Fixed

- Fix Windows PowerShell user installs, PATH scope, duplicate .exe naming, and silent failures
- Fix shell completion install paths and zsh fpath wiring so completions load
- Retry failed downloads and warn when another binary shadows the install directory on PATH

## 1.1.1 - 2026-07-06

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
