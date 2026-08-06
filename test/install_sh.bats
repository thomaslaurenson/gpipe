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
# Uses BATS_TEST_TMPDIR as HOME so any stray write to a dotfile is contained
# and visible to the tests that assert none happens.
#
# Globals set:
#   REPO_ROOT, FIXTURE_DIR, PATH, HOME, GITHUB_REPO, VERSION,
#   BINARY, USER_INSTALL, NO_VERIFY, INSTALL_DIR, PLATFORM
setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/.." && pwd)"
  FIXTURE_DIR="${REPO_ROOT}/test/fixtures"

  export GPIPE_FIXTURE_DIR="${FIXTURE_DIR}"

  # Isolate HOME so any write that escapes goes to a throwaway directory.
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

  # Note: GITHUB_REPO, VERSION, BINARY are readonly after sourcing
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
  [[ "${output}" =~ "Checksum verified" ]]
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
#
# manage_path reports only. BINARY is readonly after sourcing (it is "mytool"
# in the fixture), so INSTALL_DIR is pointed at scratch dirs holding a binary
# of that name.

@test "manage_path: silent when the installed binary is what PATH resolves to" {
  INSTALL_DIR="${BATS_TEST_TMPDIR}/installdir"
  mkdir -p "${INSTALL_DIR}"
  cp "${FIXTURE_DIR}/fake_binary" "${INSTALL_DIR}/${BINARY}"
  export PATH="${INSTALL_DIR}:${PATH}"
  run manage_path
  (( status == 0 ))
  [[ -z "${output}" ]]
}

@test "manage_path: warns when a different binary shadows INSTALL_DIR on PATH" {
  INSTALL_DIR="${BATS_TEST_TMPDIR}/realinstalldir"
  mkdir -p "${INSTALL_DIR}"
  cp "${FIXTURE_DIR}/fake_binary" "${INSTALL_DIR}/${BINARY}"

  # A different directory earlier in PATH shadows INSTALL_DIR
  local shadow_bin="${BATS_TEST_TMPDIR}/shadowbin"
  mkdir -p "${shadow_bin}"
  cp "${FIXTURE_DIR}/fake_binary" "${shadow_bin}/${BINARY}"
  export PATH="${shadow_bin}:${PATH}:${INSTALL_DIR}"

  run manage_path
  (( status == 0 ))
  [[ "${output}" =~ "resolves to" ]]
}

@test "manage_path: prints the export line when INSTALL_DIR is not on PATH" {
  INSTALL_DIR="${BATS_TEST_TMPDIR}/unreachable"
  mkdir -p "${INSTALL_DIR}"
  cp "${FIXTURE_DIR}/fake_binary" "${INSTALL_DIR}/${BINARY}"
  run manage_path
  (( status == 0 ))
  [[ "${output}" =~ "is not in PATH" ]]
  [[ "${output}" =~ "export PATH=" ]]
}

# The whole point of the v2 trim: report, never edit. A dotfile the user did
# not ask the installer to touch must come out of a full run untouched.
@test "manage_path: writes to no shell profile" {
  INSTALL_DIR="${BATS_TEST_TMPDIR}/unreachable"
  mkdir -p "${INSTALL_DIR}"
  cp "${FIXTURE_DIR}/fake_binary" "${INSTALL_DIR}/${BINARY}"
  touch "${HOME}/.bashrc" "${HOME}/.bash_profile" "${HOME}/.profile" \
        "${HOME}/.zshrc" "${HOME}/.zprofile"
  mkdir -p "${HOME}/.config/fish"
  touch "${HOME}/.config/fish/config.fish"

  for shell in /bin/bash /bin/zsh /usr/bin/fish; do
    export SHELL="${shell}"
    manage_path
  done

  local f
  for f in "${HOME}/.bashrc" "${HOME}/.bash_profile" "${HOME}/.profile" \
           "${HOME}/.zshrc" "${HOME}/.zprofile" "${HOME}/.config/fish/config.fish"; do
    [[ ! -s "${f}" ]] || {
      printf 'manage_path wrote to %s\n' "${f}" >&2
      return 1
    }
  done
}

@test "fixture: contains no completion or dotfile-writing logic" {
  ! grep -qE 'completion|\.bashrc|\.zshrc|\.zfunc|config\.fish' \
    "${FIXTURE_DIR}/install_rendered.sh"
}

# install_rendered.sh: hook injection
#
# These tests verify that the rendered fixture (rendered with pre/post hooks
# injected from test/fixtures/hooks/) contains the expected sentinels and hook
# content. File-content checks use grep directly.

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
