#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/release.sh <version>

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

if [[ $# -ne 1 ]]; then
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

input_version="$1"
semver_re='^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if [[ ! "$input_version" =~ $semver_re ]]; then
  usage
  die "invalid release version: $input_version"
fi

version="${input_version#v}"
tag="v${version}"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  die "tag already exists: $tag"
fi

current_version=""
if [[ -f VERSION ]]; then
  current_version="$(tr -d '[:space:]' < VERSION)"
fi
current_package_version="$(node -p "require('./frontend/package.json').version")"
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

Then manually publish a GitHub Release for ${tag}. The backend and frontend
Docker images will be built and pushed only after that GitHub Release is published.
EOF
