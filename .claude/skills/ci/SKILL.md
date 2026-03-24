---
name: ci
description: Set up and manage quality-gate CI pipelines. Use when the user asks to add quality checks to CI/CD, configure GitHub Actions, GitLab CI, or any pipeline with crivo. Also use when debugging CI failures related to quality gate.
allowed-tools: Read, Write, Edit, Bash, Glob, Grep, Agent
user-invocable: true
argument-hint: [setup|diagnose|pr-comment|baseline|show-config]
---

# Quality Gate CI Skill

You are an expert at integrating **quality-gate (`crivo`)** into CI/CD pipelines. This skill covers setup, scripts, debugging, and advanced patterns.

## What is quality-gate?

A single Go binary (`crivo`) that orchestrates existing OSS tools (tsc, eslint, jest, jscpd, semgrep, gitleaks) and produces unified reports. Zero config to start, `.qualitygate.yaml` to customize.

**Key outputs:**
- Exit code 0 (passed) or 1 (failed) — native CI gate
- `--json` → structured JSON for AI agents and automation
- `--sarif report.sarif` → GitHub Code Scanning integration
- `--md report.md` → PR comment body
- `--save` → local SQLite history for baseline comparison
- `--new-code` → only flag issues in changed files/lines

## Subcommand: `$ARGUMENTS`

Route based on the argument:

### `setup` (default if no argument)

Walk the user through full CI setup. Detect their CI platform and project type, then generate the appropriate config. Follow these steps:

1. **Detect CI platform** — look for `.github/workflows/`, `.gitlab-ci.yml`, `Jenkinsfile`, `.circleci/`, `bitbucket-pipelines.yml`, `azure-pipelines.yml`
2. **Detect project type** — check `package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`
3. **Check if `.qualitygate.yaml` exists** — if not, offer to create one
4. **Generate CI config** using the templates below
5. **Generate helper scripts** from `${CLAUDE_SKILL_DIR}/scripts/`
6. **Explain what each part does** — don't just dump config

### `diagnose`

Debug a failing CI quality gate run:
1. Ask for the CI log or read it from a provided URL/file
2. Check common issues (see Troubleshooting section)
3. Suggest fixes

### `pr-comment`

Set up automatic PR comments with quality gate results:
1. Generate the markdown report step
2. Add the PR comment action (platform-specific)
3. Show how to customize the comment template

### `baseline`

Set up baseline comparison so legacy debt doesn't block PRs:
1. Run initial `crivo run --save` to capture baseline
2. Configure CI to use `--new-code` for PRs
3. Set up the baseline update workflow on main branch merges

### `show-config`

Show the current quality gate configuration and explain each field. Read `.qualitygate.yaml` and annotate it.

---

## CI Platform Templates

### GitHub Actions (Primary)

Generate this workflow at `.github/workflows/quality-gate.yml`:

```yaml
name: Quality Gate

on:
  pull_request:
    branches: [main, develop]
  push:
    branches: [main]

permissions:
  contents: read
  pull-requests: write
  security-events: write  # For SARIF upload

jobs:
  quality-gate:
    name: Quality Gate
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for --new-code and git blame

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - run: npm ci

      - name: Install quality-gate
        run: |
          curl -fsSL https://github.com/guilherme11gr/crivo/releases/latest/download/quality-gate_linux_amd64.tar.gz | tar xz
          sudo mv crivo /usr/local/bin/

      - name: Run Quality Gate (PR)
        if: github.event_name == 'pull_request'
        run: |
          crivo run \
            --new-code \
            --json > qg-output.json \
            --md qg-report.md \
            --sarif qg-report.sarif \
            --save
        continue-on-error: true
        id: qg

      - name: Run Quality Gate (Push to main)
        if: github.event_name == 'push'
        run: crivo run --save --sarif qg-report.sarif

      - name: Upload SARIF
        if: always() && hashFiles('qg-report.sarif') != ''
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: qg-report.sarif

      - name: Comment PR
        if: github.event_name == 'pull_request' && always()
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          path: qg-report.md

      - name: Gate Decision
        if: github.event_name == 'pull_request'
        run: |
          if [ "${{ steps.qg.outcome }}" = "failure" ]; then
            echo "::error::Quality Gate FAILED — see PR comment for details"
            exit 1
          fi
```

### GitLab CI

Generate at `.gitlab-ci.yml` (or append to existing):

```yaml
quality-gate:
  stage: test
  image: node:20
  before_script:
    - curl -fsSL https://github.com/guilherme11gr/crivo/releases/latest/download/quality-gate_linux_amd64.tar.gz | tar xz
    - mv crivo /usr/local/bin/
    - npm ci
  script:
    - crivo run --new-code --md qg-report.md --sarif qg-report.sarif --save
  artifacts:
    reports:
      sast: qg-report.sarif
    paths:
      - qg-report.md
    when: always
  rules:
    - if: $CI_MERGE_REQUEST_IID
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

### Generic (Jenkins, CircleCI, etc.)

Use the helper scripts approach — generate standalone shell scripts that any CI can call:

```bash
# Install
curl -fsSL https://github.com/guilherme11gr/crivo/releases/latest/download/quality-gate_linux_amd64.tar.gz | tar xz
export PATH=$PWD:$PATH

# Run
crivo run --new-code --json > qg-output.json --md qg-report.md --save
```

---

## Helper Scripts

The following scripts live in `${CLAUDE_SKILL_DIR}/scripts/` and should be copied to the user's project at `scripts/ci/` when setting up CI.

### `scripts/ci/qg-install.sh`
Portable installer that works on Linux/macOS, detects arch, verifies checksum.

### `scripts/ci/qg-gate.sh`
Runs the quality gate with smart defaults based on context (PR vs push, new-code vs full).

### `scripts/ci/qg-baseline.sh`
Updates the baseline database — run on main branch merges only.

### `scripts/ci/qg-pr-comment.sh`
Generates and posts a PR comment from the markdown report.

### `scripts/ci/qg-parse-json.sh`
Parses the JSON output and extracts key metrics for custom integrations.

When setting up CI, copy these scripts with:
```bash
cp ${CLAUDE_SKILL_DIR}/scripts/*.sh scripts/ci/
chmod +x scripts/ci/*.sh
```

---

## Configuration Reference

### `.qualitygate.yaml` — Full annotated example

```yaml
# Profile: "balanced" (default), "strict", "lenient"
# strict = lower thresholds, more checks enabled
# lenient = higher thresholds, only critical checks
profile: balanced

# Languages detected in the project
languages:
  - typescript
  - javascript

# Source directories to analyze (relative to project root)
src:
  - src/
  - lib/

# Directories/patterns to exclude
exclude:
  - node_modules/
  - dist/
  - .next/
  - coverage/
  - "*.min.js"
  - "*.generated.*"

# Enable/disable individual checks
checks:
  typescript: true    # tsc --noEmit (type errors)
  eslint: true        # eslint with project config
  coverage: true      # jest --coverage
  duplication: true   # jscpd (copy-paste detection)
  semgrep: false      # semgrep SAST (needs semgrep installed)
  secrets: false      # gitleaks (secret detection)
  dead-code: false    # knip (unused exports/deps)

# Coverage thresholds (percentage)
coverage:
  lines: 60
  branches: 50
  functions: 60
  statements: 60

# Duplication thresholds
duplication:
  threshold: 5        # max % duplicated
  min-lines: 5        # minimum block size
  min-tokens: 50      # minimum token count

# Cognitive complexity threshold per function
complexity:
  threshold: 15       # SonarSource cognitive complexity max

# Quality gate conditions
quality-gate:
  new-code:           # Applied with --new-code
    coverage: 80      # New code must have 80% coverage
    bugs: 0           # Zero new bugs
    vulnerabilities: 0
    duplications: 3
  overall:            # Applied on full codebase
    coverage: 60
    bugs: 0
    vulnerabilities: 0
    duplications: 5
```

---

## Troubleshooting

### Common CI failures

| Symptom | Cause | Fix |
|---------|-------|-----|
| `node not found` | crivo needs node for AST analysis + jest | Add `setup-node` step before crivo |
| `typescript not in node_modules` | Missing `npm ci` before crivo run | Add `npm ci` step |
| `tsc: command not found` | TypeScript not in devDependencies | `npm i -D typescript` |
| Coverage shows 0% | Jest not configured or no test files | Check `jest.config.*` exists |
| `--new-code` shows no issues | Shallow clone, no git history | Use `fetch-depth: 0` |
| SARIF upload fails | Missing `security-events: write` permission | Add to workflow permissions |
| Gate fails on legacy code | No baseline saved | Run `crivo run --save` on main first |
| Duplication timeout | Huge codebase, no excludes | Add excludes to `.qualitygate.yaml` |

### Reading the JSON output

The JSON output (`--json`) has this structure — teach agents to parse it:

```json
{
  "projectDir": "/path/to/project",
  "status": "passed|failed",
  "checks": [
    {
      "id": "typescript",
      "name": "Type Safety",
      "status": "passed|failed|warning|skipped|error",
      "summary": "3 prod errors, 12 in tests",
      "issues": [
        {
          "ruleId": "TS2345",
          "message": "Argument of type 'string' is not assignable...",
          "file": "src/api/booking.ts",
          "line": 42,
          "column": 5,
          "severity": "major",
          "type": "bug",
          "source": "tsc",
          "effort": "10min"
        }
      ],
      "metrics": {
        "errors": 15,
        "prod_errors": 3,
        "test_errors": 12
      }
    }
  ],
  "conditions": [
    { "metric": "type_errors", "operator": "lt", "threshold": 1, "actual": 3, "passed": false }
  ],
  "ratings": {
    "Reliability": "C",
    "Security": "A",
    "Maintainability": "B"
  },
  "totalIssues": 45,
  "totalDuration": "12.5s",
  "timestamp": "2026-03-22T10:30:00Z"
}
```

**Key fields for automation:**
- `status` → gate pass/fail (exit code mirrors this)
- `checks[].status` → per-check status
- `checks[].metrics` → numeric values for trending
- `checks[].issues[]` → individual findings with file:line
- `conditions[]` → which gate conditions passed/failed
- `ratings` → A-E quality ratings (Reliability, Security, Maintainability)

### Feeding output to an AI agent

```bash
# Run crivo, capture JSON, send to agent
crivo run --json > qg-output.json

# Example: ask Claude to fix the top issues
cat qg-output.json | claude "Fix the top 5 most critical issues from this quality gate report"

# Example: extract just failing checks
jq '.checks[] | select(.status == "failed") | {name, summary, issues: [.issues[:3][]]}' qg-output.json
```
