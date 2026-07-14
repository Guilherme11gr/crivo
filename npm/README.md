# crivo

Local quality gate for AI-agent coding workflows and CI. `crivo` runs core checks, consolidates the output, and applies project-specific rules before a coding session is considered done or a PR is merged.

## Install

```bash
npm install -g crivo
```

Installing the npm package does not download or build the native binary. The
first `crivo` command downloads the matching release on demand, with a bounded
timeout. To install the binary explicitly, run `node node_modules/crivo/install.js`.

## Quick Start

```bash
# Initialize in your project
crivo init

# Run the full gate
crivo run

# JSON output for agents/automation
crivo run --json

# Only changed code (for PRs/CI)
crivo run --new-code

# Save baseline for trend tracking
crivo run --save
```

## Typical Workflow

1. An agent writes code
2. `crivo run --json` checks whether the session is done
3. The agent fixes blocking issues
4. CI runs `crivo run --new-code --md report.md --sarif report.sarif`

## What it checks

| Check | Tool | What |
|-------|------|------|
| Type Safety | tsc | TypeScript compilation errors |
| Coverage | jest/vitest | Line, branch, function coverage |
| Duplication | jscpd + semantic | Copy-paste + structural clone detection |
| Secrets | gitleaks | Leaked credentials |
| Complexity | AST + regex fallback | Cognitive complexity per function |
| Dead Code | knip | Unused exports and files |
| Security | semgrep | SAST vulnerability patterns |
| Custom Rules | regex + semgrep | Team-specific project rules |

## Positioning

`crivo` is strongest as a pragmatic final gate for repositories that already use coding agents. It is not trying to replace a full code quality platform. It is meant to stop obvious regressions, catch unsafe patterns, and enforce local rules consistently.

## Output Formats

- Terminal (default) — colored box-drawing UI
- `--json` — structured JSON for agents and automation
- `--sarif report.sarif` — SARIF 2.1.0 for GitHub Code Scanning
- `--md report.md` — Markdown for PR comments

## More

See [github.com/guilherme11gr/crivo](https://github.com/guilherme11gr/crivo) for full documentation.
