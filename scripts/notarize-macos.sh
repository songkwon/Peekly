#!/bin/bash
set -euo pipefail

if [[ $# != 4 || "$(uname -s)" != Darwin ]]; then
  echo "Usage on macOS: $0 UNSIGNED_ZIP SIGNED_ZIP TEAM_ID NOTARYTOOL_PROFILE" >&2
  exit 2
fi

input="$1"
output="$2"
team="$3"
profile="$4"
[[ "$team" =~ ^[A-Z0-9]{10}$ ]] || { echo 'Invalid Apple Team ID.' >&2; exit 2; }
script_dir="$(cd "$(dirname "$0")" && pwd)"
test "$team" = "$(cat "$script_dir/../build/darwin/TeamIdentifier")" || {
  echo 'Use the Developer ID team pinned to the published Relaydroid app.' >&2
  exit 2
}
identity="$(security find-identity -v -p codesigning |
  awk -v team="$team" 'index($0, "Developer ID Application:") && index($0, "(" team ")\042") { print $2; exit }')"
[[ "$identity" =~ ^[0-9A-Fa-f]{40}$ ]] || { echo "No usable Developer ID Application identity for team $team." >&2; exit 1; }

stage="$(mktemp -d "${TMPDIR:-/tmp}/peekly-notary.XXXXXX")"
trap 'rm -rf "$stage"' EXIT

unzip -tq "$input"
ditto -x -k "$input" "$stage"
app="$stage/Peekly.app"
test "$(plutil -extract CFBundleIdentifier raw "$app/Contents/Info.plist")" = app.peekly.desktop

codesign --force --options runtime --timestamp --sign "$identity" "$app"
codesign --verify --deep --strict "$app"
codesign -dv --verbose=4 "$app" 2>&1 | grep -Fx "TeamIdentifier=$team"

submission="$stage/submission.zip"
ditto -c -k --sequesterRsrc --keepParent "$app" "$submission"
xcrun notarytool submit "$submission" --keychain-profile "$profile" --wait --output-format json > "$stage/result.json"
status="$(plutil -extract status raw "$stage/result.json")"
if [[ "$status" != Accepted ]]; then
  id="$(plutil -extract id raw "$stage/result.json")"
  printf 'Notarization status %s for submission %s\n' "$status" "$id" >&2
  exit 1
fi
xcrun stapler staple "$app"
xcrun stapler validate "$app"
codesign --verify --deep --strict "$app"

zip="$stage/signed.zip"
ditto -c -k --sequesterRsrc --keepParent "$app" "$zip"
unzip -tq "$zip"
mv "$zip" "$output"
printf 'Notarized package: %s\n' "$output"
