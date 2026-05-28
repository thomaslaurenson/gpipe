#!/usr/bin/env bats

# Tests for templates/install.sh
#
# The script is sourced (not executed) via the BASH_SOURCE guard at the
# bottom of the template. Each test function is called directly, with
# external dependencies replaced by mock executables on PATH.

bats_require_minimum_version 1.7.0

# Configure the test environment before each test.
#
# Sources the rendered install.sh fixture so all functions are available.
# Places mock executables first on PATH to intercept curl, uname, cosign.
# Uses BATS_TEST_TMPDIR as HOME to keep RC file writes isolated.
#
# Globals set:
#   REPO_ROOT, FIXTURE_DIR, PATH, HOME, INSTALL_NAME, GITHUB_REPO, VERSION,
#   BINARY, USER_INSTALL, NO_VERIFY, INSTALL_DIR, PLATFORM
setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/.." && pwd)"
  FIXTURE_DIR="${REPO_ROOT}/test/fixtures"

  export GPIPE_FIXTURE_DIR="${FIXTURE_DIR}"

  # Isolate HOME so RC file writes go to a throwaway directory.
  export HOME="${BATS_TEST_TMPDIR}/home"
  mkdir -p "${HOME}"

  # Create a bin dir with mock executables named after the real commands they
  # replace. This mirrors the pass-env pattern: symlink, don't rename, so the
  # originals remain available under their real names if needed.
  local mock_bin="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "${mock_bin}"
  ln -sf "${REPO_ROOT}/test/helpers/mock_uname"  "${mock_bin}/uname"
  ln -sf "${REPO_ROOT}/test/helpers/mock_curl"   "${mock_bin}/curl"
  ln -sf "${REPO_ROOT}/test/helpers/mock_cosign" "${mock_bin}/cosign"
  export PATH="${mock_bin}:${PATH}"

  # The BASH_SOURCE guard prevents main from running.
  # shellcheck source=test/fixtures/install_rendered.sh
  source "${FIXTURE_DIR}/install_rendered.sh"

  # Note: GITHUB_REPO, VERSION, BINARY, INSTALL_NAME are readonly after sourcing
  USER_INSTALL=false
  NO_VERIFY=false
  INSTALL_DIR=""
  PLATFORM=""
}

# parse_args

@test "parse_args: --user sets USER_INSTALL to true" {
  parse_args --user
  [[ "${USER_INSTALL}" == "true" ]]
}

@test "parse_args: --system sets USER_INSTALL to false" {
  USER_INSTALL=true
  parse_args --system
  [[ "${USER_INSTALL}" == "false" ]]
}

@test "parse_args: --no-verify sets NO_VERIFY to true" {
  parse_args --no-verify
  [[ "${NO_VERIFY}" == "true" ]]
}

@test "parse_args: unknown flag exits non-zero with message" {
  run parse_args --unknown-flag
  (( status != 0 ))
  [[ "${output}" =~ "Unknown option" ]]
}

@test "parse_args: no arguments leaves defaults unchanged" {
  parse_args
  [[ "${USER_INSTALL}" == "false" ]]
  [[ "${NO_VERIFY}" == "false" ]]
}

# detect_platform

@test "detect_platform: Linux x86_64 maps to linux_amd64" {
  export MOCK_UNAME_S="Linux"
  export MOCK_UNAME_M="x86_64"
  run detect_platform
  (( status == 0 ))
  [[ "${output}" == "linux_amd64" ]]
}

@test "detect_platform: Linux aarch64 maps to linux_arm64" {
  export MOCK_UNAME_S="Linux"
  export MOCK_UNAME_M="aarch64"
  run detect_platform
  (( status == 0 ))
  [[ "${output}" == "linux_arm64" ]]
}

@test "detect_platform: Darwin arm64 maps to darwin_arm64" {
  export MOCK_UNAME_S="Darwin"
  export MOCK_UNAME_M="arm64"
  run detect_platform
  (( status == 0 ))
  [[ "${output}" == "darwin_arm64" ]]
}

@test "detect_platform: Darwin x86_64 maps to darwin_amd64" {
  export MOCK_UNAME_S="Darwin"
  export MOCK_UNAME_M="x86_64"
  run detect_platform
  (( status == 0 ))
  [[ "${output}" == "darwin_amd64" ]]
}

@test "detect_platform: unsupported OS exits non-zero with message" {
  export MOCK_UNAME_S="Windows_NT"
  export MOCK_UNAME_M="x86_64"
  run detect_platform
  (( status != 0 ))
  [[ "${output}" =~ "Unsupported operating system" ]]
}

@test "detect_platform: unsupported architecture exits non-zero with message" {
  export MOCK_UNAME_S="Linux"
  export MOCK_UNAME_M="riscv64"
  run detect_platform
  (( status != 0 ))
  [[ "${output}" =~ "Unsupported architecture" ]]
}

# resolve_asset

@test "resolve_asset: linux_amd64 returns expected asset name" {
  run resolve_asset "linux_amd64"
  (( status == 0 ))
  [[ "${output}" == "mytool_linux_amd64" ]]
}

@test "resolve_asset: darwin_arm64 returns expected asset name" {
  run resolve_asset "darwin_arm64"
  (( status == 0 ))
  [[ "${output}" == "mytool_darwin_arm64" ]]
}

@test "resolve_asset: unknown platform exits non-zero with message" {
  run resolve_asset "freebsd_amd64"
  (( status != 0 ))
  [[ "${output}" =~ "Unsupported platform" ]]
}

# verify_checksum

@test "verify_checksum: passes when hash matches" {
  run verify_checksum "${FIXTURE_DIR}" "fake_binary"
  (( status == 0 ))
  [[ "${output}" =~ "Checksum OK" ]]
}

@test "verify_checksum: fails when hash does not match" {
  # Point to the bad checksums file by copying it into a tmp dir alongside the binary.
  local tmp_dir="${BATS_TEST_TMPDIR}/checksum_fail"
  mkdir -p "${tmp_dir}"
  cp "${FIXTURE_DIR}/fake_binary" "${tmp_dir}/fake_binary"
  cp "${FIXTURE_DIR}/checksums_bad.txt" "${tmp_dir}/checksums.txt"
  run verify_checksum "${tmp_dir}" "fake_binary"
  (( status != 0 ))
  [[ "${output}" =~ "Checksum mismatch" ]]
}

@test "verify_checksum: fails when asset is missing from checksums.txt" {
  local tmp_dir="${BATS_TEST_TMPDIR}/checksum_missing"
  mkdir -p "${tmp_dir}"
  cp "${FIXTURE_DIR}/fake_binary" "${tmp_dir}/fake_binary"
  printf '' > "${tmp_dir}/checksums.txt"
  run verify_checksum "${tmp_dir}" "fake_binary"
  (( status != 0 ))
  [[ "${output}" =~ "Checksum not found" ]]
}

# verify_signature

@test "verify_signature: skips and warns when NO_VERIFY is true" {
  NO_VERIFY=true
  run verify_signature "${FIXTURE_DIR}"
  (( status == 0 ))
  [[ "${output}" =~ "Skipping cosign" ]]
}

@test "verify_signature: exits non-zero when cosign verification fails" {
  NO_VERIFY=false
  export MOCK_COSIGN_EXIT=1
  run verify_signature "${FIXTURE_DIR}"
  (( status != 0 ))
}

@test "verify_signature: passes when cosign exits 0" {
  NO_VERIFY=false
  export MOCK_COSIGN_EXIT=0
  run verify_signature "${FIXTURE_DIR}"
  (( status == 0 ))
}

# _try_install

@test "_try_install: copies binary to destination with executable permissions" {
  local dest_dir="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "${dest_dir}"
  _try_install "${FIXTURE_DIR}/fake_binary" "${dest_dir}" "mytool"
  [[ -f "${dest_dir}/mytool" ]]
  [[ -x "${dest_dir}/mytool" ]]
}

@test "_try_install: creates destination directory if absent" {
  local dest_dir="${BATS_TEST_TMPDIR}/newdir/bin"
  _try_install "${FIXTURE_DIR}/fake_binary" "${dest_dir}" "mytool"
  [[ -f "${dest_dir}/mytool" ]]
}

# manage_path

@test "manage_path: appends export line to .bashrc when not in PATH" {
  USER_INSTALL=true
  INSTALL_DIR="${HOME}/.local/bin"
  touch "${HOME}/.bashrc"
  export SHELL="/bin/bash"
  export PATH="${PATH//${INSTALL_DIR}/}"
  manage_path
  grep -qF "${INSTALL_DIR}" "${HOME}/.bashrc"
}

@test "manage_path: does not modify .bashrc when INSTALL_DIR already in PATH" {
  USER_INSTALL=true
  INSTALL_DIR="${BATS_TEST_TMPDIR}/already_present"
  touch "${HOME}/.bashrc"
  export SHELL="/bin/bash"
  export PATH="${INSTALL_DIR}:${PATH}"
  manage_path
  # File should be empty (untouched).
  [[ ! -s "${HOME}/.bashrc" ]]
}

@test "manage_path: skips RC modification for system install" {
  USER_INSTALL=false
  INSTALL_DIR="/usr/local/bin"
  # INSTALL_NAME is readonly after sourcing; it is already set to "mytool"
  touch "${HOME}/.bashrc"
  # Put a fake binary named after INSTALL_NAME on PATH so the availability
  # check inside manage_path does not fire a warning
  local tmp_bin="${BATS_TEST_TMPDIR}/sysbin"
  mkdir -p "${tmp_bin}"
  cp "${FIXTURE_DIR}/fake_binary" "${tmp_bin}/${INSTALL_NAME}"
  export PATH="${tmp_bin}:${PATH}"
  manage_path
  [[ ! -s "${HOME}/.bashrc" ]]
}

@test "manage_path: appends fish_add_path to config.fish for fish shell" {
  USER_INSTALL=true
  INSTALL_DIR="${HOME}/.local/bin"
  export SHELL="/usr/bin/fish"
  mkdir -p "${HOME}/.config/fish"
  touch "${HOME}/.config/fish/config.fish"
  export PATH="${PATH//${INSTALL_DIR}/}"
  manage_path
  grep -qF "fish_add_path" "${HOME}/.config/fish/config.fish"
}

# install_rendered.sh: hook and completion injection
#
# These tests verify that the rendered fixture (rendered with all completions
# enabled and pre/post hooks injected from test/fixtures/hooks/) contains
# the expected sentinels and hook content. File-content checks use grep
# directly. Function-availability checks source the fixture in a subprocess
# to avoid readonly variable conflicts with the already-sourced setup().

@test "fixture: bash-completions sentinel present in file" {
  grep -qF "# gpipe test: bash-completions" "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: zsh-completions sentinel present in file" {
  grep -qF "# gpipe test: zsh-completions" "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: fish-completions sentinel present in file" {
  grep -qF "# gpipe test: fish-completions" "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: pre-install-hook sentinel present in file" {
  grep -qF "# gpipe test: pre-install-hook" "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: post-install-hook sentinel present in file" {
  grep -qF "# gpipe test: post-install-hook" "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: pre-hook content injected" {
  grep -qF 'echo "gpipe-fixture-pre-hook"' "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: post-hook content injected" {
  grep -qF 'echo "gpipe-fixture-post-hook"' "${FIXTURE_DIR}/install_rendered.sh"
}

@test "fixture: install_bash_completions function defined when sourced" {
  run bash -c "source '${FIXTURE_DIR}/install_rendered.sh'; declare -f install_bash_completions"
  (( status == 0 ))
}

@test "fixture: install_zsh_completions function defined when sourced" {
  run bash -c "source '${FIXTURE_DIR}/install_rendered.sh'; declare -f install_zsh_completions"
  (( status == 0 ))
}

@test "fixture: install_fish_completions function defined when sourced" {
  run bash -c "source '${FIXTURE_DIR}/install_rendered.sh'; declare -f install_fish_completions"
  (( status == 0 ))
}
