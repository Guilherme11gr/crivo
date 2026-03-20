# Quality Gate

Lightweight, open-source alternative to SonarQube. Single Go binary that orchestrates existing OSS tools, normalizes output, calculates ratings, and presents results via terminal, JSON (for AI agents), SARIF, or Markdown.

## Commands

```bash
go build -o qg.exe ./cmd/qg/    # Build
go test ./internal/...            # Run tests
go vet ./...                      # Lint

qg run                            # Run all checks (default)
qg run --json                     # JSON output for AI agents
qg run --verbose                  # Full details
qg run --save                     # Save to local history
qg run --new-code                 # Only analyze changed code
qg run --sarif report.sarif       # SARIF 2.1.0 output
qg run --md report.md             # Markdown for PR comments
qg init                           # Setup in project
qg trends                         # Show sparkline history
qg version                        # Show version
```

## Architecture

- **Provider pattern**: Each check implements `check.Provider` interface (Name, ID, Detect, Analyze)
- **Parallel runner**: Goroutines with semaphore, progress channel for real-time updates
- **Orchestrator philosophy**: Runs existing tools (tsc, eslint, jest, jscpd, semgrep, gitleaks, knip), parses their output
- **Zero CGO**: Uses `modernc.org/sqlite` for pure-Go SQLite

## Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/qg/` | CLI entry point |
| `internal/domain/` | Issue, CheckResult, Rating, AnalysisResult |
| `internal/config/` | YAML config loading + defaults |
| `internal/check/` | Provider interface, Registry, parallel Runner |
| `internal/check/providers/` | tsc, eslint, coverage, duplication, semgrep, gitleaks, complexity, knip |
| `internal/rating/` | A-E ratings (Reliability, Security, Maintainability) |
| `internal/output/` | Console (box-drawing), JSON, Markdown, SARIF |
| `internal/store/` | SQLite persistence (trends, issue lifecycle) |
| `internal/git/` | git diff, branch analysis, new code detection |
