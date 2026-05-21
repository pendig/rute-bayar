#!/usr/bin/env bash
set -euo pipefail

REPO="pendig/rute-bayar"
VERSION="latest"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="rutebayar"
FORCE_LOCAL=false

usage() {
  cat <<'EOF'
Usage:
  install.sh [--version <tag>] [--dir <path>] [--local] [--help]

Options:
  --version <tag>   Release tag to install (default: latest)
  --dir <path>      Installation directory (default: /usr/local/bin)
  --local           Prefer ~/.local/bin instead of /usr/local/bin
  --help            Show this help message

Example:
  ./install.sh --version v0.1.4
  ./install.sh --local
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --local)
      FORCE_LOCAL=true
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "$FORCE_LOCAL" == "true" ]]; then
  INSTALL_DIR="$HOME/.local/bin"
fi

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS:$ARCH" in
  Linux:x86_64) PLATFORM="linux-amd64" ;;
  Linux:aarch64|Linux:arm64) PLATFORM="linux-arm64" ;;
  Darwin:x86_64) PLATFORM="darwin-amd64" ;;
  Darwin:arm64) PLATFORM="darwin-arm64" ;;
  *) echo "Unsupported platform: $OS $ARCH" >&2; exit 1 ;;
esac

if [[ -z "${VERSION}" ]]; then
  VERSION="latest"
fi

API_URL="https://api.github.com/repos/${REPO}/releases/${VERSION}"
if [[ "${VERSION}" == "latest" ]]; then
  ASSET_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}-${PLATFORM}"
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required." >&2
    exit 1
  fi
else
  ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}-${PLATFORM}"
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required." >&2
  exit 1
fi

if [[ "$VERSION" != "latest" ]]; then
  if ! curl -fsSL -I "$API_URL" >/dev/null 2>&1; then
    echo "Release ${VERSION} not found." >&2
    exit 1
  fi
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "Downloading ${ASSET_URL}"
curl -fsSL -o "$tmp" "$ASSET_URL"
chmod +x "$tmp"

mkdir -p "$INSTALL_DIR"

install_cmd="cp \"$tmp\" \"${INSTALL_DIR}/${BINARY_NAME}\""
if command -v install >/dev/null 2>&1; then
  install_cmd="install -m 0755 \"$tmp\" \"${INSTALL_DIR}/${BINARY_NAME}\""
fi

if ! (eval "$install_cmd"); then
  echo "Failed to write to ${INSTALL_DIR}. Trying fallback to \$HOME/.local/bin." >&2
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  install_cmd="install -m 0755 \"$tmp\" \"${INSTALL_DIR}/${BINARY_NAME}\""
  eval "$install_cmd"
  echo "Saved binary to ${INSTALL_DIR}/${BINARY_NAME}."
  echo "Add to PATH if needed:"
  echo "  echo 'export PATH=\"${HOME}/.local/bin:\$PATH\"' >> ~/.bashrc  # or ~/.zshrc"
  echo "  source ~/.bashrc  # or restart your shell"
else
  echo "Saved binary to ${INSTALL_DIR}/${BINARY_NAME}."
fi

"${INSTALL_DIR}/${BINARY_NAME}" version || true
EOF
