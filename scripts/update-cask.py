import argparse
import re
from pathlib import Path


parser = argparse.ArgumentParser(description="Update Peekly's Homebrew cask from verified release checksums.")
parser.add_argument("tag")
parser.add_argument("checksums", type=Path)
args = parser.parse_args()

version_match = re.fullmatch(r"v([0-9]+\.[0-9]+\.[0-9]+)", args.tag)
if not version_match:
    parser.error("the tag must be a stable vMAJOR.MINOR.PATCH version")
version = version_match[1]

checksums = {}
for line in args.checksums.read_text(encoding="utf-8").splitlines():
    match = re.fullmatch(r"([a-f0-9]{64})  (Peekly-macOS-(?:arm64|amd64)\.zip)", line)
    if not match or match[2] in checksums:
        parser.error("expected unique SHA-256 values for the two macOS release archives")
    checksums[match[2]] = match[1]
if len(checksums) != 2:
    parser.error("both macOS release archives must have SHA-256 values")

cask = Path("Casks/peekly.rb")
if cask.exists():
    current = re.search(r'^  version "([0-9]+\.[0-9]+\.[0-9]+)"$', cask.read_text(encoding="utf-8"), re.M)
    if not current:
        parser.error("could not read the existing cask version")
    if tuple(map(int, current[1].split("."))) > tuple(map(int, version.split("."))):
        raise SystemExit(0)

cask.parent.mkdir(parents=True, exist_ok=True)
cask.write_text(
    f'''cask "peekly" do
  arch arm: "arm64", intel: "amd64"

  version "{version}"
  sha256 arm:   "{checksums['Peekly-macOS-arm64.zip']}",
         intel: "{checksums['Peekly-macOS-amd64.zip']}"

  url "https://github.com/songkwon/Peekly/releases/download/v#{{version}}/Peekly-macOS-#{{arch}}.zip"
  name "Peekly"
  desc "Capture and debug HTTP and HTTPS traffic"
  homepage "https://github.com/songkwon/Peekly"

  depends_on macos: :monterey

  app "Peekly.app"
end
''',
    encoding="utf-8",
)
