#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."
stage="$(mktemp -d "${TMPDIR:-/tmp}/peekly-icon.XXXXXX")"
trap 'rm -rf "$stage"' EXIT

xcrun actool build/darwin/AppIcon.icon \
  --compile "$stage" --platform macosx --target-device mac \
  --minimum-deployment-target 12.0 --app-icon AppIcon \
  --output-partial-info-plist "$stage/icon.plist" --output-format human-readable-text
test -s "$stage/AppIcon.icns"
test -s "$stage/Assets.car"
cp "$stage/AppIcon.icns" "$stage/Assets.car" build/darwin/
