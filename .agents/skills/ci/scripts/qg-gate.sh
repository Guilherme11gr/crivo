#!/usr/bin/env bash
# crivo-gate.sh — Smart quality gate runner for CI
# Detects context (PR vs push) and runs with appropriate flags.
#
# Usage: ./crivo-gate.sh [--strict|--lenient|--informational]
#
# Environment variables (auto-detected from common CI platforms):
#   CRIVO_MODE         - "pr" or "push" (auto-detected from CI env)
#   CRIVO_BASE_BRANCH  - branch to compare against (default: auto-detect)
#   CRIVO_OUTPUT_DIR   - where to write reports (default: ./crivo-reports)
#   CRIVO_EXTRA_FLAGS  - additional flags to pass to crivo
#
# Outputs:
#   crivo-reports/output.json    - Full JSON report
#   crivo-reports/report.md      - Markdown for PR comments
#   crivo-reports/report.sarif   - SARIF for code scanning
#   Exit code 0 (pass) or 1 (fail)
set -euo pipefail

MODE="${CRIVO_MODE:-}"
OUTPUT_DIR="${CRIVO_OUTPUT_DIR:-./crivo-reports}"
EXTRA_FLAGS="${CRIVO_EXTRA_FLAGS:-}"
GATE_POLICY="${1:-}"
CRIVO_BIN="${CRIVO_BIN:-crivo}"

mkdir -p "$OUTPUT_DIR"

# ---------------------------------------------------------------------------
# Cross-platform path handling (WSL, Git Bash, native)
# ---------------------------------------------------------------------------
to_native_path() {
  local p="$1"
  # WSL: convert /mnt/c/... to C:/...
  if [ -f /proc/version ] && grep -qi microsoft /proc/version 2>/dev/null; then
    p=$(wslpath -w "$p" 2>/dev/null || echo "$p")
  fi
  echo "$p"
}

# If output dir is relative, make it absolute for the binary
if [[ "$OUTPUT_DIR" != /* ]]; then
  OUTPUT_DIR="$(pwd)/$OUTPUT_DIR"
fi

# ---------------------------------------------------------------------------
# Auto-detect CI context
# ---------------------------------------------------------------------------
detect_mode() {
  # GitHub Actions
  if [ -n "${GITHUB_EVENT_NAME:-}" ]; then
    if [ "$GITHUB_EVENT_NAME" = "pull_request" ]; then
      echo "pr"
    else
      echo "push"
    fi
    return
  fi

  # GitLab CI
  if [ -n "${CI_MERGE_REQUEST_IID:-}" ]; then
    echo "pr"
    return
  fi
  if [ -n "${CI_COMMIT_BRANCH:-}" ]; then
    echo "push"
    return
  fi

  # CircleCI
  if [ -n "${CIRCLE_PULL_REQUEST:-}" ]; then
    echo "pr"
    return
  fi
  if [ -n "${CIRCLECI:-}" ]; then
    echo "push"
    return
  fi

  # Azure DevOps
  if [ -n "${SYSTEM_PULLREQUEST_PULLREQUESTID:-}" ]; then
    echo "pr"
    return
  fi
  if [ -n "${BUILD_SOURCEBRANCH:-}" ]; then
    echo "push"
    return
  fi

  # Bitbucket Pipelines
  if [ -n "${BITBUCKET_PR_ID:-}" ]; then
    echo "pr"
    return
  fi

  # Fallback: if on a branch that isn't main/master, treat as PR
  local branch
  branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
  if [ "$branch" != "main" ] && [ "$branch" != "master" ]; then
    echo "pr"
  else
    echo "push"
  fi
}

if [ -z "$MODE" ]; then
  MODE=$(detect_mode)
fi

echo "=== Quality Gate ==="
echo "  Mode:   $MODE"
echo "  Output: $OUTPUT_DIR"

# ---------------------------------------------------------------------------
# Build crivo flags
# ---------------------------------------------------------------------------
MD_PATH=$(to_native_path "$OUTPUT_DIR/report.md")
SARIF_PATH=$(to_native_path "$OUTPUT_DIR/report.sarif")

CRIVO_FLAGS="--save"
CRIVO_FLAGS="$CRIVO_FLAGS --md $MD_PATH"
CRIVO_FLAGS="$CRIVO_FLAGS --sarif $SARIF_PATH"

if [ "$MODE" = "pr" ]; then
  CRIVO_FLAGS="$CRIVO_FLAGS --new-code"
  if [ -n "${CRIVO_BASE_BRANCH:-}" ]; then
    CRIVO_FLAGS="$CRIVO_FLAGS --branch $CRIVO_BASE_BRANCH"
  fi
fi

if [ -n "$EXTRA_FLAGS" ]; then
  CRIVO_FLAGS="$CRIVO_FLAGS $EXTRA_FLAGS"
fi

# ---------------------------------------------------------------------------
# Run quality gate
# ---------------------------------------------------------------------------
echo "  Running: $CRIVO_BIN run $CRIVO_FLAGS --json"
echo ""

EXIT_CODE=0
# Run: JSON goes to file, stderr stays on console (never mix them)
$CRIVO_BIN run $CRIVO_FLAGS --json > "$OUTPUT_DIR/output.json" 2>"$OUTPUT_DIR/stderr.log" || EXIT_CODE=$?

# Show stderr to console for debugging
if [ -s "$OUTPUT_DIR/stderr.log" ]; then
  cat "$OUTPUT_DIR/stderr.log" >&2
fi

# ---------------------------------------------------------------------------
# Parse results
# ---------------------------------------------------------------------------
if command -v jq &>/dev/null && [ -f "$OUTPUT_DIR/output.json" ]; then
  STATUS=$(jq -r '.status // "unknown"' "$OUTPUT_DIR/output.json")
  TOTAL=$(jq -r '.totalIssues // 0' "$OUTPUT_DIR/output.json")
  DURATION=$(jq -r '.totalDuration // "?"' "$OUTPUT_DIR/output.json")

  echo "=== Results ==="
  echo "  Status:     $STATUS"
  echo "  Issues:     $TOTAL"
  echo "  Duration:   $DURATION"
  echo ""

  # Per-check summary
  echo "  Checks:"
  jq -r '.checks[] | "    \(.status | if . == "passed" then "✅" elif . == "failed" then "❌" elif . == "warning" then "⚠️" else "⏭️" end)  \(.name): \(.summary)"' "$OUTPUT_DIR/output.json" 2>/dev/null || true
  echo ""

  # Ratings
  echo "  Ratings:"
  jq -r '.ratings | to_entries[] | "    \(.key): \(.value)"' "$OUTPUT_DIR/output.json" 2>/dev/null || true
  echo ""
fi

# ---------------------------------------------------------------------------
# Gate decision
# ---------------------------------------------------------------------------
case "$GATE_POLICY" in
  --informational)
    echo "  Policy: informational (never blocks)"
    exit 0
    ;;
  --lenient)
    # Only block on type errors and secrets
    if command -v jq &>/dev/null && [ -f "$OUTPUT_DIR/output.json" ]; then
      BLOCKERS=$(jq '[.conditions[] | select(.passed == false and (.metric == "type_errors" or .metric == "secrets"))] | length' "$OUTPUT_DIR/output.json" 2>/dev/null || echo "0")
      if [ "$BLOCKERS" -gt 0 ]; then
        echo "  Policy: lenient — BLOCKED by $BLOCKERS critical condition(s)"
        exit 1
      fi
      echo "  Policy: lenient — passed (non-critical issues tolerated)"
      exit 0
    fi
    ;;
  --strict)
    echo "  Policy: strict — using crivo exit code directly"
    exit $EXIT_CODE
    ;;
  *)
    # Default: block on type errors, lint errors, secrets. Warn on coverage/complexity.
    if command -v jq &>/dev/null && [ -f "$OUTPUT_DIR/output.json" ]; then
      BLOCKERS=$(jq '[.conditions[] | select(.passed == false and (.metric == "type_errors" or .metric == "lint_errors" or .metric == "secrets"))] | length' "$OUTPUT_DIR/output.json" 2>/dev/null || echo "0")
      if [ "$BLOCKERS" -gt 0 ]; then
        echo "  Policy: default — BLOCKED by $BLOCKERS condition(s)"
        exit 1
      fi
      echo "  Policy: default — passed (coverage/complexity warnings tolerated)"
      exit 0
    fi
    # Fallback: use crivo exit code
    exit $EXIT_CODE
    ;;
esac

exit $EXIT_CODE
