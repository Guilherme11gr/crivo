#!/usr/bin/env bash
# crivo-parse-json.sh — Extract metrics from quality gate JSON output
# Useful for custom dashboards, Slack notifications, or agent pipelines.
#
# Usage: ./crivo-parse-json.sh <command> [json-file]
#
# Commands:
#   summary      - One-line summary (for Slack/notifications)
#   status       - Just "passed" or "failed"
#   metrics      - Key=value pairs (for scripting)
#   issues       - Top N issues as TSV (file:line, severity, message)
#   conditions   - Gate conditions as TSV (metric, passed, actual, threshold)
#   ratings      - Ratings as key=value
#   for-agent    - Compact format optimized for AI agent consumption
#
# Examples:
#   ./crivo-parse-json.sh summary crivo-reports/output.json
#   ./crivo-parse-json.sh metrics | grep coverage
#   ./crivo-parse-json.sh issues | head -10
#   ./crivo-parse-json.sh for-agent | claude "Fix these issues"
set -euo pipefail

COMMAND="${1:-summary}"
JSON="${2:-crivo-reports/output.json}"

if [ ! -f "$JSON" ]; then
  echo "Error: $JSON not found. Run 'crivo run --json > $JSON' first." >&2
  exit 1
fi

if ! command -v jq &>/dev/null; then
  echo "Error: jq is required. Install it with: apt-get install jq / brew install jq" >&2
  exit 1
fi

case "$COMMAND" in
  summary)
    STATUS=$(jq -r '.status' "$JSON")
    TOTAL=$(jq -r '.totalIssues' "$JSON")
    DURATION=$(jq -r '.duration // "?"' "$JSON")
    CHECKS_PASSED=$(jq '[.checks[] | select(.status == "passed")] | length' "$JSON")
    CHECKS_TOTAL=$(jq '.checks | length' "$JSON")
    RATINGS=$(jq -r '[.ratings | to_entries[] | "\(.key[0:3]):\(.value)"] | join(" ")' "$JSON")

    if [ "$STATUS" = "passed" ]; then
      echo "✅ Quality Gate PASSED — $TOTAL issues, $CHECKS_PASSED/$CHECKS_TOTAL checks passed [$RATINGS] ($DURATION)"
    else
      FAILED=$(jq -r '[.conditions[] | select(.passed == false) | .metric] | join(", ")' "$JSON")
      echo "❌ Quality Gate FAILED — $TOTAL issues, blocked by: $FAILED [$RATINGS] ($DURATION)"
    fi
    ;;

  status)
    jq -r '.status' "$JSON"
    ;;

  metrics)
    # Output all metrics as key=value for easy grepping/sourcing
    jq -r '.checks[] | .id as $id | (.metrics // {}) | to_entries[] | "\($id)_\(.key)=\(.value)"' "$JSON" 2>/dev/null || echo "(no metrics)"
    ;;

  issues)
    # Output issues as TSV: file:line  severity  type  source  message
    jq -r '[.checks[] | (.issues // [])[] ] | sort_by(.severity) | .[] | ["\(.file):\(.line)", .severity, .type, .source, .message] | @tsv' "$JSON" 2>/dev/null || echo "(no issues)"
    ;;

  conditions)
    # Output gate conditions as TSV: metric  passed  actual  threshold
    jq -r '.conditions[] | [.metric, .passed, .actual, .threshold] | @tsv' "$JSON"
    ;;

  ratings)
    jq -r '.ratings | to_entries[] | "\(.key)=\(.value)"' "$JSON"
    ;;

  for-agent)
    # Compact format designed for AI agent consumption
    # Includes only actionable information, sorted by severity
    cat <<HEADER
# Quality Gate Report
Status: $(jq -r '.status' "$JSON")
Total Issues: $(jq -r '.totalIssues' "$JSON")

## Failed Conditions
HEADER
    jq -r '.conditions[] | select(.passed == false) | "- \(.metric): \(.actual) (threshold: \(.threshold))"' "$JSON" 2>/dev/null || echo "(none)"

    echo ""
    echo "## Issues by Severity (top 20)"
    echo ""

    # Critical/blocker first, then major, then minor
    # Handle null issues arrays (Go omitempty serializes empty slices as null)
    jq -r '
      [.checks[] | (.issues // [])[]] |
      sort_by(
        if .severity == "blocker" then 0
        elif .severity == "critical" then 1
        elif .severity == "major" then 2
        elif .severity == "minor" then 3
        else 4 end
      ) |
      if length == 0 then "(no issues)"
      else .[:20][] | "[\(.severity)] \(.file):\(.line) — \(.message) (\(.source)/\(.ruleId))"
      end
    ' "$JSON" 2>/dev/null || echo "(no issues)"

    echo ""
    echo "## Ratings"
    jq -r '.ratings | to_entries[] | "- \(.key): \(.value)"' "$JSON"
    ;;

  *)
    echo "Unknown command: $COMMAND" >&2
    echo "Available: summary, status, metrics, issues, conditions, ratings, for-agent" >&2
    exit 1
    ;;
esac
