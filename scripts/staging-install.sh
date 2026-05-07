#!/usr/bin/env bash
# Install the lrok CLI on Linux or macOS, configured for the STAGING
# environment.
#
# Same Go binary as production. Installed under the name `staging-lrok`
# so the env package detects "staging" from argv[0] and points at:
#   - https://api.staging.lrok.io
#   - tunnel.lrok.io:7001  (port 7001 is the staging differentiator;
#     hostname reuses the prod wildcard TLS cert)
#   - ~/.lrok-staging/config
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/orcs-to/lrok.io-cli/main/scripts/staging-install.sh | bash
#
# Optional env (mirrors install.sh):
#   LROK_VERSION       pin a release tag (default: latest)
#   LROK_INSTALL_DIR   install path (default: $HOME/.local/bin)
#   LROK_TELEMETRY=0   disable install lifecycle beacons

set -euo pipefail

REPO="orcs-to/lrok.io-cli"
INSTALL_DIR="${LROK_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${LROK_VERSION:-latest}"

err() { beacon failed; printf 'staging-lrok-install: %s\n' "$*" >&2; exit 1; }
info() { printf 'staging-lrok-install: %s\n' "$*"; }

# Lifecycle beacon — same shape as production install.sh, points at
# the staging tracker so install funnel for staging is observable
# independently of prod.
beacon() {
  [ "${LROK_TELEMETRY:-1}" = "0" ] && return 0
  local stage="$1"
  local body
  body="{\"channel\":\"sh-staging\",\"arch\":\"${arch:-unknown}\",\"stage\":\"$stage\"}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsS -m 3 -X POST -H 'Content-Type: application/json' \
      -d "$body" \
      'https://api.staging.lrok.io/api/v1/track/install' >/dev/null 2>&1 || true
  fi
}

# --- detect OS ---
uname_s=$(uname -s)
case "$uname_s" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS: $uname_s (use staging-install.ps1 on Windows)" ;;
esac

# --- detect arch ---
uname_m=$(uname -m)
case "$uname_m" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported arch: $uname_m" ;;
esac

asset="lrok-${os}-${arch}"
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
  DL() { wget -q -O "$2" "$1"; }
else
  err "neither curl nor wget found"
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

info "downloading $asset (staging)"
DL "$bin_url" "$tmpdir/$asset"

info "downloading checksums.txt"
DL "$sums_url" "$tmpdir/checksums.txt"

# --- verify SHA256 ---
if command -v sha256sum >/dev/null 2>&1; then
  shacmd=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  shacmd="shasum -a 256"
else
  err "no sha256sum / shasum available"
fi
expected=$(awk -v t="$asset" '$2 == t {print $1}' "$tmpdir/checksums.txt")
[ -n "$expected" ] || err "no checksum entry for $asset"
actual=$($shacmd "$tmpdir/$asset" | awk '{print $1}')
if [ "$expected" != "$actual" ]; then
  err "checksum mismatch (expected $expected, got $actual)"
fi
info "checksum OK"

# --- install as `staging-lrok` ---
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/$asset" "$INSTALL_DIR/staging-lrok"
info "installed to $INSTALL_DIR/staging-lrok"

case ":$PATH:" in
  *:"$INSTALL_DIR":*) ;;
  *)
    info "warning: $INSTALL_DIR is not on your PATH"
    info "  add this to your shell rc:  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

"$INSTALL_DIR/staging-lrok" version || true
beacon ok
