#!/usr/bin/env bash
# Install vibe-check from the latest GitHub release.
#   curl -fsSL https://raw.githubusercontent.com/gaurav-dhakal/vibe-check/main/install.sh | bash
# Honors: VIBE_CHECK_VERSION (tag or "latest"), VIBE_CHECK_INSTALL_DIR, REPO.
set -euo pipefail

REPO="${REPO:-gaurav-dhakal/vibe-check}"
WANT_VERSION="${VIBE_CHECK_VERSION:-latest}"
INSTALL_DIR="${VIBE_CHECK_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '\033[1;36m»\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m✘\033[0m %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar  >/dev/null 2>&1 || die "tar is required"

case "$(uname -s)" in
  Linux)   os=linux ;;
  Darwin)  os=darwin ;;
  FreeBSD) os=freebsd ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT) die "on Windows use:  irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex" ;;
  *) die "unsupported OS: $(uname -s) — see the README for building from source" ;;
esac
case "$(uname -m)" in
  x86_64|amd64)          arch=amd64 ;;
  arm64|aarch64)         arch=arm64 ;;
  *) die "unsupported CPU: $(uname -m)" ;;
esac

api="https://api.github.com/repos/$REPO"
if [ "$WANT_VERSION" = "latest" ]; then
  # `|| true` everywhere: a 404 here is an expected state (no release yet), and
  # pipefail would otherwise abort before the message below.
  latest=$(curl -fsSL "$api/releases/latest" 2>/dev/null || true)
  tag=$(printf '%s\n' "$latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | sed -n 1p) || true
  [ -n "${tag:-}" ] || die "no published release found for $REPO (or the GitHub API rate limit bit).
  With Go installed you can build from source:  go install github.com/$REPO@latest"
else
  tag="$WANT_VERSION"
  case "$tag" in v*) ;; *) tag="v$tag" ;; esac
fi

ver="${tag#v}"
asset="vibe-check_${ver}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$asset"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "installing vibe-check $tag ($os/$arch)"
curl -fSL --progress-bar -o "$tmp/$asset" "$url" || die "download failed: $url"

# Verify against the release checksums when the asset list is reachable. Never
# install an artifact we cannot account for.
sha() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d' ' -f1
  else printf ''; fi
}
want=$(curl -fsSL "https://github.com/$REPO/releases/download/$tag/checksums.txt" 2>/dev/null \
       | awk -v a="$asset" '$2 == a {print $1}' || true)
if [ -n "$want" ]; then
  got=$(sha "$tmp/$asset")
  [ "$got" = "$want" ] || die "checksum mismatch for $asset — refusing to install (want $want, got ${got:-no sha256 tool})"
  say "sha256 verified"
else
  say "warning: checksums.txt unavailable, installed unverified"
fi

tar -xzf "$tmp/$asset" -C "$tmp"
[ -f "$tmp/vibe-check" ] || die "archive did not contain a vibe-check binary"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/vibe-check" "$INSTALL_DIR/vibe-check" 2>/dev/null \
  || { cp "$tmp/vibe-check" "$INSTALL_DIR/vibe-check"; chmod 0755 "$INSTALL_DIR/vibe-check"; }
cp -f "$INSTALL_DIR/vibe-check" "$INSTALL_DIR/vcheck"
chmod 0755 "$INSTALL_DIR/vcheck"

say "installed to $INSTALL_DIR/vibe-check"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) printf '\n  %s is not on PATH. Add it with:\n\n    export PATH="%s:$PATH"\n    # and put that line in ~/.bashrc or ~/.zshrc\n\n' "$INSTALL_DIR" "$INSTALL_DIR" ;;
esac

"$INSTALL_DIR/vibe-check" --version || die "installed binary did not run"
say "done — run  vibe-check   (or  vcheck)"
