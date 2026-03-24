# crivo

Lightweight, open-source alternative to SonarQube. Single binary that orchestrates existing OSS tools (tsc, eslint, jest, jscpd, semgrep, gitleaks), calculates A-E ratings, and enforces quality gates.

## Install

```bash
npm install -g crivo
```

## Quick Start

```bash
# Initialize in your project
crivo init

# Run all checks
crivo run

# JSON output (for CI/AI agents)
crivo run --json

# Only changed code (for PRs)
crivo run --new-code

# Save baseline for trend tracking
crivo run --save
```

## What it checks

| Check | Tool | What |
|-------|------|------|
| Type Safety | tsc | TypeScript compilation errors |
| Code Quality | eslint | Lint errors and warnings |
| Coverage | jest/vitest | Line, branch, function coverage |
| Duplication | jscpd + semantic | Copy-paste + structural clone detection |
| Secrets | gitleaks | Leaked credentials |
| Complexity | custom AST | Cognitive complexity per function |
| Dead Code | knip | Unused exports and files |
| Security | semgrep | SAST vulnerability patterns (auto-installed) |

## Output Formats

- Terminal (default) — colored box-drawing UI
- `--json` — structured JSON for AI agents
- `--sarif report.sarif` — SARIF 2.1.0 for GitHub Code Scanning
- `--md report.md` — Markdown for PR comments

## More

See [github.com/guilherme11gr/crivo](https://github.com/guilherme11gr/crivo) for full documentation.
