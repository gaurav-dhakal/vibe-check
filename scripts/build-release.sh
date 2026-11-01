#!/usr/bin/env bash
# Build every supported platform into dist/ as GitHub-release assets.
#   dist/vibe-check_<ver>_<os>_<arch>.tar.gz   (unix)
#   dist/vibe-check_<ver>_windows_<arch>.zip   (windows)
#   dist/checksums.txt
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(grep -m1 '^\s*version\s*=\s*"' cmd/root.go | sed 's/.*"\(.*\)".*/\1/')}"
DIST="$PWD/dist"
rm -rf "$DIST"; mkdir -p "$DIST"

build() {
  local goos="$1" goarch="$2" name ext out
  name="vibe-check_${VERSION}_${goos}_${goarch}"
  ext="tar.gz"; [ "$goos" = windows ] && ext="zip"
  out="$DIST/$name.$ext"

  local bin="vibe-check"
  [ "$goos" = windows ] && bin="vibe-check.exe"

  local stage; stage="$(mktemp -d)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w -X github.com/gaurav-dhakal/vibe-check/cmd.version=$VERSION" \
      -o "$stage/$bin" .

  # Same binary answers to the short name; ship both so installs need no symlink.
  cp "$stage/$bin" "$stage/vcheck$([ "$goos" = windows ] && echo .exe)"
  cp LICENSE "$stage/LICENSE" 2>/dev/null || true
  [ -f README.md ] && cp README.md "$stage/README.md"

  if [ "$ext" = zip ]; then
    if command -v zip >/dev/null 2>&1; then
      ( cd "$stage" && zip -qr "$out" . )
    elif command -v python3 >/dev/null 2>&1; then
      ( cd "$stage" && python3 -m zipfile -c "$out" . )
    else
      echo "need zip or python3 to package windows assets" >&2; exit 1
    fi
  else
    tar -C "$stage" -czf "$out" $(cd "$stage" && ls)
  fi
  rm -rf "$stage"
  printf '  %-46s %s\n' "$out" "$(du -h "$out" | cut -f1)"
}

echo "building vibe-check $VERSION"
for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 \
         windows/amd64 windows/arm64 freebsd/amd64; do
  build "${t%/*}" "${t#*/}"
done

( cd "$DIST" && sha256sum vibe-check_* > checksums.txt )
echo "$(ls "$DIST" | wc -l) assets in $DIST/"
