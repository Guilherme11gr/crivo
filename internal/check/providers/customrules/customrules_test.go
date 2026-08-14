package customrules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

// ─── Rule Compilation ───────────────────────────────────────────────────────

// ─── Test Fixtures (plan 008) ────────────────────────────────────────────────

func TestCompileRules_FixtureBanPatternPasses(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "no-eval", Type: "ban-pattern", Pattern: `\beval\s*\(`, Message: "No eval",
			Tests: []config.TestSpec{
				{Code: "const x = eval(userInput)", Match: true},
				{Code: "const x = evaluate(userInput)", Match: false},
			},
		},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors for correct fixtures, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
}

func TestCompileRules_FixtureBanPatternWrongExpectationIsError(t *testing.T) {
	// The fixture says the code must NOT match, but the pattern does match it.
	rules := []config.CustomRule{
		{
			ID: "no-eval", Type: "ban-pattern", Pattern: `\beval\s*\(`, Message: "No eval",
			Tests: []config.TestSpec{
				{Code: "const x = eval(userInput)", Match: false},
			},
		},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 {
		t.Fatalf("expected 1 fixture error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "no-eval") {
		t.Errorf("error should cite the rule id, got %q", msg)
	}
	if !strings.Contains(msg, "fixture 1") {
		t.Errorf("error should cite the fixture case, got %q", msg)
	}
	if !strings.Contains(msg, "const x = eval(userInput)") {
		t.Errorf("error should cite the fixture code, got %q", msg)
	}
	if !strings.Contains(msg, "expected the rule to NOT match") {
		t.Errorf("error should state expected vs got, got %q", msg)
	}
}

func TestCompileRules_FixtureBanPatternMissedMatchIsError(t *testing.T) {
	// The fixture says the code MUST match, but the pattern does not fire.
	rules := []config.CustomRule{
		{
			ID: "no-eval", Type: "ban-pattern", Pattern: `\beval\s*\(`, Message: "No eval",
			Tests: []config.TestSpec{
				{Code: "const x = evaluate(userInput)", Match: true},
			},
		},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 {
		t.Fatalf("expected 1 fixture error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "expected the rule to match, but it did not") {
		t.Errorf("unexpected error message: %q", errs[0].Error())
	}
}

func TestCompileRules_FixtureBanImport(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "no-moment", Type: "ban-import", Packages: []string{"moment"}, Message: "Use date-fns",
			Tests: []config.TestSpec{
				{Code: "import moment from 'moment'", Match: true},
				{Code: "import { format } from 'date-fns'", Match: false},
			},
		},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
}

func TestCompileRules_FixtureMaxLines(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "component-size", Type: "max-lines", MaxLines: 3, Message: "Too big",
			Tests: []config.TestSpec{
				{Code: "a\nb\nc\nd", Match: true}, // 4 lines > 3
				{Code: "a\nb\nc", Match: false},   // 3 lines == 3
			},
		},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
}

func TestCompileRules_FixtureRequireImport(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "dates-from-utils", Type: "require-import",
			MustImportFrom: "@/shared/utils/date-utils",
			WhenPattern:    `formatDate`,
			Message:        "Import from date-utils",
			Tests: []config.TestSpec{
				{Code: "const d = formatDate(new Date())", Match: true},
				{Code: "import { formatDate } from '@/shared/utils/date-utils'\nconst d = formatDate(new Date())", Match: false},
			},
		},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
}

func TestCompileRules_FixtureEnforcePattern(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "api-rate-limit", Type: "enforce-pattern", Pattern: `applyRateLimit`, Message: "Need rate limiting",
			Tests: []config.TestSpec{
				{Code: "export function GET() {}", Match: true},
				{Code: "applyRateLimit(req)", Match: false},
			},
		},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
}

func TestCompileRules_FixtureEmptyCodeIsError(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "no-eval", Type: "ban-pattern", Pattern: `eval`, Message: "No eval",
			Tests: []config.TestSpec{{Code: "", Match: true}},
		},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "empty 'code'") {
		t.Fatalf("expected empty-code error, got: %v", errs)
	}
}

func TestCompileRules_FixtureCollectsAllErrors(t *testing.T) {
	rules := []config.CustomRule{
		{
			ID: "a", Type: "ban-pattern", Pattern: `eval`, Message: "m",
			Tests: []config.TestSpec{{Code: "eval(x)", Match: false}},
		},
		{
			ID: "b", Type: "ban-pattern", Pattern: `eval`, Message: "m",
			Tests: []config.TestSpec{{Code: "eval(x)", Match: false}},
		},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 2 {
		t.Fatalf("expected 2 fixture errors (one per rule), got %d: %v", len(errs), errs)
	}
}

func TestCompileRules_FixtureSemgrepWithoutBinaryWarns(t *testing.T) {
	// semgrep is not installed in this environment: fixtures must NOT fail
	// compilation — they are collected as a warning (plan 008, coherent with
	// plan 002's runtime skip).
	t.Setenv("PATH", "/nonexistent-bin")
	t.Setenv("CRIVO_NO_AUTO_INSTALL", "1")

	rules := []config.CustomRule{
		{
			ID: "no-eval", Type: "semgrep", Pattern: "eval(...)", Message: "No eval",
			Tests: []config.TestSpec{
				{Code: "const x = eval(userInput)", Match: true},
			},
		},
	}
	compiled, errs, warnings := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors when semgrep is unavailable, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning about unvalidated fixtures, got %d: %#v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "no-eval") || !strings.Contains(warnings[0], "NOT validated") {
		t.Errorf("expected warning naming the rule and the unvalidated state, got %q", warnings[0])
	}
}

func TestCompileRules_FixtureBanDependencyWarns(t *testing.T) {
	// ban-dependency matches package.json on disk, not code: fixtures cannot be
	// validated and must not fail compilation.
	rules := []config.CustomRule{
		{
			ID: "no-axios", Type: "ban-dependency", Packages: []string{"axios"}, Message: "Use fetch",
			Tests: []config.TestSpec{{Code: "anything", Match: true}},
		},
	}
	compiled, errs, warnings := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no-axios") {
		t.Fatalf("expected a ban-dependency fixture warning, got %#v", warnings)
	}
}

func TestProvider_Analyze_FixtureErrorFailsCompilation(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1\n")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{
			ID: "no-eval", Type: "ban-pattern", Pattern: `\beval\s*\(`, Message: "No eval",
			Tests: []config.TestSpec{{Code: "const x = eval(userInput)", Match: false}},
		},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusError {
		t.Fatalf("expected error status (fixture disagrees with matcher), got %s", result.Status)
	}
	if !strings.Contains(result.Summary, "compilation errors") {
		t.Errorf("expected compilation error summary, got %q", result.Summary)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "no-eval") && strings.Contains(d, "fixture 1") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a detail citing the rule and fixture case, got %#v", result.Details)
	}
}

func TestProvider_Analyze_FixtureWarningSurfaced(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1\n")

	t.Setenv("PATH", "/nonexistent-bin")
	t.Setenv("CRIVO_NO_AUTO_INSTALL", "1")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{
			ID: "no-eval", Type: "semgrep", Pattern: "eval(...)", Message: "No eval",
			Tests: []config.TestSpec{{Code: "const x = eval(userInput)", Match: true}},
		},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The rule still compiles; the runtime semgrep skip forces a warning status.
	if result.Status != domain.StatusWarning {
		t.Fatalf("expected warning (semgrep skipped), got %s", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "NOT validated") && strings.Contains(d, "no-eval") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a fixture-not-validated detail, got %#v", result.Details)
	}
}

func TestCompileRules_ValidBanImport(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "no-moment", Type: "ban-import", Packages: []string{"moment"}, Message: "Use date-fns"},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
	if compiled[0].Type != RuleTypeBanImport {
		t.Errorf("expected type ban-import, got %s", compiled[0].Type)
	}
	if compiled[0].Severity != domain.SeverityMajor {
		t.Errorf("expected default severity major, got %s", compiled[0].Severity)
	}
}

func TestCompileRules_ValidBanPattern(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "no-raw-date", Type: "ban-pattern", Pattern: `new Date\(`, Message: "Use createLocalDate()", Severity: "blocker"},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if compiled[0].PatternRe == nil {
		t.Fatal("expected compiled regex")
	}
	if compiled[0].Severity != domain.SeverityBlocker {
		t.Errorf("expected severity blocker, got %s", compiled[0].Severity)
	}
}

func TestCompileRules_MissingID(t *testing.T) {
	rules := []config.CustomRule{
		{Type: "ban-import", Packages: []string{"moment"}, Message: "no"},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "missing required field 'id'") {
		t.Fatalf("expected missing id error, got: %v", errs)
	}
}

func TestCompileRules_DuplicateID(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "dup", Type: "ban-import", Packages: []string{"x"}, Message: "m"},
		{ID: "dup", Type: "ban-import", Packages: []string{"y"}, Message: "m"},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "duplicate id") {
		t.Fatalf("expected duplicate id error, got: %v", errs)
	}
}

func TestCompileRules_UnknownType(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "x", Type: "unknown-type", Message: "m"},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "unknown type") {
		t.Fatalf("expected unknown type error, got: %v", errs)
	}
}

func TestCompileRules_InvalidRegex(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "bad-re", Type: "ban-pattern", Pattern: "[invalid", Message: "m"},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "invalid regex") {
		t.Fatalf("expected invalid regex error, got: %v", errs)
	}
}

func TestCompileRules_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		rule config.CustomRule
		want string
	}{
		{"ban-import no packages", config.CustomRule{ID: "a", Type: "ban-import", Message: "m"}, "requires 'packages'"},
		{"ban-pattern no pattern", config.CustomRule{ID: "b", Type: "ban-pattern", Message: "m"}, "requires 'pattern'"},
		{"require-import no must-import-from", config.CustomRule{ID: "c", Type: "require-import", Message: "m"}, "requires 'must-import-from'"},
		{"enforce-pattern no pattern", config.CustomRule{ID: "d", Type: "enforce-pattern", Message: "m"}, "requires 'pattern'"},
		{"ban-dependency no packages", config.CustomRule{ID: "e", Type: "ban-dependency", Message: "m"}, "requires 'packages'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs, _ := CompileRules([]config.CustomRule{tt.rule})
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), tt.want) {
				t.Fatalf("expected error containing %q, got: %v", tt.want, errs)
			}
		})
	}
}

func TestCompileRules_MissingMessage(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "x", Type: "ban-import", Packages: []string{"moment"}},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "missing required field 'message'") {
		t.Fatalf("expected missing message error, got: %v", errs)
	}
}

func TestCompileRules_CollectsAllErrors(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "a", Type: "ban-pattern", Message: "m"}, // missing pattern
		{ID: "b", Type: "unknown", Message: "m"},     // unknown type
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

// ─── Glob Matching ──────────────────────────────────────────────────────────

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"src/**/*.ts", "src/app/page.ts", true},
		{"src/**/*.ts", "src/page.ts", true},
		{"src/**/*.ts", "src/a/b/c/d.ts", true},
		{"src/**/*.ts", "src/app/page.tsx", false},
		{"src/**/*.{ts,tsx}", "src/app/page.tsx", true},
		{"src/**/*.{ts,tsx}", "src/app/page.ts", true},
		{"**/*.test.ts", "src/utils/date.test.ts", true},
		{"**/*.test.ts", "date.test.ts", true},
		{"**/date-utils.ts", "src/shared/utils/date-utils.ts", true},
		{"*.ts", "file.ts", true},
		{"*.ts", "src/file.ts", false},
		{"src/app/api/**/route.ts", "src/app/api/users/route.ts", true},
		{"src/app/api/**/route.ts", "src/app/api/users/[id]/route.ts", true},
		{"src/app/api/**/route.ts", "src/app/page.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// ─── Ban Import Matcher ─────────────────────────────────────────────────────

func TestMatchBanImport(t *testing.T) {
	rule := compileRule(t, config.CustomRule{
		ID:       "no-moment",
		Type:     "ban-import",
		Packages: []string{"moment", "dayjs"},
		Message:  "banned",
	})

	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"ES import", `import moment from 'moment'`, 1},
		{"ES import with sub-path", `import { format } from 'moment/format'`, 1},
		{"require", `const m = require('moment')`, 1},
		{"dayjs", `import dayjs from 'dayjs'`, 1},
		{"safe name", `import safemoment from 'safe-moment'`, 0},
		{"no match", `import React from 'react'`, 0},
		{"multiple on same line", `import 'moment'; import 'dayjs'`, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.content, "\n")
			issues := matchBanImport(rule, "test.ts", lines)
			if len(issues) != tt.want {
				t.Errorf("expected %d issues, got %d: %+v", tt.want, len(issues), issues)
			}
		})
	}
}

// ─── Ban Pattern Matcher ────────────────────────────────────────────────────

func TestMatchBanPattern(t *testing.T) {
	rule := CompiledRule{
		Raw:          config.CustomRule{ID: "no-raw-date", Message: "Use createLocalDate()"},
		Type:         RuleTypeBanPattern,
		PatternRe:    mustCompile(`new Date\(`),
		Severity:     domain.SeverityBlocker,
		AllowInGlobs: []string{"**/date-utils.ts", "**/*.test.ts"},
	}

	t.Run("matches pattern", func(t *testing.T) {
		lines := []string{"const d = new Date()", "const x = 1", "const e = new Date('2024')"}
		issues := matchBanPattern(rule, "src/app.ts", lines)
		if len(issues) != 2 {
			t.Errorf("expected 2 issues, got %d", len(issues))
		}
		if len(issues) > 0 && issues[0].Line != 1 {
			t.Errorf("expected line 1, got %d", issues[0].Line)
		}
	})

	t.Run("allow-in skips file", func(t *testing.T) {
		lines := []string{"const d = new Date()"}
		issues := matchBanPattern(rule, "src/shared/utils/date-utils.ts", lines)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues for allowed file, got %d", len(issues))
		}
	})

	t.Run("allow-in test file", func(t *testing.T) {
		lines := []string{"const d = new Date()"}
		issues := matchBanPattern(rule, "src/utils/date.test.ts", lines)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues for test file, got %d", len(issues))
		}
	})
}

func TestMatchBanPattern_IgnoreComments(t *testing.T) {
	rule := CompiledRule{
		Raw:            config.CustomRule{ID: "no-console", Message: "Use logger"},
		Type:           RuleTypeBanPattern,
		PatternRe:      mustCompile(`console\.log\(`),
		Severity:       domain.SeverityMajor,
		IgnoreComments: true,
	}

	lines := []string{
		"// console.log('commented out')",
		"/* console.log('block comment') */",
		" * console.log('jsdoc example')",
		"  // TODO: remove console.log(x)",
		"console.log('real violation')",
		"doSomething(); console.log('inline real')",
		"   */ console.log('end block')",
	}
	issues := matchBanPattern(rule, "src/app.ts", lines)
	// Lines 1-4 are comment lines (skipped), line 7 starts with */ (skipped)
	// Lines 5 and 6 are real code with console.log → 2 issues
	if len(issues) != 2 {
		t.Errorf("expected 2 issues (skipping comment lines), got %d", len(issues))
		for _, iss := range issues {
			t.Logf("  line %d: %s", iss.Line, lines[iss.Line-1])
		}
	}
}

func TestMatchBanPattern_IgnoreCommentsDisabled(t *testing.T) {
	rule := CompiledRule{
		Raw:            config.CustomRule{ID: "no-console", Message: "Use logger"},
		Type:           RuleTypeBanPattern,
		PatternRe:      mustCompile(`console\.log\(`),
		Severity:       domain.SeverityMajor,
		IgnoreComments: false,
	}

	lines := []string{
		"// console.log('commented out')",
		"console.log('real')",
	}
	issues := matchBanPattern(rule, "src/app.ts", lines)
	if len(issues) != 2 {
		t.Errorf("expected 2 issues (comments not ignored), got %d", len(issues))
	}
}

func TestMatchBanImport_IgnoreComments(t *testing.T) {
	rule := compileRule(t, config.CustomRule{
		ID:            "no-moment",
		Type:          "ban-import",
		Packages:      []string{"moment"},
		Message:       "banned",
		IgnoreComments: boolPtr(true),
	})

	lines := []string{
		"// import moment from 'moment'",
		"/* import moment from 'moment' */",
		"import moment from 'moment'",
	}
	issues := matchBanImport(rule, "test.ts", lines)
	if len(issues) != 1 {
		t.Errorf("expected 1 issue (skipping comments), got %d", len(issues))
	}
	if len(issues) > 0 && issues[0].Line != 3 {
		t.Errorf("expected issue on line 3, got %d", issues[0].Line)
	}
}

func TestIsCommentLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"// single line comment", true},
		{"  // indented comment", true},
		{"/* block start */", true},
		{" * jsdoc line", true},
		{" */", true},
		{"*", true},
		{"const x = 1", false},
		{"const x = 1 // trailing comment", false},
		{"import foo from 'bar'", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := isCommentLine(tt.line)
			if got != tt.want {
				t.Errorf("isCommentLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// ─── Require Import Matcher ─────────────────────────────────────────────────

func TestMatchRequireImport(t *testing.T) {
	rule := compileRule(t, config.CustomRule{
		ID:             "dates-from-utils",
		Type:           "require-import",
		MustImportFrom: "@/shared/utils/date-utils",
		Message:        "Import from date-utils",
		WhenPattern:    `(formatDate|parseUTCString|createLocalDate)`,
	})

	t.Run("has pattern and import", func(t *testing.T) {
		content := `import { formatDate } from '@/shared/utils/date-utils'
const d = formatDate(new Date())`
		issues := matchRequireImport(rule, "src/app.ts", content)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}
	})

	t.Run("has pattern but no import", func(t *testing.T) {
		content := `const d = formatDate(new Date())`
		issues := matchRequireImport(rule, "src/app.ts", content)
		if len(issues) != 1 {
			t.Errorf("expected 1 issue, got %d", len(issues))
		}
	})

	t.Run("no pattern match skips", func(t *testing.T) {
		content := `const x = 1`
		issues := matchRequireImport(rule, "src/app.ts", content)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}
	})
}

// ─── Enforce Pattern Matcher ────────────────────────────────────────────────

func TestMatchEnforcePattern(t *testing.T) {
	rule := CompiledRule{
		Raw:       config.CustomRule{ID: "api-rate-limit", Message: "Need rate limiting"},
		Type:      RuleTypeEnforcePattern,
		PatternRe: mustCompile(`applyRateLimit`),
		Severity:  domain.SeverityMajor,
	}

	t.Run("pattern present", func(t *testing.T) {
		issues := matchEnforcePattern(rule, "src/app/api/route.ts", "applyRateLimit(req)")
		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}
	})

	t.Run("pattern absent", func(t *testing.T) {
		issues := matchEnforcePattern(rule, "src/app/api/route.ts", "export function GET() {}")
		if len(issues) != 1 {
			t.Errorf("expected 1 issue, got %d", len(issues))
		}
	})
}

// ─── Ban Dependency Matcher ─────────────────────────────────────────────────

func TestMatchBanDependency(t *testing.T) {
	rule := CompiledRule{
		Raw:      config.CustomRule{ID: "no-axios", Packages: []string{"axios", "got"}, Message: "Use native fetch"},
		Type:     RuleTypeBanDependency,
		Severity: domain.SeverityBlocker,
	}

	t.Run("finds banned dep", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "package.json", `{
  "dependencies": {
    "react": "^18.0.0",
    "axios": "^1.5.0"
  },
  "devDependencies": {
    "got": "^13.0.0"
  }
}`)
		issues := matchBanDependency(rule, dir, nil)
		if len(issues) != 2 {
			t.Errorf("expected 2 issues, got %d: %+v", len(issues), issues)
		}
	})

	t.Run("no banned deps", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "package.json", `{
  "dependencies": {
    "react": "^18.0.0"
  }
}`)
		issues := matchBanDependency(rule, dir, nil)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}
	})

	t.Run("no package.json", func(t *testing.T) {
		issues := matchBanDependency(rule, t.TempDir(), nil)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}
	})

	t.Run("skips when package json did not change in scoped mode", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "package.json", `{"dependencies":{"axios":"^1.5.0"}}`)
		issues := matchBanDependency(rule, dir, map[string]bool{"src/app.ts": true})
		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}
	})
}

func TestFilesForGlob_ScopedFiles(t *testing.T) {
	files, err := filesForGlob(context.Background(), t.TempDir(), "src/**/*.{ts,tsx}", nil, []string{"src/app/page.tsx", "src/lib/util.ts", "scripts/build.js"})
	if err != nil {
		t.Fatalf("filesForGlob() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %#v", len(files), files)
	}
}

// ─── Provider Integration ───────────────────────────────────────────────────

func TestProvider_Detect(t *testing.T) {
	p := New()

	t.Run("no config", func(t *testing.T) {
		dir := t.TempDir()
		if p.Detect(context.Background(), dir) {
			t.Error("expected Detect=false with no config")
		}
	})

	t.Run("with custom rules", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".qualitygate.yaml", `
custom-rules:
  - id: test
    type: ban-import
    packages: ["moment"]
    message: "no"
`)
		if !p.Detect(context.Background(), dir) {
			t.Error("expected Detect=true with custom rules")
		}
	})
}

func TestProvider_Analyze_Mixed(t *testing.T) {
	dir := t.TempDir()

	// Create source files
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	writeFile(t, srcDir, "app.ts", `import moment from 'moment'
const d = new Date()
console.log(d)
`)

	writeFile(t, srcDir, "utils.ts", `import { format } from 'date-fns'
export const x = 1
`)

	writeFile(t, dir, "package.json", `{
  "dependencies": {
    "moment": "^2.0.0",
    "react": "^18.0.0"
  }
}`)

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "no-moment", Type: "ban-import", Packages: []string{"moment", "date-fns"}, Message: "banned lib", Severity: "blocker"},
		{ID: "no-raw-date", Type: "ban-pattern", Pattern: `new Date\(`, Message: "Use createLocalDate()", Severity: "major"},
		{ID: "no-moment-dep", Type: "ban-dependency", Packages: []string{"moment"}, Message: "Remove moment", Severity: "blocker"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.StatusFailed {
		t.Errorf("expected status failed, got %s", result.Status)
	}

	// Should have: 1 moment import in app.ts + 1 date-fns import in utils.ts + 1 new Date in app.ts + 1 moment in package.json = 4
	if len(result.Issues) < 3 {
		t.Errorf("expected at least 3 issues, got %d: %+v", len(result.Issues), result.Issues)
	}

	// Verify sources
	for _, issue := range result.Issues {
		if issue.Source != "custom-rules" {
			t.Errorf("expected source 'custom-rules', got %q", issue.Source)
		}
	}
}

func TestProvider_Analyze_GlobMissIsVisible(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1\n")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "no-todo", Type: "ban-pattern", Pattern: "TODO", Message: "No TODOs", Files: "src/**/*.py"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusPassed {
		t.Fatalf("expected passed (glob miss is not a violation), got %s", result.Status)
	}

	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "no-todo") && strings.Contains(d, "matched 0 files") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a glob-miss detail naming the rule, got %#v", result.Details)
	}
}

func TestProvider_Analyze_UnreadableFileIsVisible(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not block reads")
	}

	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1\n")
	writeFile(t, srcDir, "locked.ts", "const y = 2\n")
	if err := os.Chmod(filepath.Join(srcDir, "locked.ts"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(srcDir, "locked.ts"), 0644) })

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "no-todo", Type: "ban-pattern", Pattern: "TODO", Message: "No TODOs", Files: "src/**/*.ts"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "could not read") && strings.Contains(d, "src/locked.ts") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a read-error detail naming the file, got %#v", result.Details)
	}
}

func TestProvider_Analyze_NoRules(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CustomRules = nil

	p := New()
	result, err := p.Analyze(context.Background(), t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusSkipped {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestProvider_Analyze_InvalidRule(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "bad", Type: "ban-pattern", Pattern: "[invalid", Message: "m"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusError {
		t.Errorf("expected error status, got %s", result.Status)
	}
}

func TestProvider_Analyze_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "test", Type: "enforce-pattern", Pattern: "neverexists", Message: "m", Files: "src/**/*.ts"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := New()
	result, err := p.Analyze(ctx, dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusError {
		t.Errorf("expected error (cancelled), got %s", result.Status)
	}
}

// ─── Ignore Tests ───────────────────────────────────────────────────────────

func TestCompileRules_IgnoreTestsDefault(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "ban-p", Type: "ban-pattern", Pattern: "foo", Message: "m"},
		{ID: "ban-i", Type: "ban-import", Packages: []string{"x"}, Message: "m"},
		{ID: "enf-p", Type: "enforce-pattern", Pattern: "bar", Message: "m"},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// ban-pattern and ban-import should have IgnoreTests=true by default
	if !compiled[0].IgnoreTests {
		t.Error("ban-pattern should default to IgnoreTests=true")
	}
	if !compiled[1].IgnoreTests {
		t.Error("ban-import should default to IgnoreTests=true")
	}
	// enforce-pattern should NOT have IgnoreTests=true
	if compiled[2].IgnoreTests {
		t.Error("enforce-pattern should default to IgnoreTests=false")
	}

	// ban-pattern allow-in should include test globs
	hasTestGlob := false
	for _, g := range compiled[0].AllowInGlobs {
		if g == "**/*.test.ts" {
			hasTestGlob = true
			break
		}
	}
	if !hasTestGlob {
		t.Errorf("ban-pattern should have test globs in AllowInGlobs, got: %v", compiled[0].AllowInGlobs)
	}
}

func TestCompileRules_IgnoreTestsExplicitFalse(t *testing.T) {
	f := false
	rules := []config.CustomRule{
		{ID: "no-console", Type: "ban-pattern", Pattern: "console", Message: "m", IgnoreTests: &f},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if compiled[0].IgnoreTests {
		t.Error("expected IgnoreTests=false when explicitly set")
	}
	// Should NOT have test globs
	for _, g := range compiled[0].AllowInGlobs {
		if g == "**/*.test.ts" {
			t.Error("should not have test globs when IgnoreTests=false")
		}
	}
}

func TestMatchBanImport_IgnoreTestsSkipsTestFile(t *testing.T) {
	rule := compileRule(t, config.CustomRule{
		ID:       "no-moment",
		Type:     "ban-import",
		Packages: []string{"moment"},
		Message:  "banned",
	})

	lines := []string{"import moment from 'moment'"}
	// Test file should be skipped
	issues := matchBanImport(rule, "src/utils/date.test.ts", lines)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for test file, got %d", len(issues))
	}
	// Non-test file should match
	issues = matchBanImport(rule, "src/app.ts", lines)
	if len(issues) != 1 {
		t.Errorf("expected 1 issue for non-test file, got %d", len(issues))
	}
}

// ─── Allow Subpaths ─────────────────────────────────────────────────────────

func TestMatchBanImport_AllowSubpaths(t *testing.T) {
	rule := compileRule(t, config.CustomRule{
		ID:            "no-date-fns",
		Type:          "ban-import",
		Packages:      []string{"date-fns"},
		Message:       "banned",
		AllowSubpaths: []string{"locale"},
	})

	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"base import banned", `import { format } from 'date-fns'`, 1},
		{"locale subpath allowed", `import { ptBR } from 'date-fns/locale/pt-BR'`, 0},
		{"locale direct allowed", `import { ptBR } from 'date-fns/locale'`, 0},
		{"other subpath banned", `import { format } from 'date-fns/format'`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.content, "\n")
			issues := matchBanImport(rule, "src/app.ts", lines)
			if len(issues) != tt.want {
				t.Errorf("expected %d issues, got %d", tt.want, len(issues))
			}
		})
	}
}

func TestIsAllowedSubpath(t *testing.T) {
	tests := []struct {
		line          string
		pkg           string
		allowSubpaths []string
		want          bool
	}{
		{"import { ptBR } from 'date-fns/locale'", "date-fns", []string{"locale"}, true},
		{"import { ptBR } from 'date-fns/locale/pt-BR'", "date-fns", []string{"locale"}, true},
		{"import { format } from 'date-fns/format'", "date-fns", []string{"locale"}, false},
		{"import { format } from 'date-fns'", "date-fns", []string{"locale"}, false},
		{"import x from 'date-fns'", "date-fns", nil, false},
		{"import x from 'date-fns'", "date-fns", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := isAllowedSubpath(tt.line, tt.pkg, tt.allowSubpaths)
			if got != tt.want {
				t.Errorf("isAllowedSubpath(%q, %q, %v) = %v, want %v", tt.line, tt.pkg, tt.allowSubpaths, got, tt.want)
			}
		})
	}
}

// ─── Advisory Mode ──────────────────────────────────────────────────────────

func TestCompileRules_AdvisoryMode(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "adv", Type: "ban-pattern", Pattern: "TODO", Message: "m", Mode: "advisory"},
		{ID: "block", Type: "ban-pattern", Pattern: "FIXME", Message: "m"},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !compiled[0].Advisory {
		t.Error("expected advisory=true for mode=advisory")
	}
	if compiled[1].Advisory {
		t.Error("expected advisory=false for default mode")
	}
}

func TestProvider_Analyze_AdvisoryDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "console.log('hello')\nconst x = 1")

	f := false
	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "adv-console", Type: "ban-pattern", Pattern: `console\.log`, Message: "Use logger", Severity: "blocker", Mode: "advisory", IgnoreTests: &f, IgnoreComments: &f},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have issues but status should be passed (advisory doesn't block)
	if len(result.Issues) == 0 {
		t.Error("expected issues from advisory rule")
	}
	if result.Status != domain.StatusPassed {
		t.Errorf("expected passed status (advisory only), got %s", result.Status)
	}
}

func TestProvider_Analyze_MixedAdvisoryAndBlocking(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "console.log('hello')\nnew Date()")

	f := false
	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "adv-console", Type: "ban-pattern", Pattern: `console\.log`, Message: "Use logger", Severity: "blocker", Mode: "advisory", IgnoreTests: &f, IgnoreComments: &f},
		{ID: "no-date", Type: "ban-pattern", Pattern: `new Date\(`, Message: "Use util", Severity: "major", IgnoreTests: &f, IgnoreComments: &f},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(result.Issues))
	}
	// no-date is blocking + major → StatusWarning
	if result.Status != domain.StatusWarning {
		t.Errorf("expected warning (blocking major issue), got %s", result.Status)
	}
}

// ─── Max Lines ─────────────────────────────────────────────────────────────

func TestCompileRules_MaxLines(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "component-size", Type: "max-lines", MaxLines: 300, Message: "Split large components", Severity: "major"},
	}

	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if compiled[0].Type != RuleTypeMaxLines {
		t.Fatalf("expected type max-lines, got %s", compiled[0].Type)
	}
	if compiled[0].MaxLines != 300 {
		t.Fatalf("expected maxLines 300, got %d", compiled[0].MaxLines)
	}
}

func TestCompileRules_MaxLinesRequiresPositiveLimit(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "component-size", Type: "max-lines", MaxLines: 0, Message: "Split large components"},
	}

	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "requires positive 'max-lines'") {
		t.Fatalf("expected max-lines validation error, got %v", errs)
	}
}

func TestMatchMaxLines(t *testing.T) {
	rule := CompiledRule{
		Raw:      config.CustomRule{ID: "component-size", Message: "Split large components"},
		Type:     RuleTypeMaxLines,
		MaxLines: 3,
		Severity: domain.SeverityMajor,
	}

	issues := matchMaxLines(rule, "src/components/Huge.tsx", []string{"1", "2", "3", "4"})
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "found: 4 lines, max: 3") {
		t.Fatalf("expected detailed max-lines message, got %q", issues[0].Message)
	}
}

func TestProvider_Analyze_MaxLines(t *testing.T) {
	dir := t.TempDir()
	componentsDir := filepath.Join(dir, "src", "components")
	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, componentsDir, "Huge.tsx", "line1\nline2\nline3\nline4\n")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "component-size", Type: "max-lines", MaxLines: 3, Files: "src/components/**/*.tsx", Message: "Components should stay under 3 lines", Severity: "major"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Status != domain.StatusWarning {
		t.Fatalf("expected warning status, got %s", result.Status)
	}
}

// ─── Semgrep Rule Type ──────────────────────────────────────────────────────

func TestCompileRules_Semgrep(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "no-eval", Type: "semgrep", Pattern: "eval(...)", Message: "Do not use eval", Severity: "blocker"},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if compiled[0].Type != RuleTypeSemgrep {
		t.Errorf("expected type semgrep, got %s", compiled[0].Type)
	}
	if compiled[0].Language != "ts" {
		t.Errorf("expected default language 'ts', got %q", compiled[0].Language)
	}
}

func TestCompileRules_SemgrepCustomLanguage(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "no-eval-py", Type: "semgrep", Pattern: "eval(...)", Message: "No eval", Language: "python"},
	}
	compiled, errs, _ := CompileRules(rules)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if compiled[0].Language != "python" {
		t.Errorf("expected language 'python', got %q", compiled[0].Language)
	}
}

func TestCompileRules_SemgrepRequiresPattern(t *testing.T) {
	rules := []config.CustomRule{
		{ID: "bad", Type: "semgrep", Message: "m"},
	}
	_, errs, _ := CompileRules(rules)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "semgrep requires 'pattern'") {
		t.Fatalf("expected semgrep pattern error, got: %v", errs)
	}
}

// TestMatchSemgrep_NotInstalled was the single-rule path test; the single-rule
// path was deleted (plan 006). Its intent — 0 issues when semgrep is not
// installed — is covered by TestMatchSemgrepBatch_NotInstalled below, which
// also asserts the skip detail.

func TestHasAdvancedSemgrepOptions(t *testing.T) {
	tests := []struct {
		name string
		rule config.CustomRule
		want bool
	}{
		{"simple pattern", config.CustomRule{Pattern: "eval(...)"}, false},
		{"with pattern-not", config.CustomRule{Pattern: "eval(...)", PatternNot: "eval('safe')"}, true},
		{"with pattern-inside", config.CustomRule{Pattern: "$X / 100", PatternInside: "function $F(...) { ... }"}, true},
		{"with pattern-not-inside", config.CustomRule{Pattern: "$X", PatternNotInside: "function safe(...) { ... }"}, true},
		{"with metavariable-regex", config.CustomRule{Pattern: "$X / 100", MetavariableRegex: map[string]string{"$X": ".*Cents.*"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAdvancedSemgrepOptions(tt.rule)
			if got != tt.want {
				t.Errorf("hasAdvancedSemgrepOptions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildSemgrepBatchConfig_AdvancedRule(t *testing.T) {
	rule := CompiledRule{
		Raw: config.CustomRule{
			ID:                "no-cents-div",
			Pattern:           "$X / 100",
			PatternNotInside:  "function centsToReais(...) { ... }",
			MetavariableRegex: map[string]string{"$X": ".*[Cc]ents.*"},
			Message:           "Use centsToReais()",
		},
		Language: "ts",
	}

	path, err := buildSemgrepBatchConfig([]CompiledRule{rule})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	// Verify it contains the key semgrep YAML structure
	if !strings.Contains(content, "no-cents-div") {
		t.Error("config file should contain rule ID")
	}
	if !strings.Contains(content, "$X / 100") {
		t.Error("config file should contain pattern")
	}
	if !strings.Contains(content, "pattern-not-inside") {
		t.Error("config file should contain pattern-not-inside")
	}
	if !strings.Contains(content, "metavariable-regex") {
		t.Error("config file should contain metavariable-regex")
	}
	if !strings.Contains(content, "centsToReais") {
		t.Error("config file should contain the pattern-not-inside value")
	}
}

func TestBuildSemgrepBatchConfig_AllOptions(t *testing.T) {
	rule := CompiledRule{
		Raw: config.CustomRule{
			ID:               "complex-rule",
			Pattern:          "$X.auth.getUser()",
			PatternNot:       "mock.auth.getUser()",
			PatternInside:    "export async function $METHOD(...) { ... }",
			PatternNotInside: "function extractAuthenticatedTenant(...) { ... }",
			MetavariableRegex: map[string]string{
				"$X": "supabase|client",
			},
			Message: "Use extractAuthenticatedTenant()",
		},
		Language: "ts",
	}

	path, err := buildSemgrepBatchConfig([]CompiledRule{rule})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	for _, expected := range []string{"pattern-not:", "pattern-inside:", "pattern-not-inside:", "metavariable-regex:"} {
		if !strings.Contains(content, expected) {
			t.Errorf("config file should contain %q, got:\n%s", expected, content)
		}
	}
}

// ─── Semgrep Batch ──────────────────────────────────────────────────────────

func TestMatchSemgrepBatch_NotInstalled(t *testing.T) {
	// Force semgrep unavailable: strip it from PATH and disable auto-install so
	// the test never attempts a real pip install.
	t.Setenv("PATH", "/nonexistent-bin")
	t.Setenv("CRIVO_NO_AUTO_INSTALL", "1")

	rules := []CompiledRule{
		{
			Raw:      config.CustomRule{ID: "no-eval", Pattern: "eval(...)", Message: "No eval"},
			Type:     RuleTypeSemgrep,
			Severity: domain.SeverityBlocker,
			Language: "ts",
		},
		{
			Raw:      config.CustomRule{ID: "no-exec", Pattern: "exec(...)", Message: "No exec"},
			Type:     RuleTypeSemgrep,
			Severity: domain.SeverityBlocker,
			Language: "ts",
		},
	}

	issues, details := matchSemgrepBatch(context.Background(), rules, t.TempDir(), nil, nil)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues when semgrep not installed, got %d", len(issues))
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 detail listing skipped rules, got %d: %#v", len(details), details)
	}
	if !strings.Contains(details[0], "semgrep unavailable") || !strings.Contains(details[0], "2 rules skipped") {
		t.Errorf("expected unavailable detail with rule count, got %q", details[0])
	}
	for _, id := range []string{"no-eval", "no-exec"} {
		if !strings.Contains(details[0], id) {
			t.Errorf("expected skipped detail to list rule %q, got %q", id, details[0])
		}
	}
}

func TestBuildSemgrepBatchConfig(t *testing.T) {
	rules := []CompiledRule{
		{
			Raw:      config.CustomRule{ID: "no-eval", Pattern: "eval(...)", Message: "Do not use eval"},
			Language: "ts",
		},
		{
			Raw:      config.CustomRule{ID: "no-exec", Pattern: "exec(...)", Message: "Do not use exec"},
			Language: "ts",
		},
		{
			Raw:      config.CustomRule{ID: "no-system", Pattern: "system(...)", Message: "Do not use system"},
			Language: "python",
		},
	}

	path, err := buildSemgrepBatchConfig(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)

	// Verify all three rule IDs are present
	for _, id := range []string{"no-eval", "no-exec", "no-system"} {
		if !strings.Contains(content, id) {
			t.Errorf("config file should contain rule ID %q", id)
		}
	}

	// Verify all patterns are present
	for _, pat := range []string{"eval(...)", "exec(...)", "system(...)"} {
		if !strings.Contains(content, pat) {
			t.Errorf("config file should contain pattern %q", pat)
		}
	}

	// Verify languages
	if !strings.Contains(content, "python") {
		t.Error("config file should contain language 'python'")
	}

	// Verify it's valid YAML with rules key
	if !strings.Contains(content, "rules:") {
		t.Error("config file should have a 'rules:' key")
	}

	// Simple rules should use 'pattern' directly, not 'patterns'
	if strings.Contains(content, "patterns:") {
		t.Error("simple rules should use 'pattern' not 'patterns'")
	}
}

func TestBuildSemgrepBatchConfig_MixedSimpleAndAdvanced(t *testing.T) {
	rules := []CompiledRule{
		{
			Raw:      config.CustomRule{ID: "simple-rule", Pattern: "eval(...)", Message: "No eval"},
			Language: "ts",
		},
		{
			Raw: config.CustomRule{
				ID:               "advanced-rule",
				Pattern:          "$X / 100",
				PatternNotInside: "function centsToReais(...) { ... }",
				MetavariableRegex: map[string]string{
					"$X": ".*[Cc]ents.*",
				},
				Message: "Use centsToReais()",
			},
			Language: "ts",
		},
	}

	path, err := buildSemgrepBatchConfig(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)

	// Both rule IDs should be present
	if !strings.Contains(content, "simple-rule") {
		t.Error("config file should contain 'simple-rule'")
	}
	if !strings.Contains(content, "advanced-rule") {
		t.Error("config file should contain 'advanced-rule'")
	}

	// Advanced rule should have patterns list with pattern-not-inside and metavariable-regex
	if !strings.Contains(content, "pattern-not-inside") {
		t.Error("config file should contain 'pattern-not-inside' for advanced rule")
	}
	if !strings.Contains(content, "metavariable-regex") {
		t.Error("config file should contain 'metavariable-regex' for advanced rule")
	}
	if !strings.Contains(content, "centsToReais") {
		t.Error("config file should contain the pattern-not-inside value")
	}

	// The advanced rule should use 'patterns' (plural) key
	if !strings.Contains(content, "patterns:") {
		t.Error("advanced rule should use 'patterns' key")
	}
}

// ─── Semgrep unavailable (must run before stub tests cache a positive lookup) ─
//
// check.FindTool caches positive lookups for the process lifetime and Go runs
// tests in source order — so "unavailable" tests must be defined BEFORE the
// stub tests below, or the cached stub path would make semgrep look available.

func TestProvider_Analyze_SemgrepUnavailableWarns(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1\n")

	// Strip semgrep from PATH and disable auto-install so this never attempts
	// a real pip install.
	t.Setenv("PATH", "/nonexistent-bin")
	t.Setenv("CRIVO_NO_AUTO_INSTALL", "1")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "no-eval", Type: "semgrep", Pattern: "eval(...)", Message: "Do not use eval", Severity: "blocker"},
		{ID: "no-exec", Type: "semgrep", Pattern: "exec(...)", Message: "Do not use exec"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusWarning {
		t.Fatalf("expected warning when semgrep rules cannot run, got %s (summary %q)", result.Status, result.Summary)
	}
	if !strings.Contains(result.Summary, "SKIPPED") {
		t.Errorf("expected SKIPPED in summary, got %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "2 semgrep rules SKIPPED") {
		t.Errorf("expected 2 skipped semgrep rules in summary, got %q", result.Summary)
	}
	var hasEval, hasExec bool
	for _, d := range result.Details {
		if strings.Contains(d, "no-eval") {
			hasEval = true
		}
		if strings.Contains(d, "no-exec") {
			hasExec = true
		}
	}
	if !hasEval || !hasExec {
		t.Errorf("expected details to list both rule IDs, got %#v", result.Details)
	}
}

// ─── Semgrep stub end-to-end ─────────────────────────────────────────────────
//
// check.FindTool caches positive lookups for the process lifetime, so the stub
// binary is created once per package (sync.Once) and shared by all stub tests.
// Its behavior is selected by CRIVO_SEMGREP_STUB_MODE (inherited by the
// subprocess through os.Environ):
//   - "fail"    → exit 3 with a stderr message
//   - "empty"   → valid JSON with zero results
//   - (default) → valid JSON with one finding for rule "no-eval" (path
//     resolved relative to the subprocess cwd, which matchSemgrepBatch sets to
//     the project dir) plus an unknown check_id that must be ignored

var (
	semgrepStubOnce sync.Once
	semgrepStubDir  string
)

func semgrepStubBin(t *testing.T) {
	t.Helper()
	semgrepStubOnce.Do(func() {
		dir, err := os.MkdirTemp("", "crivo-customrules-semgrep-stub-*")
		if err != nil {
			t.Fatalf("create stub dir: %v", err)
		}
		semgrepStubDir = dir
		bin := filepath.Join(dir, "semgrep")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		script := `#!/bin/sh
if [ "$CRIVO_SEMGREP_STUB_MODE" = "fail" ]; then
  echo "semgrep: internal error: rule no-eval crashed" >&2
  exit 3
fi
if [ "$CRIVO_SEMGREP_STUB_MODE" = "empty" ]; then
  echo '{"results": [], "errors": []}'
  exit 0
fi
cat <<EOF
{"results": [{"check_id": "no-eval", "path": "$PWD/src/app.ts", "start": {"line": 3, "col": 5}, "extra": {"lines": "eval(x)"}}, {"check_id": "unknown.rule", "path": "$PWD/src/app.ts", "start": {"line": 9, "col": 1}, "extra": {"lines": "x"}}], "errors": []}
EOF
`
		if runtime.GOOS == "windows" {
			script = `@echo off
if "%CRIVO_SEMGREP_STUB_MODE%"=="fail" (
  echo semgrep: internal error: rule no-eval crashed 1>&2
  exit /b 3
)
if "%CRIVO_SEMGREP_STUB_MODE%"=="empty" (
  echo {"results": [], "errors": []}
  exit /b 0
)
echo {"results": [{"check_id": "no-eval", "path": "%CD%\src\app.ts", "start": {"line": 3, "col": 5}, "extra": {"lines": "eval(x)"}}, {"check_id": "unknown.rule", "path": "%CD%\src\app.ts", "start": {"line": 9, "col": 1}, "extra": {"lines": "x"}}], "errors": []}
`
		}
		if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
			t.Fatalf("write stub: %v", err)
		}
	})
	t.Setenv("PATH", semgrepStubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// semgrepRule is a minimal compiled semgrep rule fixture.
func semgrepRule(id, pattern, message string) CompiledRule {
	return CompiledRule{
		Raw:      config.CustomRule{ID: id, Pattern: pattern, Message: message, Files: "src/**/*.ts"},
		Type:     RuleTypeSemgrep,
		Severity: domain.SeverityBlocker,
		Language: "ts",
	}
}

func TestMatchSemgrepBatch_StubEndToEnd(t *testing.T) {
	projectDir := t.TempDir()
	srcDir := filepath.Join(projectDir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = eval(y)\n")

	semgrepStubBin(t)

	rules := []CompiledRule{semgrepRule("no-eval", "eval(...)", "Do not use eval")}

	issues, details := matchSemgrepBatch(context.Background(), rules, projectDir, nil, nil)
	if len(details) != 0 {
		t.Fatalf("expected no error details, got %#v", details)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (unknown check_id must be ignored), got %d: %#v", len(issues), issues)
	}
	iss := issues[0]
	if iss.RuleID != "no-eval" {
		t.Errorf("expected rule ID no-eval from check_id mapping, got %q", iss.RuleID)
	}
	if iss.File != "src/app.ts" {
		t.Errorf("expected relative path src/app.ts, got %q", iss.File)
	}
	if iss.Line != 3 || iss.Column != 5 {
		t.Errorf("expected line 3 col 5, got line %d col %d", iss.Line, iss.Column)
	}
	if iss.Remediation != domain.CustomRuleRemediation("semgrep", "Do not use eval") {
		t.Errorf("expected semgrep remediation on batch finding, got %q", iss.Remediation)
	}
}

func TestMatchSemgrepBatch_StubAdvisoryRuleGetsAdvisoryRemediation(t *testing.T) {
	projectDir := t.TempDir()
	srcDir := filepath.Join(projectDir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = eval(y)\n")

	semgrepStubBin(t)

	rule := semgrepRule("no-eval", "eval(...)", "Do not use eval")
	rule.Advisory = true

	issues, details := matchSemgrepBatch(context.Background(), []CompiledRule{rule}, projectDir, nil, nil)
	if len(details) != 0 {
		t.Fatalf("expected no error details, got %#v", details)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %#v", len(issues), issues)
	}
	if got := issues[0].Remediation; got != domain.CustomRuleRemediation("advisory", "Do not use eval") {
		t.Errorf("expected advisory remediation for an advisory rule, got %q", got)
	}
}

func TestMatchSemgrepBatch_StubAllowInFiltersFinding(t *testing.T) {
	projectDir := t.TempDir()
	srcDir := filepath.Join(projectDir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = eval(y)\n")

	semgrepStubBin(t)

	rule := semgrepRule("no-eval", "eval(...)", "Do not use eval")
	rule.AllowInGlobs = []string{"src/app.ts"}

	issues, details := matchSemgrepBatch(context.Background(), []CompiledRule{rule}, projectDir, nil, nil)
	if len(details) != 0 {
		t.Fatalf("expected no error details, got %#v", details)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues when the finding path is allow-listed, got %d: %#v", len(issues), issues)
	}
}

func TestMatchSemgrepBatch_StubSubprocessError(t *testing.T) {
	projectDir := t.TempDir()
	srcDir := filepath.Join(projectDir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = eval(y)\n")

	semgrepStubBin(t)
	t.Setenv("CRIVO_SEMGREP_STUB_MODE", "fail")

	rules := []CompiledRule{semgrepRule("no-eval", "eval(...)", "Do not use eval")}

	issues, details := matchSemgrepBatch(context.Background(), rules, projectDir, nil, nil)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues from a failing stub, got %d", len(issues))
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 error detail, got %d: %#v", len(details), details)
	}
	if !strings.Contains(details[0], "semgrep failed for rules no-eval") {
		t.Errorf("expected failure detail naming the rule, got %q", details[0])
	}
	if !strings.Contains(details[0], "internal error") {
		t.Errorf("expected stderr snippet in the detail, got %q", details[0])
	}
}

func TestProvider_Analyze_SemgrepFindingsWithStub(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = eval(y)\n")

	semgrepStubBin(t)

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "no-eval", Type: "semgrep", Pattern: "eval(...)", Message: "Do not use eval", Severity: "blocker", Files: "src/**/*.ts"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusFailed {
		t.Fatalf("expected failed for a blocker semgrep finding, got %s (summary %q)", result.Status, result.Summary)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %#v", len(result.Issues), result.Issues)
	}
	iss := result.Issues[0]
	if iss.RuleID != "no-eval" || iss.File != "src/app.ts" || iss.Line != 3 {
		t.Errorf("unexpected issue mapping: %+v", iss)
	}
	if iss.Remediation != domain.CustomRuleRemediation("semgrep", "Do not use eval") {
		t.Errorf("expected semgrep remediation on batch finding, got %q", iss.Remediation)
	}
	// No skip/failure details: the semgrep rules actually ran.
	for _, d := range result.Details {
		if strings.HasPrefix(d, "semgrep unavailable") || strings.HasPrefix(d, "semgrep failed") {
			t.Errorf("unexpected semgrep failure detail on a clean stub run: %q", d)
		}
	}
}

func TestProvider_Analyze_SemgrepRunsCleanWithStub(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeFile(t, srcDir, "app.ts", "const x = 1\n")

	semgrepStubBin(t)
	t.Setenv("CRIVO_SEMGREP_STUB_MODE", "empty")

	cfg := config.DefaultConfig()
	cfg.CustomRules = []config.CustomRule{
		{ID: "no-eval", Type: "semgrep", Pattern: "eval(...)", Message: "Do not use eval", Severity: "blocker", Files: "src/**/*.ts"},
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusPassed {
		t.Fatalf("expected passed after a real (stubbed) scan with no findings, got %s (summary %q)", result.Status, result.Summary)
	}
	if !strings.Contains(result.Summary, "1 rules checked · no violations") {
		t.Errorf("expected clean summary, got %q", result.Summary)
	}
	if len(result.Details) != 0 {
		t.Errorf("expected no details on a clean run, got %#v", result.Details)
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// BenchmarkMatchBanImport measures the cost of matching a ban-import rule
// against a 200-line fixture with 4 banned packages. Before the regex
// precompilation (plan 004), each call recompiled 2 regexes per package
// (8 MustCompile per file); after, the regexes are compiled once per rule.
func BenchmarkMatchBanImport(b *testing.B) {
	rule := compileRule(b, config.CustomRule{
		ID:       "no-banned",
		Type:     "ban-import",
		Packages: []string{"moment", "dayjs", "lodash", "axios"},
		Message:  "banned",
	})

	lines := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("import { helper%d } from 'react'", i))
	}
	// A few real violations so the matcher does real work
	lines[10] = "import moment from 'moment'"
	lines[50] = "const m = require('dayjs')"
	lines[100] = "import { get } from 'lodash'"
	lines[150] = "import axios from 'axios'"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		issues := matchBanImport(rule, "src/app.ts", lines)
		if len(issues) != 4 {
			b.Fatalf("expected 4 issues, got %d", len(issues))
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// compileRule compiles a single raw rule via CompileRules, failing the test on
// any compile error. Fixtures must go through CompileRules so the pre-compiled
// regex fields (ImportRes, MustImportRe) are populated.
func compileRule(tb testing.TB, raw config.CustomRule) CompiledRule {
	tb.Helper()
	compiled, errs, _ := CompileRules([]config.CustomRule{raw})
	if len(errs) > 0 {
		tb.Fatalf("compileRule(%q): unexpected errors: %v", raw.ID, errs)
	}
	return compiled[0]
}

func boolPtr(v bool) *bool { return &v }

func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
