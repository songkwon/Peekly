#!/bin/bash
set -euo pipefail

if [[ "$(uname -s)" != Darwin ]]; then
  echo 'The macOS package must be built on macOS.' >&2
  exit 2
fi

cd "$(dirname "$0")/.."
arch="${1:-$(go env GOARCH)}"
case "$arch" in
  arm64) expected=arm64 ;;
  amd64) expected=x86_64 ;;
  universal) expected='x86_64 arm64' ;;
  *) echo 'Use arm64, amd64, or universal.' >&2; exit 2 ;;
esac

out="$PWD/build-out"
stage="$(mktemp -d "${TMPDIR:-/tmp}/peekly-package.XXXXXX")"
trap 'rm -rf "$stage"' EXIT
app="$stage/Peekly.app"
mkdir -p "$out" "$app/Contents/MacOS" "$app/Contents/Resources"

if [[ "${PEEKLY_SKIP_FRONTEND:-0}" != 1 ]]; then
  npm --prefix frontend ci
  npm --prefix frontend run build
fi

version="$(plutil -extract CFBundleShortVersionString raw build/darwin/Info.plist)"
frontend_version="$(node -p "require('./frontend/package.json').version")"
test "$version" = "$frontend_version"
test "$version" = "${PEEKLY_RELEASE_VERSION:-$version}"

build_binary() {
  GOOS=darwin GOARCH="$1" CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=12.0 \
    go build -mod=readonly -tags production -trimpath -buildvcs=false \
      -ldflags='-s -w -extldflags=-mmacosx-version-min=12.0' -o "$2" .
}

binary="$app/Contents/MacOS/peekly"
if [[ "$arch" = universal ]]; then
  build_binary amd64 "$stage/peekly-amd64"
  build_binary arm64 "$stage/peekly-arm64"
  lipo -create "$stage/peekly-amd64" "$stage/peekly-arm64" -output "$binary"
else
  build_binary "$arch" "$binary"
fi
test "$(lipo -archs "$binary")" = "$expected"
cp build/darwin/Info.plist "$app/Contents/"
cp build/darwin/icons.icns "$app/Contents/Resources/"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"

zip="$stage/Peekly-macOS-$arch.zip"
ditto -c -k --sequesterRsrc --keepParent "$app" "$zip"
unzip -tq "$zip"
mv "$zip" "$out/"
printf '%s\n' "$out/Peekly-macOS-$arch.zip"
