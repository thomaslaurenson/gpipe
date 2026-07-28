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

@test "resolve_asset: windows_amd64 is rejected (excluded from install.sh's platform list)" {
  # windows_amd64 is a globally valid platform identifier, but install.sh's
  # case statement is built from ShPlatforms (non-windows only), so it must
  # be rejected here even though it is not "unknown" in the way freebsd_amd64
  # above is. Regression test for the per-OS platform-list split.
  run resolve_asset "windows_amd64"
  (( status != 0 ))
  [[ "${output}" =~ "Unsupported platform" ]]
}

# setup_downloader / _download
#
# These invoke the real curl/wget binaries directly, using
# setup_downloader's exact flag lines, rather than going through
# setup_downloader/_download themselves or mocking. A mock cannot catch an
# invalid flag combination the way invoking the real binary can - which is
# exactly what happened here once: wget's -nv is its own two-character
# short option and cannot be bundled with -O the way -q (a genuine single
# character) could, and no test invoked real wget to catch it.
# "PATH=/usr/bin:/bin command -v" bypasses the mock curl that setup() puts
# ahead of PATH for every other test in this file.

@test "setup_downloader: curl branch's flags are valid for the real curl binary" {
  local real_curl
  if ! real_curl="$(PATH="/usr/bin:/bin" command -v curl)"; then
    skip "curl not installed"
  fi

  local src="${BATS_TEST_TMPDIR}/curl_src"
  printf 'hello from curl test\n' > "${src}"
  local dest="${BATS_TEST_TMPDIR}/curl_dest"

  run "${real_curl}" -fsSL --retry 3 --connect-timeout 30 --speed-limit 1024 --speed-time 30 \
    "file://${src}" -o "${dest}"
  (( status == 0 ))
  grep -qF "hello from curl test" "${dest}"
}

@test "setup_downloader: wget branch's flags are valid for the real wget binary" {
  local real_wget
  if ! real_wget="$(PATH="/usr/bin:/bin" command -v wget)"; then
    skip "wget not installed"
  fi

  # wget (unlike curl) has no file:// support, so this cannot be a real
  # end-to-end download the way the curl test above is. Instead it targets
  # a loopback port that refuses the connection immediately (no network
  # access and no timeout wait) and asserts the failure is a genuine
  # connection failure, not wget rejecting its own flags before ever
  # attempting to connect - which is exactly the bug this test exists to
  # catch.
  local dest="${BATS_TEST_TMPDIR}/wget_dest"
  run "${real_wget}" --tries=3 --connect-timeout=30 --timeout=30 -nv -O "${dest}" "http://127.0.0.1:1/nope"
  (( status != 0 ))
  [[ ! "${output}" =~ "illegal option" ]]
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
  # INSTALL_NAME is readonly after sourcing; it is already set to "mytool"
  touch "${HOME}/.bashrc"
  # Put a fake binary named after INSTALL_NAME at exactly INSTALL_DIR so the
  # availability check inside manage_path resolves to it, not just something
  # of the same name (see the shadowing test below for that case).
  local tmp_bin="${BATS_TEST_TMPDIR}/sysbin"
  mkdir -p "${tmp_bin}"
  cp "${FIXTURE_DIR}/fake_binary" "${tmp_bin}/${INSTALL_NAME}"
  INSTALL_DIR="${tmp_bin}"
  export PATH="${tmp_bin}:${PATH}"
  manage_path
  [[ ! -s "${HOME}/.bashrc" ]]
}

@test "manage_path: warns when a different binary shadows INSTALL_DIR on PATH" {
  USER_INSTALL=false
  INSTALL_DIR="${BATS_TEST_TMPDIR}/realinstalldir"
  mkdir -p "${INSTALL_DIR}"
  cp "${FIXTURE_DIR}/fake_binary" "${INSTALL_DIR}/${INSTALL_NAME}"

  # A different directory earlier in PATH shadows INSTALL_DIR
  local shadow_bin="${BATS_TEST_TMPDIR}/shadowbin"
  mkdir -p "${shadow_bin}"
  cp "${FIXTURE_DIR}/fake_binary" "${shadow_bin}/${INSTALL_NAME}"
  export PATH="${shadow_bin}:${PATH}:${INSTALL_DIR}"

  run manage_path
  (( status == 0 ))
  [[ "${output}" =~ "resolves to" ]]
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

@test "manage_path: zsh writes to only one of .zshrc/.zprofile when both exist" {
  USER_INSTALL=true
  INSTALL_DIR="${HOME}/.local/bin"
  export SHELL="/bin/zsh"
  touch "${HOME}/.zshrc" "${HOME}/.zprofile"
  export PATH="${PATH//${INSTALL_DIR}/}"
  manage_path
  local hits=0
  grep -qF "${INSTALL_DIR}" "${HOME}/.zshrc"    && hits=$((hits + 1))
  grep -qF "${INSTALL_DIR}" "${HOME}/.zprofile" && hits=$((hits + 1))
  (( hits == 1 ))
}

# install_bash_completions / install_zsh_completions
#
# INSTALL_NAME and BINARY are readonly after sourcing (both "mytool" in the
# fixture), so INSTALL_DIR is pointed at a scratch dir containing a mock
# binary named "mytool" that answers `completion <shell>`.

@test "install_bash_completions: user install writes to XDG completions dir" {
  USER_INSTALL=true
  INSTALL_DIR="${BATS_TEST_TMPDIR}/installdir"
  mkdir -p "${INSTALL_DIR}"
  cp "${REPO_ROOT}/test/helpers/mock_completion_binary" "${INSTALL_DIR}/${INSTALL_NAME}"
  unset XDG_DATA_HOME
  run install_bash_completions
  (( status == 0 ))
  [[ -f "${HOME}/.local/share/bash-completion/completions/${BINARY}" ]]
  [[ ! -e "${HOME}/.bash_completion.d/${BINARY}" ]]
}

@test "install_bash_completions: respects XDG_DATA_HOME when set" {
  USER_INSTALL=true
  INSTALL_DIR="${BATS_TEST_TMPDIR}/installdir"
  mkdir -p "${INSTALL_DIR}"
  cp "${REPO_ROOT}/test/helpers/mock_completion_binary" "${INSTALL_DIR}/${INSTALL_NAME}"
  export XDG_DATA_HOME="${BATS_TEST_TMPDIR}/xdgdata"
  run install_bash_completions
  (( status == 0 ))
  [[ -f "${XDG_DATA_HOME}/bash-completion/completions/${BINARY}" ]]
}

@test "install_bash_completions: warns and returns 0 when binary lacks completion support" {
  USER_INSTALL=true
  INSTALL_DIR="${BATS_TEST_TMPDIR}/installdir"
  mkdir -p "${INSTALL_DIR}"
  printf '#!/usr/bin/env bash\nexit 1\n' > "${INSTALL_DIR}/${INSTALL_NAME}"
  chmod +x "${INSTALL_DIR}/${INSTALL_NAME}"
  run install_bash_completions
  (( status == 0 ))
  [[ "${output}" =~ "not available" ]]
}

@test "install_zsh_completions: user install writes to ~/.zfunc and wires fpath" {
  USER_INSTALL=true
  INSTALL_DIR="${BATS_TEST_TMPDIR}/installdir"
  mkdir -p "${INSTALL_DIR}"
  cp "${REPO_ROOT}/test/helpers/mock_completion_binary" "${INSTALL_DIR}/${INSTALL_NAME}"
  run install_zsh_completions
  (( status == 0 ))
  [[ -f "${HOME}/.zfunc/_${BINARY}" ]]
  grep -qF '.zfunc' "${HOME}/.zshrc"
  grep -qF 'compinit' "${HOME}/.zshrc"
}

@test "install_zsh_completions: does not duplicate fpath wiring when already present" {
  USER_INSTALL=true
  INSTALL_DIR="${BATS_TEST_TMPDIR}/installdir"
  mkdir -p "${INSTALL_DIR}"
  cp "${REPO_ROOT}/test/helpers/mock_completion_binary" "${INSTALL_DIR}/${INSTALL_NAME}"
  printf 'fpath+=(~/.zfunc)\n' > "${HOME}/.zshrc"
  install_zsh_completions
  (( $(grep -cF '.zfunc' "${HOME}/.zshrc") == 1 ))
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
