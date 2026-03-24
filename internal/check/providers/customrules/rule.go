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
	AllowInGlobs   []string
	Severity       domain.Severity
	IgnoreComments bool     // skip comment lines in ban-pattern/ban-import
	IgnoreTests    bool     // auto-skip test files
	AllowSubpaths  []string // subpaths allowed even when package is banned (ban-import)
	Advisory       bool     // true = report but don't affect gate status
	MaxLines       int      // maximum allowed file lines (max-lines)
	Language       string   // semgrep language (e.g. "ts", "python", "go")
}

// CompileRules validates and compiles all custom rules, collecting all errors at once
func CompileRules(rules []config.CustomRule) ([]CompiledRule, []error) {
	var compiled []CompiledRule
	var errs []error

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

	return compiled, errs
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
