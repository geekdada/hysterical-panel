#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/release.sh [version]

Without a version, the script shows the current version and asks for the next
one: a patch, minor or major bump, or a version typed by hand.

Accepted versions:
  1.2.3
  v1.2.3
  1.2.3-rc.1
  v1.2.3-rc.1

Build metadata such as 1.2.3+build.1 is not accepted because Docker tags do not support +.
USAGE
}

die() {
  echo "error: $*" >&2
  exit 1
}

run() {
  echo "==> $*"
  "$@"
}

semver_re='^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

# A bump from a prerelease finalizes it when the bumped part is already set,
# so patch on 1.3.0-rc.1 gives 1.3.0 instead of skipping to 1.3.1.
bump_version() {
  local kind="$1" current="$2"
  [[ "$current" =~ $semver_re ]] || die "cannot bump invalid version: $current"
  local major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}" patch="${BASH_REMATCH[3]}"
  local prerelease="${BASH_REMATCH[4]}"

  case "$kind" in
    major)
      if [[ -z "$prerelease" || "$minor" != 0 || "$patch" != 0 ]]; then
        major=$((major + 1)) minor=0 patch=0
      fi
      ;;
    minor)
      if [[ -z "$prerelease" || "$patch" != 0 ]]; then
        minor=$((minor + 1)) patch=0
      fi
      ;;
    patch)
      [[ -n "$prerelease" ]] || patch=$((patch + 1))
      ;;
  esac
  printf '%s.%s.%s\n' "$major" "$minor" "$patch"
}

prompt_version() {
  if [[ ! -t 0 ]]; then
    usage
    die "no version given and stdin is not a terminal"
  fi

  local base="${current_version:-0.0.0}" latest_tag choice custom
  latest_tag="$(git tag --list 'v*' --sort=-v:refname | sed -n 1p)"
  local patch_version minor_version major_version
  patch_version="$(bump_version patch "$base")"
  minor_version="$(bump_version minor "$base")"
  major_version="$(bump_version major "$base")"

  {
    echo "Current version: ${current_version:-none} (VERSION)"
    echo "Latest tag:      ${latest_tag:-none}"
    echo
    echo "  1) patch   ${patch_version}"
    echo "  2) minor   ${minor_version}"
    echo "  3) major   ${major_version}"
    echo "  4) custom"
    echo
  } >&2

  read -r -p "Next version [1]: " choice
  case "${choice:-1}" in
    1 | patch) echo "$patch_version" ;;
    2 | minor) echo "$minor_version" ;;
    3 | major) echo "$major_version" ;;
    4 | custom)
      read -r -p "Version: " custom
      echo "$custom"
      ;;
    *) die "invalid choice: $choice" ;;
  esac
}

if [[ $# -gt 1 ]]; then
  usage
  exit 1
fi

for cmd in git go node pnpm; do
  command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
done

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

if [[ -n "$(git status --porcelain)" ]]; then
  die "git worktree must be clean before preparing a release"
fi

current_version=""
if [[ -f VERSION ]]; then
  current_version="$(tr -d '[:space:]' < VERSION)"
fi
current_package_version="$(node -p "require('./frontend/package.json').version")"

if [[ $# -eq 1 ]]; then
  input_version="$1"
else
  input_version="$(prompt_version)"
fi

if [[ ! "$input_version" =~ $semver_re ]]; then
  usage
  die "invalid release version: $input_version"
fi

version="${input_version#v}"
tag="v${version}"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  die "tag already exists: $tag"
fi

if [[ "$current_version" == "$version" && "$current_package_version" == "$version" ]]; then
  die "version files already contain $version"
fi

printf '%s\n' "$version" > VERSION
node - "$version" <<'NODE'
const fs = require("fs");

const version = process.argv[2];
const path = "frontend/package.json";
const pkg = JSON.parse(fs.readFileSync(path, "utf8"));

pkg.version = version;
fs.writeFileSync(path, `${JSON.stringify(pkg, null, 2)}\n`);
NODE

run bash -c 'cd backend && go test ./...'
run bash -c 'cd backend && go vet ./...'
run bash -c 'cd backend && go build ./...'
run bash -c 'cd frontend && pnpm install --frozen-lockfile'
run bash -c 'cd frontend && pnpm lint'
run bash -c 'cd frontend && pnpm format:check'
run bash -c 'cd frontend && pnpm typecheck'
run bash -c 'cd frontend && pnpm build'

unexpected_changes="$(
  git diff --name-only
  git diff --cached --name-only
)"
unexpected_changes="$(
  printf '%s\n' "$unexpected_changes" \
    | sed '/^$/d' \
    | grep -Ev '^(VERSION|frontend/package\.json)$' || true
)"
if [[ -n "$unexpected_changes" ]]; then
  echo "$unexpected_changes" >&2
  die "release checks changed tracked files outside VERSION and frontend/package.json"
fi

run git add VERSION frontend/package.json
run git commit -m "chore(release): ${tag}"
run git tag -a "$tag" -m "Release ${tag}"

cat <<EOF

Release ${tag} is prepared locally.

Next manual steps:
  git push origin master
  git push origin ${tag}

Pushing the tag builds and pushes the backend and frontend Docker images, then
creates a draft GitHub Release for ${tag} with the image versions in its body.
Review and publish that draft from the Releases page when you are ready.
EOF
