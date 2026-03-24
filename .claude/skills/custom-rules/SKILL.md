---
name: custom-rules
description: Read project docs, guidelines, or conventions and generate custom-rules for .qualitygate.yaml. Use when the user asks to create quality gate rules, enforce coding standards, add custom checks, or mentions "custom rules" in the context of quality gate / crivo.
allowed-tools: Read, Write, Edit, Bash, Glob, Grep, Agent
user-invocable: true
argument-hint: [scan|add|show|remove]
---

# Custom Rules Skill

You generate `custom-rules` entries for `.qualitygate.yaml` by reading project documentation, guidelines, and source code conventions. The goal is to turn human-readable rules into machine-enforceable regex checks that quality-gate (`crivo`) validates on every run.

## How custom-rules work

Custom rules are defined in `.qualitygate.yaml` under the `custom-rules:` key. Rules run during `crivo run` — most are regex-based, but `semgrep` rules use AST-based semantic matching. There are 8 rule types:

### Rule types reference

| Type | What it checks | Required fields | Optional fields |
|------|---------------|-----------------|-----------------|
| `ban-import` | Blocks imports of specific packages | `packages` | `files`, `allow-in`, `severity`, `message` |
| `ban-pattern` | Blocks regex pattern matches in code | `pattern` | `files`, `allow-in`, `severity`, `message` |
| `require-import` | When a pattern is used, requires import from specific module | `when-pattern`, `must-import-from` | `files`, `severity`, `message` |
| `enforce-pattern` | Requires a regex pattern to exist in matching files | `pattern` | `files`, `severity`, `message` |
| `ban-dependency` | Blocks packages in package.json | `packages` | `severity`, `message` |
| `max-lines` | Blocks files exceeding a line count | `max-lines` | `files`, `allow-in`, `severity`, `message`, `mode` |
| `semgrep` | AST-based semantic pattern matching (requires semgrep installed) | `pattern` | `language`, `files`, `allow-in`, `severity`, `message`, `mode` |

### Field reference

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique rule identifier (kebab-case) |
| `type` | string | One of the 5 types above |
| `pattern` | string | Regex pattern (YAML string, escape `\` properly) |
| `packages` | string[] | Package names for ban-import / ban-dependency |
| `when-pattern` | string | Regex — triggers require-import check |
| `must-import-from` | string | Module path that must be imported |
| `files` | string | Glob pattern for files to check (default: `src/**/*.{ts,tsx,js,jsx}`) |
| `allow-in` | string[] | Glob patterns for files exempt from this rule |
| `message` | string | Human-readable explanation shown in violations |
| `severity` | string | `blocker`, `critical`, `major` (default), `minor`, `info` |
| `ignore-comments` | bool | Skip comment lines (`//`, `/* */`, `* `). Default: `true` for ban-pattern/ban-import, `false` for others. Set to `false` explicitly to match inside comments. |
| `ignore-tests` | bool | Auto-skip test files (`*.test.*`, `*.spec.*`, `__tests__/`). Default: `true` for ban-pattern/ban-import. Set to `false` if the rule should apply to tests too. |
| `allow-subpaths` | string[] | Subpaths of banned packages that are still allowed (ban-import only). Example: `["locale"]` allows `date-fns/locale` while banning `date-fns`. |
| `mode` | string | `blocking` (default) or `advisory`. Advisory rules report violations but don't affect the gate status — useful for rolling out new rules gradually. |
| `max-lines` | int | Maximum allowed file line count (max-lines type only) |
| `language` | string | Semgrep target language (default: `ts`). Examples: `python`, `go`, `java`, `ruby`, `js` |
| `pattern-not` | string | Semgrep pattern to exclude from matches (semgrep only). Filters out results matching this pattern. |
| `pattern-inside` | string | Semgrep pattern — only match when inside this enclosing structure (semgrep only). |
| `pattern-not-inside` | string | Semgrep pattern — exclude matches inside this structure (semgrep only). |
| `metavariable-regex` | map[string]string | Constrain semgrep metavariables to regex patterns (semgrep only). Key = `$VAR`, value = regex. |

### Severity guidelines

- **blocker**: Must fix before merge. Use for security risks, banned libraries, architectural violations.
- **critical**: Should fix before merge. Use for patterns that cause bugs.
- **major**: Fix soon. Use for style/convention violations that affect maintainability.
- **minor**: Nice to fix. Use for preferences.
- **info**: Informational only.

## Subcommand: `$ARGUMENTS`

### `scan` (default if no argument)

Discover project guidelines and generate custom rules automatically.

**Steps:**

1. **Find documentation sources** — search for files that contain coding guidelines:
   ```
   Glob: **/CONTRIBUTING.md, **/CODING_STANDARDS.md, **/CONVENTIONS.md,
         **/docs/**/*.md, **/guidelines/**/*.md, **/standards/**/*.md,
         **/.cursor/rules/**/*.md, **/.cursor/rules/**/*.mdc,
         **/.github/CONTRIBUTING.md, **/ADR-*.md, **/adr-*.md,
         **/ARCHITECTURE.md, **/CLAUDE.md, **/AGENTS.md
   ```

2. **Read each doc** and extract enforceable rules. Look for:
   - "Do not use X" / "Never use X" / "Avoid X" → `ban-import` or `ban-pattern`
   - "Use X instead of Y" → `ban-import` (Y) + message mentioning X
   - "Always import from X" / "Must come from X" → `require-import`
   - "Every file must have X" / "All routes must include X" → `enforce-pattern`
   - "Don't add X to dependencies" → `ban-dependency`
   - "Don't do X except in Y" → `ban-pattern` with `allow-in`

3. **Scan source code** for existing patterns that suggest rules:
   - Check for utility modules (`**/utils/**`, `**/helpers/**`, `**/shared/**`) — these often wrap libraries that should be used via the utility
   - Check `package.json` for dependencies that have wrapper utilities
   - Check for common anti-patterns the project already avoids

4. **Read existing `.qualitygate.yaml`** to avoid duplicating rules already defined

5. **Present the proposed rules** to the user in a clear table:
   ```
   | ID | Type | What it enforces | Severity |
   |----|------|-----------------|----------|
   | no-moment | ban-import | Blocks moment.js | blocker |
   ```

6. **Ask for confirmation** before writing — the user may want to adjust severity, add exceptions, or skip some rules

7. **Write/update `.qualitygate.yaml`** — append to existing `custom-rules:` or create the section

8. **Verify** — run `crivo run --verbose` to confirm rules are detected and working

### `add`

Add a specific custom rule interactively.

1. Ask what pattern/import/dependency the user wants to enforce or ban
2. Determine the best rule type
3. Ask for severity and any exceptions (`allow-in`)
4. Write the rule to `.qualitygate.yaml`
5. Run `crivo run --verbose` to verify

### `show`

Display all current custom rules in a readable format.

1. Read `.qualitygate.yaml`
2. If `custom-rules` section exists, display as a formatted table
3. If not, tell the user no custom rules are configured and suggest `scan`

### `remove`

Remove a custom rule by ID.

1. Read `.qualitygate.yaml`
2. List current custom rules
3. Ask which one(s) to remove (or accept ID from argument)
4. Remove from config
5. Confirm removal

---

## Rule generation guidelines

When converting documentation into rules, follow these principles:

### Be precise with regex
- Escape special regex characters in YAML: `\(` → `"\\("`
- Use word boundaries when matching identifiers
- Test the regex mentally against positive AND negative cases

### Be conservative with severity
- Default to `major` unless the doc explicitly says "must", "never", "required", "forbidden"
- Use `blocker` only for clear architectural or security violations
- When in doubt, ask the user

### Always add `message`
- The message should explain WHY and WHAT to use instead
- Reference the specific utility/module/pattern to use
- Example: `"Use createLocalDate() from src/shared/utils/date-utils.ts instead of new Date()"`

### Smart defaults reduce config noise
`ban-pattern` and `ban-import` rules automatically:
- **Skip test files** via `ignore-tests: true` (default) — covers `*.test.*`, `*.spec.*`, `__tests__/`
- **Skip comment lines** via `ignore-comments: true` (default) — covers `//`, `/* */`, `* `

You do NOT need to manually add test files to `allow-in` anymore. Only use `allow-in` for:
- The utility/wrapper file itself (e.g., `"**/date-utils.ts"`)
- Config files: `"**/*.config.*"`
- Migration files, scripts, seed files
- E2E test directories (e.g., `"e2e/**"`)
- Storybook stories: `"**/*.stories.tsx"`

Set `ignore-tests: false` or `ignore-comments: false` explicitly only when the rule should apply to those contexts.

### Use `allow-subpaths` for fine-grained import control
When banning a package but some subpaths are legitimate:
```yaml
- id: no-date-fns
  type: ban-import
  packages: ["date-fns"]
  allow-subpaths: ["locale"]  # allows date-fns/locale/* while banning date-fns/*
  message: "Use date-utils wrapper"
  severity: blocker
```

### Use `mode: advisory` for gradual rollout
New rules that you want to observe before enforcing:
```yaml
- id: no-raw-fetch
  type: ban-pattern
  pattern: "fetch\\("
  message: "Use apiClient() wrapper"
  severity: major
  mode: advisory  # reports but doesn't block the gate
```

### Use `type: semgrep` for semantic checks regex can't handle
When a pattern needs AST awareness (e.g., matching function calls regardless of formatting, metavariables, taint tracking), use `type: semgrep` instead of `ban-pattern`. Requires semgrep installed on the machine — if not present, the rule is silently skipped.

```yaml
- id: no-eval
  type: semgrep
  pattern: "eval(...)"
  language: ts
  message: "eval() is a security risk. Use safer alternatives."
  severity: blocker

- id: no-innerhtml
  type: semgrep
  pattern: "$X.innerHTML = $Y"
  language: ts
  files: "src/**/*.{ts,tsx}"
  message: "Direct innerHTML assignment is an XSS risk. Use DOMPurify or React."
  severity: blocker
```

Advanced example with `pattern-not-inside` and `metavariable-regex`:
```yaml
- id: no-manual-cents-ast
  type: semgrep
  pattern: "$X / 100"
  pattern-not-inside: "function centsToReais(...) { ... }"
  metavariable-regex:
    "$X": ".*[Cc]ents.*"
  language: ts
  files: "src/**/*.{ts,tsx}"
  message: "Use centsToReais() instead of manual cents conversion."
  severity: major

- id: no-raw-auth-ast
  type: semgrep
  pattern: "$CLIENT.auth.getUser()"
  pattern-not-inside: "function extractAuthenticatedTenant(...) { ... }"
  language: ts
  files: "src/app/api/**/route.ts"
  allow-in:
    - "**/shared/http/auth-helpers.ts"
  message: "Use extractAuthenticatedTenant() instead of calling auth.getUser() directly."
  severity: blocker
```

**When to use semgrep vs ban-pattern:**
- Use `ban-pattern` for simple text matches (e.g., `console.log`, string literals, import statements)
- Use `semgrep` when you need: metavariables (`$X`), structure matching across line breaks, `pattern-not-inside` for context-aware exclusions, or `metavariable-regex` for constrained matching

### Use `files` to scope rules
- Don't apply frontend rules to backend code and vice versa
- API route rules should target `"src/app/api/**/route.ts"` or equivalent
- Component rules should target `"src/**/*.tsx"`

---

## Example output

Given a doc that says:
> "Always use our date-utils wrapper. Never import date-fns directly. Never use `new Date()` except in date-utils itself and tests."

Generate:

```yaml
custom-rules:
  - id: no-date-libs
    type: ban-import
    packages: ["date-fns", "moment", "dayjs", "luxon"]
    message: "Use src/shared/utils/date-utils.ts for date manipulation"
    severity: blocker

  - id: no-raw-date
    type: ban-pattern
    pattern: "new Date\\("
    allow-in: ["**/date-utils.ts", "**/*.test.ts", "**/*.spec.ts"]
    message: "Use createLocalDate() or parseUTCString() from date-utils.ts"
    severity: blocker
    files: "src/**/*.{ts,tsx}"
```

---

## Integration with crivo run

After writing rules, always verify:

```bash
# Build latest binary (if in quality-gate repo)
go build -o crivo.exe ./cmd/crivo/

# Run with verbose to see custom rules detection
crivo run --verbose

# JSON output for programmatic verification
crivo run --json | jq '.checks[] | select(.id == "custom-rules")'
```

The custom-rules check will appear as a separate check in the output alongside typescript, eslint, etc.
