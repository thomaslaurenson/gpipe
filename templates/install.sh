#!/usr/bin/env bash


# {{GPIPE_GENERATED_BY}}

# Wrapping the script body in a brace block causes the shell to buffer the
# complete download before starting execution, preventing partial runs if the
# connection drops mid-pipe
{
set -euo pipefail

# Constants: baked in at generation time
readonly GITHUB_REPO="{{GITHUB_REPO}}"
readonly VERSION="{{VERSION}}"
readonly BINARY="{{BINARY}}"
readonly INSTALL_NAME="{{INSTALL_NAME}}"

# Output helpers
if [[ -t 2 ]] && [[ -z "${NO_COLOR:-}" ]] && [[ "${TERM:-}" != "dumb" ]]; then
  _RED='\033[0;31m' _GREEN='\033[0;32m' _YELLOW='\033[1;33m'
  _CYAN='\033[0;36m' _BOLD='\033[1m' _NC='\033[0m'
else
  _RED='' _GREEN='' _YELLOW='' _CYAN='' _BOLD='' _NC=''
fi

info()  { printf "${_GREEN}[INFO]${_NC}  %s\n" "$1"; }
warn()  { printf "${_YELLOW}[WARN]${_NC}  %s\n" "$1" >&2; }
error() { printf "${_RED}[ERROR]${_NC} %s\n" "$1" >&2; exit 1; }
step()  { printf "  ${_CYAN}>${_NC} %s\n" "$1"; }

# Usage
show_help() {
  cat <<EOF
${_BOLD}${INSTALL_NAME} installer${_NC}

USAGE:
  bash install.sh [OPTIONS]

OPTIONS:
  -h, --help    Show this help message
  --user        Install to ~/.local/bin (no sudo required)
  --system      Install to /usr/local/bin (default, may require sudo)

EXAMPLES:
  # System-wide install (default)
  curl -fsSL https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/install.sh | bash

  # User-local install, no sudo needed
  curl -fsSL https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/install.sh | bash -s -- --user

EOF
}

# Argument parsing
USER_INSTALL=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)   show_help; exit 0 ;;
    --user)      USER_INSTALL=true;  shift ;;
    --system)    USER_INSTALL=false; shift ;;
    *)           error "Unknown option: $1. Run with --help for usage." ;;
  esac
done

# Platform detection and normalisation
_OS="$(uname -s)"
_ARCH="$(uname -m)"

case "$_OS" in
  Linux)  OS="linux"  ;;
  Darwin) OS="darwin" ;;
  *)      error "Unsupported operating system: ${_OS}" ;;
esac

case "$_ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             error "Unsupported architecture: ${_ARCH}" ;;
esac

PLATFORM="${OS}_${ARCH}"

# Platform validation: baked in at generation time
{{SUPPORTED_PLATFORMS_BLOCK}}

# Asset map: baked in at generation time: platform -> release filename
{{ASSET_MAP_BLOCK}}

ASSET_NAME="${ASSET_NAMES[$PLATFORM]}"
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${ASSET_NAME}"
CHECKSUMS_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/checksums.txt"

# Pre-install hook
{{PRE_INSTALL_HOOK}}

# Download to temp directory
_TMPDIR="$(mktemp -d)"
trap 'rm -rf "$_TMPDIR"' EXIT

_download() {
  local url="$1" dest="$2"
  if command -v curl > /dev/null 2>&1; then
    curl -fsSL --max-time 300 "$url" -o "$dest"
  elif command -v wget > /dev/null 2>&1; then
    wget --timeout=300 -qO "$dest" "$url"
  else
    error "curl or wget is required to download files"
  fi
}

info "Downloading ${BINARY} ${VERSION} for ${PLATFORM}..."
step "${DOWNLOAD_URL}"
_download "${DOWNLOAD_URL}"  "${_TMPDIR}/${ASSET_NAME}"
_download "${CHECKSUMS_URL}" "${_TMPDIR}/checksums.txt"

# Checksum verification
info "Verifying checksum..."
EXPECTED_HASH="$(grep -F "  ${ASSET_NAME}" "${_TMPDIR}/checksums.txt" | awk '{print $1}')"
if [[ -z "$EXPECTED_HASH" ]]; then
  error "Checksum not found for ${ASSET_NAME} in checksums.txt"
fi

if command -v sha256sum > /dev/null 2>&1; then
  ACTUAL_HASH="$(sha256sum "${_TMPDIR}/${ASSET_NAME}" | awk '{print $1}')"
elif command -v shasum > /dev/null 2>&1; then
  ACTUAL_HASH="$(shasum -a 256 "${_TMPDIR}/${ASSET_NAME}" | awk '{print $1}')"
else
  warn "sha256sum/shasum not found: skipping checksum verification"
  ACTUAL_HASH="$EXPECTED_HASH"
fi

if [[ "$EXPECTED_HASH" != "$ACTUAL_HASH" ]]; then
  error "Checksum mismatch for ${ASSET_NAME}
  expected: ${EXPECTED_HASH}
  actual:   ${ACTUAL_HASH}"
fi
step "Checksum OK"

# Install
_try_install() {
  # Attempt to install src -> dir/name. Returns non-zero on failure
  local src="$1" dir="$2" name="$3"
  mkdir -p "$dir" 2>/dev/null || true
  install -m 0755 "$src" "${dir}/${name}"
}

INSTALL_DIR=""

if [[ "$USER_INSTALL" == "true" ]]; then
  _user_dir="${HOME}/.local/bin"
  _try_install "${_TMPDIR}/${ASSET_NAME}" "$_user_dir" "$INSTALL_NAME" \
    || error "Failed to install to ${_user_dir}: check directory permissions"
  INSTALL_DIR="$_user_dir"
else
  _sys_dir="/usr/local/bin"
  if _try_install "${_TMPDIR}/${ASSET_NAME}" "$_sys_dir" "$INSTALL_NAME" 2>/dev/null; then
    INSTALL_DIR="$_sys_dir"
  else
    # Permission failure: check for interactive terminal
    if [[ -t 2 ]]; then
      printf "\n${_YELLOW}Insufficient permissions to install to %s.${_NC}\n\n" "$_sys_dir" >&2
      printf "  1) Retry with sudo\n" >&2
      printf "  2) Install to ~/.local/bin (no sudo required)\n" >&2
      printf "  3) Quit\n\n" >&2
      read -rp "Choose [1/2/3]: " _choice </dev/tty
      case "$_choice" in
        1)
          info "Retrying with sudo..."
          sudo install -m 0755 "${_TMPDIR}/${ASSET_NAME}" "${_sys_dir}/${INSTALL_NAME}" \
            || error "sudo install failed"
          INSTALL_DIR="$_sys_dir"
          ;;
        2)
          USER_INSTALL=true
          _user_dir="${HOME}/.local/bin"
          _try_install "${_TMPDIR}/${ASSET_NAME}" "$_user_dir" "$INSTALL_NAME" \
            || error "Failed to install to ${_user_dir}: check directory permissions"
          INSTALL_DIR="$_user_dir"
          ;;
        *)
          error "Installation aborted." ;;
      esac
    else
      # No TTY (e.g. curl | bash): print instructions and exit non-zero
      printf "\n${_RED}[ERROR]${_NC} Insufficient permissions to install to %s\n\n" "$_sys_dir" >&2
      printf "To retry as root:\n" >&2
      printf "  sudo bash <(curl -fsSL https://github.com/%s/releases/download/%s/install.sh)\n\n" \
        "$GITHUB_REPO" "$VERSION" >&2
      printf "To install without sudo (user install):\n" >&2
      printf "  curl -fsSL https://github.com/%s/releases/download/%s/install.sh | bash -s -- --user\n" \
        "$GITHUB_REPO" "$VERSION" >&2
      exit 1
    fi
  fi
fi

info "Installed ${INSTALL_NAME} to ${INSTALL_DIR}/${INSTALL_NAME}"

# Shell completions: optional blocks, stripped at generation time if unused

# BEGIN_BLOCK:COMPLETION_BASH
if "${INSTALL_DIR}/${INSTALL_NAME}" completion bash > /dev/null 2>&1; then
  if [[ "$USER_INSTALL" == "true" ]]; then
    mkdir -p "${HOME}/.bash_completion.d"
    "${INSTALL_DIR}/${INSTALL_NAME}" completion bash > "${HOME}/.bash_completion.d/${BINARY}"
    step "Bash completions → ~/.bash_completion.d/${BINARY}"
  elif "${INSTALL_DIR}/${INSTALL_NAME}" completion bash 2>/dev/null \
      | sudo install -m 0644 /dev/stdin "/etc/bash_completion.d/${BINARY}" 2>/dev/null; then
    step "Bash completions → /etc/bash_completion.d/${BINARY}"
  else
    mkdir -p "${HOME}/.bash_completion.d"
    "${INSTALL_DIR}/${INSTALL_NAME}" completion bash > "${HOME}/.bash_completion.d/${BINARY}"
    step "Bash completions → ~/.bash_completion.d/${BINARY}"
  fi
else
  warn "Bash completion generation failed, skipping"
fi
# END_BLOCK:COMPLETION_BASH

# BEGIN_BLOCK:COMPLETION_ZSH
if "${INSTALL_DIR}/${INSTALL_NAME}" completion zsh > /dev/null 2>&1; then
  if [[ "$USER_INSTALL" == "true" ]]; then
    mkdir -p "${HOME}/.zfunc"
    "${INSTALL_DIR}/${INSTALL_NAME}" completion zsh > "${HOME}/.zfunc/_${BINARY}"
    step "Zsh completions → ~/.zfunc/_${BINARY}"
  elif "${INSTALL_DIR}/${INSTALL_NAME}" completion zsh 2>/dev/null \
      | sudo install -m 0644 /dev/stdin "/usr/share/zsh/site-functions/_${BINARY}" 2>/dev/null; then
    step "Zsh completions → /usr/share/zsh/site-functions/_${BINARY}"
  else
    mkdir -p "${HOME}/.zfunc"
    "${INSTALL_DIR}/${INSTALL_NAME}" completion zsh > "${HOME}/.zfunc/_${BINARY}"
    step "Zsh completions → ~/.zfunc/_${BINARY}"
  fi
else
  warn "Zsh completion generation failed, skipping"
fi
# END_BLOCK:COMPLETION_ZSH

# BEGIN_BLOCK:COMPLETION_FISH
if "${INSTALL_DIR}/${INSTALL_NAME}" completion fish > /dev/null 2>&1; then
  if [[ "$USER_INSTALL" == "true" ]]; then
    mkdir -p "${HOME}/.config/fish/completions"
    "${INSTALL_DIR}/${INSTALL_NAME}" completion fish > "${HOME}/.config/fish/completions/${BINARY}.fish"
    step "Fish completions → ~/.config/fish/completions/${BINARY}.fish"
  elif "${INSTALL_DIR}/${INSTALL_NAME}" completion fish 2>/dev/null \
      | sudo install -m 0644 /dev/stdin "/usr/share/fish/completions/${BINARY}.fish" 2>/dev/null; then
    step "Fish completions → /usr/share/fish/completions/${BINARY}.fish"
  else
    mkdir -p "${HOME}/.config/fish/completions"
    "${INSTALL_DIR}/${INSTALL_NAME}" completion fish > "${HOME}/.config/fish/completions/${BINARY}.fish"
    step "Fish completions → ~/.config/fish/completions/${BINARY}.fish"
  fi
else
  warn "Fish completion generation failed, skipping"
fi
# END_BLOCK:COMPLETION_FISH

# Post-install hook
{{POST_INSTALL_HOOK}}

# PATH verification and repair (user installs only)
if [[ "$USER_INSTALL" == "true" ]]; then
  if ! printf ':%s:' "$PATH" | grep -qF ":${INSTALL_DIR}:"; then
    _export_line="export PATH=\"${INSTALL_DIR}:\$PATH\""
    _modified=()

    case "${SHELL:-}" in
      */zsh)
        for _f in "${ZDOTDIR:-$HOME}/.zshrc" "${ZDOTDIR:-$HOME}/.zprofile"; do
          if [[ -f "$_f" ]] && ! grep -qF "$INSTALL_DIR" "$_f" 2>/dev/null; then
            printf '\n# Added by %s installer\n%s\n' "$INSTALL_NAME" "$_export_line" >> "$_f"
            _modified+=("$_f")
          fi
        done ;;
      */fish)
        _fish_cfg="${HOME}/.config/fish/config.fish"
        mkdir -p "${HOME}/.config/fish"
        if ! grep -qF "$INSTALL_DIR" "$_fish_cfg" 2>/dev/null; then
          printf '\n# Added by %s installer\nfish_add_path %s\n' "$INSTALL_NAME" "$INSTALL_DIR" >> "$_fish_cfg"
          _modified+=("$_fish_cfg")
        fi ;;
      *)
        for _f in "${HOME}/.bashrc" "${HOME}/.bash_profile" "${HOME}/.profile"; do
          if [[ -f "$_f" ]] && ! grep -qF "$INSTALL_DIR" "$_f" 2>/dev/null; then
            printf '\n# Added by %s installer\n%s\n' "$INSTALL_NAME" "$_export_line" >> "$_f"
            _modified+=("$_f")
            break
          fi
        done ;;
    esac

    if [[ ${#_modified[@]} -gt 0 ]]; then
      warn "${INSTALL_DIR} was not in PATH: added to: ${_modified[*]}"
      warn "Restart your shell, or run now: ${_export_line}"
    else
      warn "${INSTALL_DIR} is not in PATH. Add the following to your shell profile:"
      warn "  ${_export_line}"
    fi
  fi
else
  if ! command -v "$INSTALL_NAME" > /dev/null 2>&1; then
    warn "${INSTALL_NAME} is not reachable via PATH"
    warn "  ${INSTALL_DIR} may need to be added to PATH manually"
  fi
fi

info "Successfully installed ${INSTALL_NAME} ${VERSION}"

} # end download guard
