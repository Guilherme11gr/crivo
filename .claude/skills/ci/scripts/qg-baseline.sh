#!/usr/bin/env bash
# crivo-baseline.sh — Update quality gate baseline on main branch
# Run this after merging to main to capture the new baseline.
# Future PR checks will compare against this baseline.
#
# Usage: ./crivo-baseline.sh
#
# What it does:
#   1. Runs full quality gate (no --new-code)
#   2. Saves results to .qualitygate/history.db (--save)
#   3. Optionally commits the updated baseline to the repo
#
# Environment variables:
#   CRIVO_COMMIT_BASELINE  - "true" to auto-commit the db (default: false)
#   CRIVO_BRANCH           - branch name for tagging (default: auto-detect)
set -euo pipefail

COMMIT_BASELINE="${CRIVO_COMMIT_BASELINE:-false}"
BRANCH="${CRIVO_BRANCH:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'main')}"

echo "=== Quality Gate Baseline Update ==="
echo "  Branch: $BRANCH"
echo ""

# Run full analysis and save
crivo run --save --verbose 2>&1 | tail -30

echo ""
echo "Baseline updated in .qualitygate/history.db"

# Show what was captured
if command -v crivo &>/dev/null; then
  echo ""
  echo "=== Trend ==="
  crivo trends 2>/dev/null || echo "  (first data point — trends available after 2+ runs)"
fi

# Optionally commit the baseline
if [ "$COMMIT_BASELINE" = "true" ]; then
  if [ -f .qualitygate/history.db ]; then
    git add .qualitygate/history.db
    git commit -m "chore: update quality gate baseline

Branch: $BRANCH
Commit: $(git rev-parse --short HEAD)" 2>/dev/null || echo "  (no changes to commit)"
    echo "Baseline committed."
  fi
fi
