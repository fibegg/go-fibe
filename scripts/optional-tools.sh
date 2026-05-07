#!/usr/bin/env bash
set -euo pipefail

if command -v hadolint >/dev/null 2>&1; then
  hadolint --failure-threshold error Dockerfile Dockerfile.dev
fi

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck scripts/*.sh
fi

if command -v actionlint >/dev/null 2>&1; then
  actionlint .github/workflows/*.yml
fi

if command -v gitleaks >/dev/null 2>&1; then
  gitleaks detect --no-git --redact --source .
fi
