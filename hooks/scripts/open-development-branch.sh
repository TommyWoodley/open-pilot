#!/usr/bin/env bash
set -euo pipefail

normalize_branch_name() {
  local raw="$1"
  local out
  out="$(printf '%s' "$raw" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's|[[:space:]/]+|-|g; s|[^a-z0-9._-]+|-|g; s|-+|-|g; s|^-+||; s|-+$||')"
  if [ -z "$out" ]; then
    out="session"
  fi
  if ! git check-ref-format --branch "$out" >/dev/null 2>&1; then
    out="session"
  fi
  printf '%s' "$out"
}

repo="${OPEN_PILOT_REPO_PATH:-}"
[ -n "$repo" ] || exit 0
cd "$repo" || exit 1
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0

remote="$(git remote | head -n 1)"
if [ -z "$remote" ]; then
  if git show-ref --verify --quiet refs/heads/main; then
    base="main"
  elif git show-ref --verify --quiet refs/heads/master; then
    base="master"
  else
    base="HEAD"
  fi
elif git show-ref --verify --quiet refs/heads/main || git ls-remote --exit-code --heads "$remote" main >/dev/null 2>&1; then
  base="main"
elif git show-ref --verify --quiet refs/heads/master || git ls-remote --exit-code --heads "$remote" master >/dev/null 2>&1; then
  base="master"
else
  exit 0
fi

if [ -n "$remote" ]; then
  git fetch "$remote" "$base" >/dev/null 2>&1 || exit 1
  git show-ref --verify --quiet "refs/remotes/$remote/$base" || exit 0

  current="$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  if [ "$current" = "$base" ]; then
    git merge --ff-only "$remote/$base" >/dev/null 2>&1 || exit 1
  elif git show-ref --verify --quiet "refs/heads/$base"; then
    git branch -f "$base" "$remote/$base" >/dev/null 2>&1 || exit 1
  else
    git branch --track "$base" "$remote/$base" >/dev/null 2>&1 || exit 1
  fi
fi

session_name="${OPEN_PILOT_SESSION_NAME:-${OPEN_PILOT_SESSION_ID:-session}}"
target="$(normalize_branch_name "$session_name")"

if git show-ref --verify --quiet "refs/heads/$target"; then
  git checkout "$target" >/dev/null 2>&1 || exit 1
  upstream="$(git for-each-ref --format='%(upstream:short)' "refs/heads/$target")"
  if [ -n "$upstream" ]; then
    upstream_remote="${upstream%%/*}"
    upstream_branch="${upstream#*/}"
    if [ "$upstream_remote" != "$upstream" ] && [ -n "$upstream_branch" ]; then
      git fetch "$upstream_remote" "$upstream_branch" >/dev/null 2>&1 || exit 1
      git merge --ff-only "$upstream" >/dev/null 2>&1 || exit 1
    fi
  fi
  exit 0
fi

if [ -n "$remote" ] && git ls-remote --exit-code --heads "$remote" "$target" >/dev/null 2>&1; then
  git checkout -b "$target" --track "$remote/$target" >/dev/null 2>&1 || exit 1
else
  git checkout -b "$target" "$base" >/dev/null 2>&1 || exit 1
fi
