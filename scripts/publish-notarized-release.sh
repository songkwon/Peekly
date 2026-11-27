#!/bin/bash
set -euo pipefail

if [[ $# != 3 || "$(uname -s)" != Darwin || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Usage on macOS: $0 vMAJOR.MINOR.PATCH RELAYDROID_TEAM_ID RELAYDROID_NOTARYTOOL_PROFILE" >&2
  exit 2
fi

tag="$1"
team="$2"
profile="$3"
repo=songkwon/Peekly
script_dir="$(cd "$(dirname "$0")" && pwd)"
team_file="$script_dir/../build/darwin/TeamIdentifier"
if [[ -f "$team_file" && "$team" != "$(cat "$team_file")" ]]; then
  echo 'Use the same Developer ID team as the signed Relaydroid release.' >&2
  exit 2
fi

test "$(gh release view "$tag" --repo "$repo" --json isDraft --jq .isDraft)" = true
stage="$(mktemp -d "${TMPDIR:-/tmp}/peekly-release.XXXXXX")"
trap 'rm -rf "$stage"' EXIT
assets="$stage/assets"
mkdir "$assets"

gh release download "$tag" --repo "$repo" --pattern 'Peekly-*.zip' --pattern SHA256SUMS.txt --dir "$assets"
(cd "$assets" && shasum -a 256 -c SHA256SUMS.txt)

for arch in arm64 amd64; do
  zip="Peekly-macOS-$arch.zip"
  "$script_dir/notarize-macos.sh" "$assets/$zip" "$stage/$zip" "$team" "$profile"
  mkdir "$stage/$arch"
  ditto -x -k "$stage/$zip" "$stage/$arch"
  app="$stage/$arch/Peekly.app"
  case "$arch" in
    arm64) expected=arm64 ;;
    amd64) expected=x86_64 ;;
  esac
  test "$(lipo -archs "$app/Contents/MacOS/peekly")" = "$expected"
  test "$(plutil -extract CFBundleShortVersionString raw "$app/Contents/Info.plist")" = "${tag#v}"
  mv "$stage/$zip" "$assets/$zip"
done

cd "$assets"
shasum -a 256 Peekly-macOS-*.zip Peekly-windows-*.zip > SHA256SUMS.txt
test "$(gh release view "$tag" --repo "$repo" --json isDraft --jq .isDraft)" = true
gh release upload "$tag" --repo "$repo" --clobber Peekly-macOS-*.zip SHA256SUMS.txt
gh release edit "$tag" --repo "$repo" --draft=false --latest
