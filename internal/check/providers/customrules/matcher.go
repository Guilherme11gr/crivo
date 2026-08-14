package customrules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilherme11gr/crivo/internal/check"
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

	for i, pkg := range rule.Raw.Packages {
		// Regexes are pre-compiled at rule compile time (CompileRules):
		// one pair (ES import, require) per banned package.
		patterns := rule.ImportRes[i*2 : i*2+2]

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
						RuleID:      rule.Raw.ID,
						Message:     rule.Raw.Message,
						File:        filePath,
						Line:        lineNum + 1,
						Column:      loc[0] + 1,
						Severity:    rule.Severity,
						Type:        domain.IssueTypeCodeSmell,
						Advisory:    rule.Advisory,
						Source:      "custom-rules",
						Effort:      "10min",
						Remediation: domain.CustomRuleRemediation("ban-import", rule.Raw.Message),
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
				RuleID:      rule.Raw.ID,
				Message:     rule.Raw.Message,
				File:        filePath,
				Line:        lineNum + 1,
				Column:      loc[0] + 1,
				Severity:    rule.Severity,
				Type:        domain.IssueTypeCodeSmell,
				Advisory:    rule.Advisory,
				Source:      "custom-rules",
				Effort:      "10min",
				Remediation: domain.CustomRuleRemediation("ban-pattern", rule.Raw.Message),
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

	// Check if the required import exists (regex pre-compiled at rule compile time)
	if rule.MustImportRe.MatchString(content) {
		return nil // import found, all good
	}

	return []domain.Issue{
		{
			RuleID:      rule.Raw.ID,
			Message:     rule.Raw.Message,
			File:        filePath,
			Line:        1,
			Severity:    rule.Severity,
			Type:        domain.IssueTypeCodeSmell,
			Advisory:    rule.Advisory,
			Source:      "custom-rules",
			Effort:      "10min",
			Remediation: domain.CustomRuleRemediation("require-import", rule.Raw.Message),
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
			RuleID:      rule.Raw.ID,
			Message:     rule.Raw.Message,
			File:        filePath,
			Line:        1,
			Severity:    rule.Severity,
			Type:        domain.IssueTypeCodeSmell,
			Advisory:    rule.Advisory,
			Source:      "custom-rules",
			Effort:      "15min",
			Remediation: domain.CustomRuleRemediation("enforce-pattern", rule.Raw.Message),
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
			RuleID:      rule.Raw.ID,
			Message:     fmt.Sprintf("%s (found: %d lines, max: %d)", rule.Raw.Message, len(lines), rule.MaxLines),
			File:        filePath,
			Line:        1,
			Severity:    rule.Severity,
			Type:        domain.IssueTypeCodeSmell,
			Advisory:    rule.Advisory,
			Source:      "custom-rules",
			Effort:      "20min",
			Remediation: domain.CustomRuleRemediation("max-lines", rule.Raw.Message),
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
func matchBanDependency(rule CompiledRule, projectDir string, scopedFiles map[string]bool) []domain.Issue {
	if scopedFiles != nil && !scopedFiles["package.json"] {
		return nil
	}

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
				RuleID:      rule.Raw.ID,
				Message:     fmt.Sprintf("%s (found: %s@%s)", rule.Raw.Message, banned, allDeps[banned]),
				File:        "package.json",
				Line:        line,
				Severity:    rule.Severity,
				Type:        domain.IssueTypeCodeSmell,
				Advisory:    rule.Advisory,
				Source:      "custom-rules",
				Effort:      "15min",
				Remediation: domain.CustomRuleRemediation("ban-dependency", rule.Raw.Message),
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

// isSemgrepAvailable checks if semgrep is usable, attempting auto-install the
// same way the dedicated semgrep provider does (check.EnsureTool). The result
// is NOT cached for the process lifetime: a negative answer must not stick, or
// a later run with the tool installed would silently skip semgrep rules.
func isSemgrepAvailable() bool {
	_, err := check.EnsureTool("semgrep")
	return err == nil
}

// findSemgrepBin returns the semgrep binary path, trying auto-install if needed.
func findSemgrepBin() string {
	if p := check.FindTool("semgrep"); p != "" {
		return p
	}
	// Try auto-install
	if p, err := check.EnsureTool("semgrep"); err == nil {
		return p
	}
	return "semgrep" // fallback to PATH lookup
}

// ruleIDList joins rule IDs for a human-readable detail message.
func ruleIDList(rules []CompiledRule) string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.Raw.ID
	}
	return strings.Join(ids, ", ")
}

// truncateDetail bounds a subprocess stderr snippet to maxChars.
func truncateDetail(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "..."
}

// hasAdvancedSemgrepOptions returns true if the rule uses pattern-not, pattern-inside, etc.
func hasAdvancedSemgrepOptions(raw config.CustomRule) bool {
	return raw.PatternNot != "" || raw.PatternInside != "" ||
		raw.PatternNotInside != "" || len(raw.MetavariableRegex) > 0
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

// validateSemgrepFixture runs one semgrep rule against a single fixture file and
// reports whether the rule fired. It returns (fired, validated): validated=false
// when the binary is unavailable (mirrors the runtime skip, plan 002) or the
// subprocess failed — the caller must surface that as a warning, never an error.
func validateSemgrepFixture(rule CompiledRule, code string) (bool, bool) {
	if !isSemgrepAvailable() {
		return false, false
	}

	tmpFile, err := os.CreateTemp("", "crivo-semgrep-fixture-*")
	if err != nil {
		return false, false
	}
	path := tmpFile.Name()
	defer os.Remove(path)
	if _, err := tmpFile.WriteString(code); err != nil {
		tmpFile.Close()
		return false, false
	}
	tmpFile.Close()

	configPath, err := buildSemgrepBatchConfig([]CompiledRule{rule})
	if err != nil {
		return false, false
	}
	defer os.Remove(configPath)

	cmd := exec.Command(findSemgrepBin(), "scan", "--json", "--quiet", "--config", configPath, path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	output := stdout.Bytes()
	if runErr != nil && len(output) == 0 {
		return false, false
	}

	var result semgrepJSON
	if err := json.Unmarshal(output, &result); err != nil {
		return false, false
	}

	for _, r := range result.Results {
		if r.CheckID == rule.Raw.ID {
			return true, true
		}
	}
	return false, true
}

// matchSemgrepBatch runs semgrep once with multiple rules batched into a single config file.
// Rules are grouped by their file glob, and one semgrep invocation is made per glob group.
// Results are mapped back to the correct rule by check_id.
//
// It returns the issues found plus a list of human-readable details. A non-empty
// details slice means semgrep rules were NOT actually scanned for that group:
// either the binary is unavailable (rules skipped) or the subprocess failed —
// the caller must surface this and never report a clean pass.
func matchSemgrepBatch(ctx context.Context, rules []CompiledRule, projectDir string, exclude []string, scopedFiles []string) ([]domain.Issue, []string) {
	if len(rules) == 0 {
		return nil, nil
	}
	if !isSemgrepAvailable() {
		return nil, []string{fmt.Sprintf("semgrep unavailable — %d rules skipped: %s", len(rules), ruleIDList(rules))}
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
	var details []string

	for glob, groupRules := range globToRules {
		files, err := filesForGlob(ctx, projectDir, glob, exclude, scopedFiles)
		if err != nil {
			if ctx.Err() != nil {
				return allIssues, details
			}
			details = append(details, fmt.Sprintf("semgrep failed for rules %s: %s", ruleIDList(groupRules), truncateDetail(err.Error(), 200)))
			continue
		}
		if len(files) == 0 {
			continue
		}

		// Build batch config for this glob group
		configPath, err := buildSemgrepBatchConfig(groupRules)
		if err != nil {
			details = append(details, fmt.Sprintf("semgrep failed for rules %s: %s", ruleIDList(groupRules), truncateDetail(err.Error(), 200)))
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

		cmd := exec.CommandContext(ctx, findSemgrepBin(), args...)
		cmd.Dir = projectDir

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		os.Remove(configPath)

		output := stdout.Bytes()
		if runErr != nil && len(output) == 0 {
			// Subprocess failed without producing JSON: surface the error
			// instead of silently treating the group as clean.
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg == "" {
				errMsg = runErr.Error()
			}
			details = append(details, fmt.Sprintf("semgrep failed for rules %s: %s", ruleIDList(groupRules), truncateDetail(errMsg, 200)))
			continue
		}

		var result semgrepJSON
		if err := json.Unmarshal(output, &result); err != nil {
			details = append(details, fmt.Sprintf("semgrep failed for rules %s: invalid JSON output: %s", ruleIDList(groupRules), truncateDetail(err.Error(), 200)))
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

			// Derive the remediation from the rule mode: advisory rules get the
			// advisory remediation, blocking rules the semgrep one. The two
			// paths (single-rule vs batch) must never diverge here.
			remediationType := "semgrep"
			if rule.Advisory {
				remediationType = "advisory"
			}

			allIssues = append(allIssues, domain.Issue{
				RuleID:      rule.Raw.ID,
				Message:     rule.Raw.Message,
				File:        relPath,
				Line:        r.Start.Line,
				Column:      r.Start.Col,
				Severity:    rule.Severity,
				Type:        domain.IssueTypeCodeSmell,
				Advisory:    rule.Advisory,
				Source:      "custom-rules",
				Effort:      "15min",
				Remediation: domain.CustomRuleRemediation(remediationType, rule.Raw.Message),
			})
		}
	}

	return allIssues, details
}
