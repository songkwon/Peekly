#!/bin/bash
set -euo pipefail

if [[ $# != 1 || "$(uname -s)" != Darwin || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Usage on macOS: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

tag="$1"
repo=songkwon/Peekly
script_dir="$(cd "$(dirname "$0")" && pwd)"
cask=Casks/peekly.rb
branch="release/homebrew-$tag"
base="$(gh repo view "$repo" --json defaultBranchRef --jq .defaultBranchRef.name)"
existing="$(gh pr list --repo "$repo" --base "$base" --head "$branch" --state open --json url --jq '.[0].url // empty')"
if [[ -n "$existing" ]]; then
  printf '%s\n' "$existing"
  exit 0
fi

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
"$script_dir/verify-release-macos.sh" "$tag" "$stage/checksums.txt"
git clone --quiet --depth 1 --branch "$base" "https://github.com/$repo.git" "$stage/repo"
cd "$stage/repo"
python3 "$script_dir/update-cask.py" "$tag" "$stage/checksums.txt"
ruby -c "$cask"
git add "$cask"
if git diff --cached --quiet -- "$cask"; then
  echo 'The default branch already has this cask version or a newer version.'
  exit 0
fi

account="$(gh api user --jq '[.id, .login] | join("+")')"
git -c user.name="${account#*+}" -c user.email="$account@users.noreply.github.com" \
  commit -m "Update Homebrew cask to $tag"
ref="refs/heads/$branch"
if [[ -n "$(git ls-remote --heads origin "$ref")" ]]; then
  git fetch --quiet --depth=2 origin "$ref"
  test "$(git diff --name-only FETCH_HEAD^ FETCH_HEAD)" = "$cask"
  git show "FETCH_HEAD:$cask" | cmp - "$cask"
else
  git -c credential.helper= -c 'credential.helper=!gh auth git-credential' push origin "HEAD:$ref"
fi

gh pr create --repo "$repo" --base "$base" --head "$branch" \
  --title "Update Homebrew cask to $tag" \
  --body "Publish the Homebrew cask with SHA-256 values from the signed and notarized macOS $tag release."
