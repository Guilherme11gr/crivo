package customrules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

// RuleType identifies what kind of check a rule performs
type RuleType string

const (
	RuleTypeBanImport      RuleType = "ban-import"
	RuleTypeBanPattern     RuleType = "ban-pattern"
	RuleTypeRequireImport  RuleType = "require-import"
	RuleTypeEnforcePattern RuleType = "enforce-pattern"
	RuleTypeBanDependency  RuleType = "ban-dependency"
	RuleTypeMaxLines       RuleType = "max-lines"
	RuleTypeSemgrep        RuleType = "semgrep"
)

var validRuleTypes = map[RuleType]bool{
	RuleTypeBanImport:      true,
	RuleTypeBanPattern:     true,
	RuleTypeRequireImport:  true,
	RuleTypeEnforcePattern: true,
	RuleTypeBanDependency:  true,
	RuleTypeMaxLines:       true,
	RuleTypeSemgrep:        true,
}

// CompiledRule is a validated and pre-compiled custom rule ready for matching
type CompiledRule struct {
	Raw            config.CustomRule
	Type           RuleType
	PatternRe      *regexp.Regexp // ban-pattern, enforce-pattern
	WhenPatternRe  *regexp.Regexp // require-import
	ImportRes      []*regexp.Regexp // ban-import: one pair (ES import, require) per banned package
	MustImportRe   *regexp.Regexp // require-import: regex for the required import
	AllowInGlobs   []string
	Severity       domain.Severity
	IgnoreComments bool     // skip comment lines in ban-pattern/ban-import
	IgnoreTests    bool     // auto-skip test files
	AllowSubpaths  []string // subpaths allowed even when package is banned (ban-import)
	Advisory       bool     // true = report but don't affect gate status
	MaxLines       int      // maximum allowed file lines (max-lines)
	Language       string   // semgrep language (e.g. "ts", "python", "go")
}

// CompileRules validates and compiles all custom rules, collecting all errors at once.
// Per-rule test fixtures (raw.Tests) are validated after pre-compilation: each spec
// runs against the rule's matcher and a disagreement is a compile error naming the
// rule and the fixture case. Semgrep fixtures are validated only when the semgrep
// binary is available; without it the validation is collected as a warning, never an
// error — the rule still compiles and the runtime skip/warning path (plan 002) reports
// the unverified state.
func CompileRules(rules []config.CustomRule) ([]CompiledRule, []error, []string) {
	var compiled []CompiledRule
	var errs []error
	var warnings []string

	seenIDs := map[string]bool{}

	for i, raw := range rules {
		label := fmt.Sprintf("custom-rules[%d]", i)
		if raw.ID != "" {
			label = raw.ID
		}

		// Validate ID
		if raw.ID == "" {
			errs = append(errs, fmt.Errorf("rule %s: missing required field 'id'", label))
			continue
		}
		if seenIDs[raw.ID] {
			errs = append(errs, fmt.Errorf("rule %s: duplicate id", label))
			continue
		}
		seenIDs[raw.ID] = true

		// Validate type
		rt := RuleType(raw.Type)
		if !validRuleTypes[rt] {
			if raw.Type == "" {
				errs = append(errs, fmt.Errorf("rule %s: missing required field 'type'", label))
			} else {
				errs = append(errs, fmt.Errorf("rule %s: unknown type %q", label, raw.Type))
			}
			continue
		}

		// Default ignore-comments to true for ban-pattern and ban-import
		ignoreComments := (rt == RuleTypeBanPattern || rt == RuleTypeBanImport)
		if raw.IgnoreComments != nil {
			ignoreComments = *raw.IgnoreComments
		}

		// Default ignore-tests to true for ban-pattern and ban-import
		ignoreTests := (rt == RuleTypeBanPattern || rt == RuleTypeBanImport)
		if raw.IgnoreTests != nil {
			ignoreTests = *raw.IgnoreTests
		}

		// Build allow-in globs, appending test patterns if ignore-tests is true
		allowInGlobs := make([]string, len(raw.AllowIn))
		copy(allowInGlobs, raw.AllowIn)
		if ignoreTests {
			allowInGlobs = appendTestGlobs(allowInGlobs)
		}

		// Parse mode
		advisory := strings.EqualFold(raw.Mode, "advisory")

		cr := CompiledRule{
			Raw:            raw,
			Type:           rt,
			AllowInGlobs:   allowInGlobs,
			Severity:       parseSeverity(raw.Severity),
			IgnoreComments: ignoreComments,
			IgnoreTests:    ignoreTests,
			AllowSubpaths:  raw.AllowSubpaths,
			Advisory:       advisory,
		}

		// Validate required fields per type
		switch rt {
		case RuleTypeBanImport:
			if len(raw.Packages) == 0 {
				errs = append(errs, fmt.Errorf("rule %s: ban-import requires 'packages'", label))
				continue
			}
			// Pre-compile one pair of regexes (ES import, require) per banned
			// package so matching never recompiles them per file.
			cr.ImportRes = make([]*regexp.Regexp, 0, len(raw.Packages)*2)
			for _, pkg := range raw.Packages {
				escaped := regexp.QuoteMeta(pkg)
				cr.ImportRes = append(cr.ImportRes,
					regexp.MustCompile(`(?:import\s+.*from\s+|import\s+)['"]`+escaped+`(?:/[^'"]*)?['"]`),
					regexp.MustCompile(`require\s*\(\s*['"]`+escaped+`(?:/[^'"]*)?['"]\s*\)`),
				)
			}

		case RuleTypeBanPattern:
			if raw.Pattern == "" {
				errs = append(errs, fmt.Errorf("rule %s: ban-pattern requires 'pattern'", label))
				continue
			}
			re, err := regexp.Compile(raw.Pattern)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %s: invalid regex %q: %w", label, raw.Pattern, err))
				continue
			}
			cr.PatternRe = re

		case RuleTypeRequireImport:
			if raw.MustImportFrom == "" {
				errs = append(errs, fmt.Errorf("rule %s: require-import requires 'must-import-from'", label))
				continue
			}
			if raw.WhenPattern != "" {
				re, err := regexp.Compile(raw.WhenPattern)
				if err != nil {
					errs = append(errs, fmt.Errorf("rule %s: invalid when-pattern regex %q: %w", label, raw.WhenPattern, err))
					continue
				}
				cr.WhenPatternRe = re
			}
			// Pre-compile the required-import regex once per rule.
			escaped := regexp.QuoteMeta(raw.MustImportFrom)
			cr.MustImportRe = regexp.MustCompile(`(?:import\s+.*from\s+|import\s+|require\s*\(\s*)['"]` + escaped + `(?:/[^'"]*)?['"]`)

		case RuleTypeEnforcePattern:
			if raw.Pattern == "" {
				errs = append(errs, fmt.Errorf("rule %s: enforce-pattern requires 'pattern'", label))
				continue
			}
			re, err := regexp.Compile(raw.Pattern)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %s: invalid regex %q: %w", label, raw.Pattern, err))
				continue
			}
			cr.PatternRe = re

		case RuleTypeBanDependency:
			if len(raw.Packages) == 0 {
				errs = append(errs, fmt.Errorf("rule %s: ban-dependency requires 'packages'", label))
				continue
			}

		case RuleTypeMaxLines:
			if raw.MaxLines <= 0 {
				errs = append(errs, fmt.Errorf("rule %s: max-lines requires positive 'max-lines'", label))
				continue
			}
			cr.MaxLines = raw.MaxLines

		case RuleTypeSemgrep:
			if raw.Pattern == "" {
				errs = append(errs, fmt.Errorf("rule %s: semgrep requires 'pattern'", label))
				continue
			}
			lang := raw.Language
			if lang == "" {
				lang = "ts"
			}
			cr.Language = lang
		}

		// Validate message
		if raw.Message == "" {
			errs = append(errs, fmt.Errorf("rule %s: missing required field 'message'", label))
			continue
		}

		compiled = append(compiled, cr)
	}

	// Validate per-rule fixtures after pre-compilation: only fully compiled rules
	// with tests participate.
	if len(compiled) > 0 {
		errs = append(errs, validateFixtures(compiled)...)
		warnings = append(warnings, fixtureWarnings(compiled)...)
	}

	return compiled, errs, warnings
}

// fixtureFilePath is the synthetic file path used when validating fixtures in
// memory. The name must be a plausible TS file so allow-in globs and test-file
// detection behave exactly as they would on a real source file.
const fixtureFilePath = "src/fixture.ts"

// fixtureMatches runs the compiled rule's matcher against Code as a synthetic
// file and reports whether the rule fired. The second return is false when the
// fixture could NOT be validated: semgrep rules without a binary (mirrors the
// runtime skip, plan 002) and ban-dependency rules (they match package.json,
// not code). Unvalidated fixtures surface as warnings, never errors.
func fixtureMatches(rule CompiledRule, code string) (bool, bool) {
	switch rule.Type {
	case RuleTypeSemgrep:
		return validateSemgrepFixture(rule, code)
	case RuleTypeBanDependency:
		return false, false
	default:
		lines := strings.Split(code, "\n")
		var fired bool
		switch rule.Type {
		case RuleTypeBanImport:
			fired = len(matchBanImport(rule, fixtureFilePath, lines)) > 0
		case RuleTypeBanPattern:
			fired = len(matchBanPattern(rule, fixtureFilePath, lines)) > 0
		case RuleTypeRequireImport:
			fired = len(matchRequireImport(rule, fixtureFilePath, code)) > 0
		case RuleTypeEnforcePattern:
			fired = len(matchEnforcePattern(rule, fixtureFilePath, code)) > 0
		case RuleTypeMaxLines:
			fired = len(matchMaxLines(rule, fixtureFilePath, lines)) > 0
		}
		return fired, true
	}
}

// validateFixtures checks every test spec of every compiled rule against the
// rule's matcher. A spec that disagrees with the matcher is a compile error
// naming the rule and the case — a rule author must learn a fixture is wrong
// when the rule is compiled, not when it silently fires on real code.
func validateFixtures(compiled []CompiledRule) []error {
	var errs []error
	for _, rule := range compiled {
		for i, spec := range rule.Raw.Tests {
			if len(spec.Code) == 0 {
				errs = append(errs, fmt.Errorf("rule %s: fixture %d: empty 'code'", rule.Raw.ID, i+1))
				continue
			}
			got, validated := fixtureMatches(rule, spec.Code)
			if !validated || got == spec.Match {
				continue
			}
			if spec.Match {
				errs = append(errs, fmt.Errorf("rule %s: fixture %d failed: code %q — expected the rule to match, but it did not", rule.Raw.ID, i+1, spec.Code))
			} else {
				errs = append(errs, fmt.Errorf("rule %s: fixture %d failed: code %q — expected the rule to NOT match, but it did", rule.Raw.ID, i+1, spec.Code))
			}
		}
	}
	return errs
}

// fixtureWarnings collects non-fatal fixture validation notes: semgrep fixtures
// cannot run without the binary (the runtime already skips semgrep rules and
// surfaces that as a warning — plan 002), and ban-dependency fixtures operate on
// package.json, not on code.
func fixtureWarnings(compiled []CompiledRule) []string {
	var warnings []string
	for _, rule := range compiled {
		if len(rule.Raw.Tests) == 0 {
			continue
		}
		switch rule.Type {
		case RuleTypeSemgrep:
			if !isSemgrepAvailable() {
				warnings = append(warnings, fmt.Sprintf(
					"rule %s: %d fixture(s) NOT validated — semgrep binary unavailable (install semgrep to validate fixtures)",
					rule.Raw.ID, len(rule.Raw.Tests)))
			}
		case RuleTypeBanDependency:
			warnings = append(warnings, fmt.Sprintf(
				"rule %s: fixtures skipped — ban-dependency matches package.json, not code", rule.Raw.ID))
		}
	}
	return warnings
}

// testGlobPatterns are the default glob patterns for test files
var testGlobPatterns = []string{
	"**/*.test.ts",
	"**/*.test.tsx",
	"**/*.spec.ts",
	"**/*.spec.tsx",
	"**/__tests__/**",
}

// appendTestGlobs appends test glob patterns to the allow-in list, avoiding duplicates
func appendTestGlobs(globs []string) []string {
	existing := map[string]bool{}
	for _, g := range globs {
		existing[g] = true
	}
	for _, tg := range testGlobPatterns {
		if !existing[tg] {
			globs = append(globs, tg)
		}
	}
	return globs
}

func parseSeverity(s string) domain.Severity {
	switch strings.ToLower(s) {
	case "blocker":
		return domain.SeverityBlocker
	case "critical":
		return domain.SeverityCritical
	case "major", "":
		return domain.SeverityMajor
	case "minor":
		return domain.SeverityMinor
	case "info":
		return domain.SeverityInfo
	default:
		return domain.SeverityMajor
	}
}
