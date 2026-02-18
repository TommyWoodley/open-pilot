#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REF="${1:-main}"
OWNER_REPO="TommyWoodley/pilot-superpowers"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

archive="$tmp_dir/superpowers.tar.gz"
extract_dir="$tmp_dir/extract"
dest_dir="$REPO_ROOT/skills/superpowers"

curl -fsSL "https://github.com/${OWNER_REPO}/archive/refs/heads/${REF}.tar.gz" -o "$archive"
mkdir -p "$extract_dir"
tar -xzf "$archive" -C "$extract_dir"

src_root="$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
if [[ -z "${src_root:-}" || ! -d "$src_root" ]]; then
  echo "Unable to detect extracted repository root in: $extract_dir" >&2
  exit 1
fi
src_skills_dir="$src_root/skills"
if [[ ! -d "$src_skills_dir" ]]; then
  echo "Expected skills directory not found in downloaded archive: $src_skills_dir" >&2
  exit 1
fi

rm -rf "$dest_dir"
mkdir -p "$dest_dir"
cp -a "$src_skills_dir/." "$dest_dir/"

echo "Synced ${OWNER_REPO}@${REF} skills -> $dest_dir"
