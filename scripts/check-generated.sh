#!/usr/bin/env bash
set -euo pipefail

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git diff --exit-code -- graph/generated graph/model graph/schema.resolvers.go graph/helpers.go
else
  echo "Skipping generated diff check outside a git worktree."
fi

