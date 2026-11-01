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
# `|| true` on every fetch: a 404 here is an expected state (no release yet, or a
# bad pin), and pipefail would otherwise abort before the message below.
if [ "$WANT_VERSION" = "latest" ]; then
  rel=$(curl -fsSL "$api/releases/latest" 2>/dev/null || true)
else
  pin="$WANT_VERSION"; case "$pin" in v*) ;; *) pin="v$pin" ;; esac
  rel=$(curl -fsSL "$api/releases/tags/$pin" 2>/dev/null || true)
fi
[ -n "${rel:-}" ] || die "no release '$WANT_VERSION' found for $REPO (or the GitHub API rate limit bit).
  With Go installed you can build from source:  go install github.com/$REPO@latest"

# Take the asset list from the release itself rather than constructing a name:
# a tag and the version baked into the filenames can legitimately differ.
urls=$(printf '%s\n' "$rel" | grep -o '"browser_download_url": *"[^"]*"' | sed 's/.*"\(http[^"]*\)"$/\1/' || true)
asset_url=$(printf '%s\n' "$urls" | grep -E "_${os}_${arch}\.tar\.gz$" | sed -n 1p || true)
[ -n "$asset_url" ] || die "no release asset for $os/$arch — available:
$(printf '%s\n' "$urls" | sed 's#.*/##' | sed 's/^/    /')"
asset=${asset_url##*/}
tag=$(printf '%s' "$asset_url" | sed 's#.*/download/##; s#/.*##')
sums_url=$(printf '%s\n' "$urls" | grep -E '/checksums\.txt$' | sed -n 1p || true)

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "installing vibe-check $tag ($os/$arch)"
curl -fSL --progress-bar -o "$tmp/$asset" "$asset_url" || die "download failed: $asset_url"

# Verify against the release checksums when they are reachable. Never install an
# artifact we cannot account for.
sha() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d' ' -f1
  else printf ''; fi
}
want=""
if [ -n "${sums_url:-}" ]; then
  want=$(curl -fsSL "$sums_url" 2>/dev/null | awk -v a="$asset" '$2 == a {print $1}' || true)
fi
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
