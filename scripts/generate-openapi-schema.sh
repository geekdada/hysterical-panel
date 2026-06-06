#!/usr/bin/env bash
set -euo pipefail

die() {
  echo "error: $*" >&2
  exit 1
}

for cmd in go mktemp; do
  command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
backend_dir="$repo_root/backend"
output_path="${1:-openapi.json}"

[[ -d "$backend_dir" ]] || die "backend directory not found: $backend_dir"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

cd "$backend_dir"
PANEL_MASTER_KEY="${PANEL_MASTER_KEY:-skip}" \
  PB_DATA_DIR="$tmp_dir" \
  go run . openapi-schema -o "$output_path"
