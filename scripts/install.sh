#!/usr/bin/env bash
# Install the lrok CLI on Linux or macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/orcs-to/lrok.io-cli/main/scripts/install.sh | bash
#
# Optional env:
#   LROK_VERSION   pin a release tag (default: latest)
#   LROK_INSTALL_DIR  install path (default: $HOME/.local/bin)

set -euo pipefail

REPO="orcs-to/lrok.io-cli"
INSTALL_DIR="${LROK_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${LROK_VERSION:-latest}"

err() { beacon failed; printf 'lrok-install: %s\n' "$*" >&2; exit 1; }
info() { printf 'lrok-install: %s\n' "$*"; }

# Anonymous install lifecycle beacon. Pings lrok.io with one of:
#   started / ok / failed
# alongside the installer channel ("sh") and detected arch — never IP,
# username, hostname, install path. Disable with LROK_TELEMETRY=0.
beacon() {
  [ "${LROK_TELEMETRY:-1}" = "0" ] && return 0
  local stage="$1"
  local body
  body="{\"channel\":\"sh\",\"arch\":\"${arch:-unknown}\",\"stage\":\"$stage\"}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsS -m 3 -X POST -H 'Content-Type: application/json' \
      -d "$body" \
      'https://api.lrok.io/api/v1/track/install' >/dev/null 2>&1 || true
  fi
}

# --- detect OS ---
uname_s=$(uname -s)
case "$uname_s" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS: $uname_s (use install.ps1 on Windows)" ;;
esac

# --- detect arch ---
uname_m=$(uname -m)
case "$uname_m" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported arch: $uname_m" ;;
esac

asset="lrok-${os}-${arch}"

# Beacon: arch detected, going for the download. We fire here (not at
# the very top) so the arch field carries a real value.
beacon started

# --- resolve download URLs ---
if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

bin_url="${base_url}/${asset}"
sums_url="${base_url}/checksums.txt"

# --- pick a downloader ---
if command -v curl >/dev/null 2>&1; then
  DL() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  DL() { wget -q "$1" -O "$2"; }
else
  err "need curl or wget on PATH"
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

info "downloading ${asset}"
DL "$bin_url" "$tmpdir/$asset"

info "downloading checksums.txt"
DL "$sums_url" "$tmpdir/checksums.txt"

# --- verify SHA256 ---
expected=$(grep " ${asset}\$" "$tmpdir/checksums.txt" | awk '{print $1}' | head -n1)
if [ -z "$expected" ]; then
  err "no checksum entry for ${asset}"
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmpdir/$asset" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmpdir/$asset" | awk '{print $1}')
else
  err "need sha256sum or shasum on PATH"
fi
if [ "$expected" != "$actual" ]; then
  err "checksum mismatch: expected $expected, got $actual"
fi
info "checksum OK"

# --- install ---
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/$asset" "$INSTALL_DIR/lrok"
info "installed to $INSTALL_DIR/lrok"

case ":$PATH:" in
  *:"$INSTALL_DIR":*) ;;
  *)
    info "warning: $INSTALL_DIR is not on your PATH"
    info "  add this to your shell rc:  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

"$INSTALL_DIR/lrok" version || true

# Beacon: install completed end-to-end. err() fires "failed" on any
# earlier exit; reaching this line is the success signal.
beacon ok
