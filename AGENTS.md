# Crivo

Local quality gate for AI-agent coding workflows and CI. Single Go binary that orchestrates existing OSS tools, normalizes output, calculates ratings, and presents results via terminal, JSON, SARIF, or Markdown.

## Commands

```bash
go build -o crivo.exe ./cmd/crivo/  # Build
go test ./internal/...               # Run tests
go vet ./...                         # Lint

crivo run                            # Run the full gate (default)
crivo run --json                     # JSON output for agents/automation
crivo run --verbose                  # Full details
crivo run --save                     # Save to local history
crivo run --new-code                 # Only analyze changed code
crivo run --sarif report.sarif       # SARIF 2.1.0 output
crivo run --md report.md             # Markdown for PR comments
crivo init                           # Setup in project
crivo trends                         # Show sparkline history
crivo version                        # Show version
```

## Architecture

- **Provider pattern**: Each check implements `check.Provider` interface (Name, ID, Detect, Analyze)
- **Parallel runner**: Goroutines with semaphore, progress channel for real-time updates
- **Orchestrator philosophy**: Runs existing tools (tsc, jest/vitest, jscpd, semgrep, gitleaks, knip), parses their output
- **Zero CGO**: Uses `modernc.org/sqlite` for pure-Go SQLite

## Product Intent

- Final verification step for code written by AI agents
- Works both locally at the end of a coding session and in CI
- Combines baseline checks with project-specific custom rules
- Optimized for structured output that agents can consume and act on

## Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/crivo/` | CLI entry point |
| `internal/domain/` | Issue, CheckResult, Rating, AnalysisResult |
| `internal/config/` | YAML config loading + defaults |
| `internal/check/` | Provider interface, Registry, parallel Runner |
| `internal/check/providers/` | tsc, coverage, duplication, semgrep, gitleaks, complexity, knip, custom rules |
| `internal/rating/` | A-E ratings (Reliability, Security, Maintainability) |
| `internal/output/` | Console (box-drawing), JSON, Markdown, SARIF |
| `internal/store/` | SQLite persistence (trends, issue lifecycle) |
| `internal/git/` | git diff, branch analysis, new code detection |
