# quality-gate

Lightweight, open-source alternative to SonarQube. Single binary that orchestrates existing OSS tools (tsc, eslint, jest, jscpd, semgrep, gitleaks), calculates A-E ratings, and enforces quality gates.

## Install

```bash
npm install -g quality-gate
```

## Quick Start

```bash
# Initialize in your project
qg init

# Run all checks
qg run

# JSON output (for CI/AI agents)
qg run --json

# Only changed code (for PRs)
qg run --new-code

# Save baseline for trend tracking
qg run --save
```

## What it checks

| Check | Tool | What |
|-------|------|------|
| Type Safety | tsc | TypeScript compilation errors |
| Code Quality | eslint | Lint errors and warnings |
| Coverage | jest/vitest | Line, branch, function coverage |
| Duplication | jscpd | Copy-paste detection |
| Secrets | gitleaks | Leaked credentials |
| Complexity | custom AST | Cognitive complexity per function |
| Dead Code | knip | Unused exports and files |
| Security | semgrep | SAST vulnerability patterns |

## Output Formats

- Terminal (default) — colored box-drawing UI
- `--json` — structured JSON for AI agents
- `--sarif report.sarif` — SARIF 2.1.0 for GitHub Code Scanning
- `--md report.md` — Markdown for PR comments

## More

See [github.com/anthropics/quality-gate](https://github.com/anthropics/quality-gate) for full documentation.
