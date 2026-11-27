#!/bin/bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 || "$(uname -s)" != Darwin || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Usage on macOS: $0 vMAJOR.MINOR.PATCH [CHECKSUMS_OUTPUT]" >&2
  exit 2
fi

tag="$1"
checksums_output="${2:-}"
if [[ -n "$checksums_output" && "$checksums_output" != /* ]]; then
  checksums_output="$PWD/$checksums_output"
fi
repo=songkwon/Peekly
script_dir="$(cd "$(dirname "$0")" && pwd)"
stage="$(mktemp -d "${TMPDIR:-/tmp}/peekly-verify.XXXXXX")"
trap 'rm -rf "$stage"' EXIT

test "$(gh release view "$tag" --repo "$repo" --json isDraft,isPrerelease --jq '.isDraft or .isPrerelease')" = false
gh release download "$tag" --repo "$repo" --pattern 'Peekly-*.zip' --pattern SHA256SUMS.txt --dir "$stage"
cd "$stage"
shasum -a 256 -c SHA256SUMS.txt

team_file="$script_dir/../build/darwin/TeamIdentifier"
expected_team="$(cat "$team_file")"
for arch in arm64 amd64; do
  ditto -x -k "Peekly-macOS-$arch.zip" "$arch"
  app="$arch/Peekly.app"
  case "$arch" in
    arm64) expected=arm64 ;;
    amd64) expected=x86_64 ;;
  esac
  test "$(lipo -archs "$app/Contents/MacOS/peekly")" = "$expected"
  test "$(plutil -extract CFBundleIdentifier raw "$app/Contents/Info.plist")" = app.peekly.desktop
  test "$(plutil -extract CFBundleShortVersionString raw "$app/Contents/Info.plist")" = "${tag#v}"
  codesign --verify --deep --strict "$app"
  team="$(codesign -dv --verbose=4 "$app" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
  [[ "$team" =~ ^[A-Z0-9]{10}$ ]]
  test "$team" = "$expected_team"
  xcrun stapler validate "$app"
done

if [[ -n "$checksums_output" ]]; then
  shasum -a 256 Peekly-macOS-arm64.zip Peekly-macOS-amd64.zip > "$checksums_output"
fi
