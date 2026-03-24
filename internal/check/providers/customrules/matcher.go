package customrules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
	"gopkg.in/yaml.v3"
)

// isCommentLine returns true if the trimmed line is a single-line comment
// or inside a block comment continuation (lines starting with * or /*).
func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "*/") ||
		trimmed == "*"
}

// isAllowedSubpath checks if an import line uses an allowed subpath.
// e.g., if package is "date-fns" and allow-subpaths is ["locale"],
// then "import { ptBR } from 'date-fns/locale/pt-BR'" is allowed.
func isAllowedSubpath(line string, pkg string, allowSubpaths []string) bool {
	if len(allowSubpaths) == 0 {
		return false
	}
	for _, sub := range allowSubpaths {
		// Check for pkg/sub in the import line
		needle := pkg + "/" + sub
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

// matchBanImport checks for banned package imports in file content.
// Matches ES import and CommonJS require, including sub-paths.
func matchBanImport(rule CompiledRule, filePath string, lines []string) []domain.Issue {
	// Check allow-in: if file matches, skip entirely
	if len(rule.AllowInGlobs) > 0 && IsAllowedIn(filePath, rule.AllowInGlobs) {
		return nil
	}

	var issues []domain.Issue

	for _, pkg := range rule.Raw.Packages {
		// Match: import ... from 'pkg' or import ... from 'pkg/sub'
		// Match: require('pkg') or require('pkg/sub')
		// Word boundary: don't match 'safe-pkg' when banning 'pkg'
		escaped := regexp.QuoteMeta(pkg)
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`(?:import\s+.*from\s+|import\s+)['"]` + escaped + `(?:/[^'"]*)?['"]`),
			regexp.MustCompile(`require\s*\(\s*['"]` + escaped + `(?:/[^'"]*)?['"]\s*\)`),
		}

		for lineNum, line := range lines {
			if rule.IgnoreComments && isCommentLine(line) {
				continue
			}
			// Check if this is an allowed subpath
			if isAllowedSubpath(line, pkg, rule.AllowSubpaths) {
				continue
			}
			for _, re := range patterns {
				loc := re.FindStringIndex(line)
				if loc != nil {
					issues = append(issues, domain.Issue{
						RuleID:   rule.Raw.ID,
						Message:  rule.Raw.Message,
						File:     filePath,
						Line:     lineNum + 1,
						Column:   loc[0] + 1,
						Severity: rule.Severity,
						Type:     domain.IssueTypeCodeSmell,
						Source:   "custom-rules",
						Effort:   "10min",
					})
				}
			}
		}
	}

	return issues
}

// matchBanPattern checks for banned regex patterns line by line.
func matchBanPattern(rule CompiledRule, filePath string, lines []string) []domain.Issue {
	// Check allow-in: if file matches, skip entirely
	if len(rule.AllowInGlobs) > 0 && IsAllowedIn(filePath, rule.AllowInGlobs) {
		return nil
	}

	var issues []domain.Issue

	for lineNum, line := range lines {
		if rule.IgnoreComments && isCommentLine(line) {
			continue
		}
		loc := rule.PatternRe.FindStringIndex(line)
		if loc != nil {
			issues = append(issues, domain.Issue{
				RuleID:   rule.Raw.ID,
				Message:  rule.Raw.Message,
				File:     filePath,
				Line:     lineNum + 1,
				Column:   loc[0] + 1,
				Severity: rule.Severity,
				Type:     domain.IssueTypeCodeSmell,
				Source:   "custom-rules",
				Effort:   "10min",
			})
		}
	}

	return issues
}

// matchRequireImport checks that when a file uses certain patterns,
// it imports them from the required source.
func matchRequireImport(rule CompiledRule, filePath string, content string) []domain.Issue {
	// If when-pattern is set, check if the file matches it
	if rule.WhenPatternRe != nil {
		if !rule.WhenPatternRe.MatchString(content) {
			return nil // file doesn't use the pattern, skip
		}
	}

	// Check if the required import exists
	escaped := regexp.QuoteMeta(rule.Raw.MustImportFrom)
	importRe := regexp.MustCompile(`(?:import\s+.*from\s+|import\s+|require\s*\(\s*)['"]` + escaped + `(?:/[^'"]*)?['"]`)

	if importRe.MatchString(content) {
		return nil // import found, all good
	}

	return []domain.Issue{
		{
			RuleID:   rule.Raw.ID,
			Message:  rule.Raw.Message,
			File:     filePath,
			Line:     1,
			Severity: rule.Severity,
			Type:     domain.IssueTypeCodeSmell,
			Source:   "custom-rules",
			Effort:   "10min",
		},
	}
}

// matchEnforcePattern checks that a file contains a required pattern.
func matchEnforcePattern(rule CompiledRule, filePath string, content string) []domain.Issue {
	if rule.PatternRe.MatchString(content) {
		return nil // pattern found
	}

	return []domain.Issue{
		{
			RuleID:   rule.Raw.ID,
			Message:  rule.Raw.Message,
			File:     filePath,
			Line:     1,
			Severity: rule.Severity,
			Type:     domain.IssueTypeCodeSmell,
			Source:   "custom-rules",
			Effort:   "15min",
		},
	}
}

// matchMaxLines checks that a file does not exceed the configured line limit.
func matchMaxLines(rule CompiledRule, filePath string, lines []string) []domain.Issue {
	if len(rule.AllowInGlobs) > 0 && IsAllowedIn(filePath, rule.AllowInGlobs) {
		return nil
	}

	if len(lines) <= rule.MaxLines {
		return nil
	}

	return []domain.Issue{
		{
			RuleID:   rule.Raw.ID,
			Message:  fmt.Sprintf("%s (found: %d lines, max: %d)", rule.Raw.Message, len(lines), rule.MaxLines),
			File:     filePath,
			Line:     1,
			Severity: rule.Severity,
			Type:     domain.IssueTypeCodeSmell,
			Source:   "custom-rules",
			Effort:   "20min",
		},
	}
}

// packageJSON is a minimal structure for reading package.json dependencies
type packageJSON struct {
	Dependencies     map[string]string `json:"dependencies"`
	DevDependencies  map[string]string `json:"devDependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

// matchBanDependency checks for banned packages in package.json.
func matchBanDependency(rule CompiledRule, projectDir string) []domain.Issue {
	pkgPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil // no package.json, skip silently
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	// Read raw lines to find line numbers
	lines := strings.Split(string(data), "\n")

	var issues []domain.Issue

	allDeps := map[string]string{}
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.PeerDependencies {
		allDeps[k] = v
	}

	for _, banned := range rule.Raw.Packages {
		if _, found := allDeps[banned]; found {
			// Find the line number
			line := findLineInJSON(lines, banned)
			issues = append(issues, domain.Issue{
				RuleID:   rule.Raw.ID,
				Message:  fmt.Sprintf("%s (found: %s@%s)", rule.Raw.Message, banned, allDeps[banned]),
				File:     "package.json",
				Line:     line,
				Severity: rule.Severity,
				Type:     domain.IssueTypeCodeSmell,
				Source:   "custom-rules",
				Effort:   "15min",
			})
		}
	}

	return issues
}

// findLineInJSON finds the line number of a key in JSON lines
func findLineInJSON(lines []string, key string) int {
	needle := fmt.Sprintf(`"%s"`, key)
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	return 1
}

// semgrepJSON matches the relevant parts of Semgrep's JSON output
type semgrepJSON struct {
	Results []semgrepResultJSON `json:"results"`
}

type semgrepResultJSON struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Start   struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"start"`
	Extra struct {
		Lines string `json:"lines"`
	} `json:"extra"`
}

// semgrepAvailable caches the result of checking for semgrep binary
var semgrepAvailable *bool

// isSemgrepAvailable checks if semgrep is installed
func isSemgrepAvailable() bool {
	if semgrepAvailable != nil {
		return *semgrepAvailable
	}
	_, err := exec.LookPath("semgrep")
	avail := err == nil
	semgrepAvailable = &avail
	return avail
}

// hasAdvancedSemgrepOptions returns true if the rule uses pattern-not, pattern-inside, etc.
func hasAdvancedSemgrepOptions(raw config.CustomRule) bool {
	return raw.PatternNot != "" || raw.PatternInside != "" ||
		raw.PatternNotInside != "" || len(raw.MetavariableRegex) > 0
}

// buildSemgrepConfigFile generates a temporary semgrep YAML rule config for complex patterns.
// Returns the path to the temp file (caller must clean up) or an error.
func buildSemgrepConfigFile(rule CompiledRule) (string, error) {
	// Build the patterns list for semgrep YAML
	var patterns []map[string]any
	patterns = append(patterns, map[string]any{"pattern": rule.Raw.Pattern})

	if rule.Raw.PatternNot != "" {
		patterns = append(patterns, map[string]any{"pattern-not": rule.Raw.PatternNot})
	}
	if rule.Raw.PatternInside != "" {
		patterns = append(patterns, map[string]any{"pattern-inside": rule.Raw.PatternInside})
	}
	if rule.Raw.PatternNotInside != "" {
		patterns = append(patterns, map[string]any{"pattern-not-inside": rule.Raw.PatternNotInside})
	}
	for varName, regex := range rule.Raw.MetavariableRegex {
		patterns = append(patterns, map[string]any{
			"metavariable-regex": map[string]string{
				"metavariable": varName,
				"regex":        regex,
			},
		})
	}

	ruleConfig := map[string]any{
		"rules": []map[string]any{
			{
				"id":        rule.Raw.ID,
				"patterns":  patterns,
				"message":   rule.Raw.Message,
				"languages": []string{rule.Language},
				"severity":  "WARNING",
			},
		},
	}

	data, err := yaml.Marshal(ruleConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal semgrep config: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "crivo-semgrep-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write semgrep config: %w", err)
	}
	tmpFile.Close()

	return tmpFile.Name(), nil
}

// matchSemgrep runs semgrep with the rule's pattern against files and returns issues.
// For simple patterns, uses --pattern flag. For complex rules (pattern-not, pattern-inside, etc.),
// generates a temp YAML config file and uses --config.
func matchSemgrep(ctx context.Context, rule CompiledRule, projectDir string, files []string) []domain.Issue {
	if !isSemgrepAvailable() {
		return nil
	}

	args := []string{
		"scan",
		"--json",
		"--quiet",
	}

	// Complex rules need a config file; simple rules use --pattern
	if hasAdvancedSemgrepOptions(rule.Raw) {
		configPath, err := buildSemgrepConfigFile(rule)
		if err != nil {
			return nil
		}
		defer os.Remove(configPath)
		args = append(args, "--config", configPath)
	} else {
		args = append(args, "--pattern", rule.Raw.Pattern, "--lang", rule.Language)
	}

	// Add file targets
	for _, f := range files {
		args = append(args, filepath.Join(projectDir, f))
	}

	cmd := exec.CommandContext(ctx, "semgrep", args...)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	output := stdout.Bytes()
	if len(output) == 0 {
		return nil
	}

	var result semgrepJSON
	if err := json.Unmarshal(output, &result); err != nil {
		return nil
	}

	var issues []domain.Issue
	for _, r := range result.Results {
		relPath := r.Path
		if rel, err := filepath.Rel(projectDir, r.Path); err == nil {
			relPath = rel
		}
		relPath = filepath.ToSlash(relPath)

		// Check allow-in
		if len(rule.AllowInGlobs) > 0 && IsAllowedIn(relPath, rule.AllowInGlobs) {
			continue
		}

		issues = append(issues, domain.Issue{
			RuleID:   rule.Raw.ID,
			Message:  rule.Raw.Message,
			File:     relPath,
			Line:     r.Start.Line,
			Column:   r.Start.Col,
			Severity: rule.Severity,
			Type:     domain.IssueTypeCodeSmell,
			Source:   "custom-rules",
			Effort:   "15min",
		})
	}

	return issues
}

// buildSemgrepBatchConfig generates a temporary semgrep YAML config file containing multiple rules.
// Returns the path to the temp file (caller must clean up) or an error.
func buildSemgrepBatchConfig(rules []CompiledRule) (string, error) {
	var rulesList []map[string]any

	for _, rule := range rules {
		var ruleEntry map[string]any

		if hasAdvancedSemgrepOptions(rule.Raw) {
			// Advanced rule: use patterns list
			var patterns []map[string]any
			patterns = append(patterns, map[string]any{"pattern": rule.Raw.Pattern})

			if rule.Raw.PatternNot != "" {
				patterns = append(patterns, map[string]any{"pattern-not": rule.Raw.PatternNot})
			}
			if rule.Raw.PatternInside != "" {
				patterns = append(patterns, map[string]any{"pattern-inside": rule.Raw.PatternInside})
			}
			if rule.Raw.PatternNotInside != "" {
				patterns = append(patterns, map[string]any{"pattern-not-inside": rule.Raw.PatternNotInside})
			}
			for varName, regex := range rule.Raw.MetavariableRegex {
				patterns = append(patterns, map[string]any{
					"metavariable-regex": map[string]string{
						"metavariable": varName,
						"regex":        regex,
					},
				})
			}

			ruleEntry = map[string]any{
				"id":        rule.Raw.ID,
				"patterns":  patterns,
				"message":   rule.Raw.Message,
				"languages": []string{rule.Language},
				"severity":  "WARNING",
			}
		} else {
			// Simple rule: use pattern directly
			ruleEntry = map[string]any{
				"id":        rule.Raw.ID,
				"pattern":   rule.Raw.Pattern,
				"message":   rule.Raw.Message,
				"languages": []string{rule.Language},
				"severity":  "WARNING",
			}
		}

		rulesList = append(rulesList, ruleEntry)
	}

	batchConfig := map[string]any{
		"rules": rulesList,
	}

	data, err := yaml.Marshal(batchConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal semgrep batch config: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "crivo-semgrep-batch-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write semgrep batch config: %w", err)
	}
	tmpFile.Close()

	return tmpFile.Name(), nil
}

// matchSemgrepBatch runs semgrep once with multiple rules batched into a single config file.
// Rules are grouped by their file glob, and one semgrep invocation is made per glob group.
// Results are mapped back to the correct rule by check_id.
func matchSemgrepBatch(ctx context.Context, rules []CompiledRule, projectDir string, exclude []string) []domain.Issue {
	if !isSemgrepAvailable() || len(rules) == 0 {
		return nil
	}

	// Build a lookup of rules by ID for mapping results back
	ruleByID := map[string]CompiledRule{}
	for _, rule := range rules {
		ruleByID[rule.Raw.ID] = rule
	}

	// Group rules by their file glob
	globToRules := map[string][]CompiledRule{}
	for _, rule := range rules {
		glob := rule.Raw.Files
		if glob == "" {
			glob = defaultFileGlob
		}
		globToRules[glob] = append(globToRules[glob], rule)
	}

	var allIssues []domain.Issue

	for glob, groupRules := range globToRules {
		files, err := WalkFiles(ctx, projectDir, glob, exclude)
		if err != nil {
			if ctx.Err() != nil {
				return allIssues
			}
			continue
		}
		if len(files) == 0 {
			continue
		}

		// Build batch config for this glob group
		configPath, err := buildSemgrepBatchConfig(groupRules)
		if err != nil {
			continue
		}

		args := []string{
			"scan",
			"--json",
			"--quiet",
			"--config", configPath,
		}

		// Add file targets
		for _, f := range files {
			args = append(args, filepath.Join(projectDir, f))
		}

		cmd := exec.CommandContext(ctx, "semgrep", args...)
		cmd.Dir = projectDir

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		_ = cmd.Run()
		os.Remove(configPath)

		output := stdout.Bytes()
		if len(output) == 0 {
			continue
		}

		var result semgrepJSON
		if err := json.Unmarshal(output, &result); err != nil {
			continue
		}

		for _, r := range result.Results {
			rule, ok := ruleByID[r.CheckID]
			if !ok {
				continue
			}

			relPath := r.Path
			if rel, err := filepath.Rel(projectDir, r.Path); err == nil {
				relPath = rel
			}
			relPath = filepath.ToSlash(relPath)

			// Check allow-in
			if len(rule.AllowInGlobs) > 0 && IsAllowedIn(relPath, rule.AllowInGlobs) {
				continue
			}

			allIssues = append(allIssues, domain.Issue{
				RuleID:   rule.Raw.ID,
				Message:  rule.Raw.Message,
				File:     relPath,
				Line:     r.Start.Line,
				Column:   r.Start.Col,
				Severity: rule.Severity,
				Type:     domain.IssueTypeCodeSmell,
				Source:   "custom-rules",
				Effort:   "15min",
			})
		}
	}

	return allIssues
}
