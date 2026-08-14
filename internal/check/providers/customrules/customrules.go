package customrules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

// Provider implements check.Provider for user-defined custom rules
type Provider struct{}

// New creates a new custom rules provider
func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string { return "Custom Rules" }
func (p *Provider) ID() string   { return "custom-rules" }

// Detect returns true if custom rules are configured
func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	cfg, _ := config.Load(projectDir)
	return len(cfg.CustomRules) > 0
}

// Analyze compiles rules, walks files, and applies matchers
func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	result := &domain.CheckResult{
		Name: p.Name(),
		ID:   p.ID(),
	}

	if len(cfg.CustomRules) == 0 {
		result.Status = domain.StatusSkipped
		result.Summary = "No custom rules configured"
		result.Duration = time.Since(start)
		return result, nil
	}

	var scopedFiles []string
	var scopedFileSet map[string]bool
	if scope, ok := check.NewCodeScopeFromContext(ctx); ok {
		scopedFiles = filterExistingFiles(projectDir, scope.ChangedFiles)
		scopedFileSet = make(map[string]bool, len(scopedFiles))
		for _, file := range scopedFiles {
			scopedFileSet[file] = true
		}
	}

	// Compile rules
	compiled, compileErrs := CompileRules(cfg.CustomRules)
	if len(compileErrs) > 0 {
		msgs := make([]string, len(compileErrs))
		for i, e := range compileErrs {
			msgs[i] = e.Error()
		}
		result.Status = domain.StatusError
		result.Summary = fmt.Sprintf("%d rule compilation errors", len(compileErrs))
		result.Details = msgs
		result.Duration = time.Since(start)
		return result, nil
	}

	var allIssues []domain.Issue
	var details []string

	// Separate rules by execution strategy
	var fileRules []CompiledRule
	var semgrepRules []CompiledRule
	for _, rule := range compiled {
		switch rule.Type {
		case RuleTypeBanDependency:
			issues := matchBanDependency(rule, projectDir, scopedFileSet)
			allIssues = append(allIssues, issues...)
		case RuleTypeSemgrep:
			semgrepRules = append(semgrepRules, rule)
		default:
			fileRules = append(fileRules, rule)
		}
	}

	// Run semgrep rules — batch all rules into minimal semgrep invocations (grouped by file glob)
	// Details are non-empty when semgrep is unavailable or a group failed to scan:
	// those rules must never be reported as "checked".
	if len(semgrepRules) > 0 {
		issues, semgrepDetails := matchSemgrepBatch(ctx, semgrepRules, projectDir, cfg.Exclude, scopedFiles)
		allIssues = append(allIssues, issues...)
		details = append(details, semgrepDetails...)
	}

	// Group file rules by their file glob to minimize walks
	if len(fileRules) > 0 {
		globToRules := map[string][]CompiledRule{}
		for _, rule := range fileRules {
			glob := rule.Raw.Files
			if glob == "" {
				glob = defaultFileGlob
			}
			globToRules[glob] = append(globToRules[glob], rule)
		}

		for glob, rules := range globToRules {
			files, err := filesForGlob(ctx, projectDir, glob, cfg.Exclude, scopedFiles)
			if err != nil {
				if ctx.Err() != nil {
					result.Status = domain.StatusError
					result.Summary = "Cancelled"
					result.Duration = time.Since(start)
					return result, nil
				}
				continue
			}

			for _, file := range files {
				// Check context
				select {
				case <-ctx.Done():
					result.Status = domain.StatusError
					result.Summary = "Cancelled"
					result.Duration = time.Since(start)
					return result, nil
				default:
				}

				absPath := filepath.Join(projectDir, file)
				data, err := os.ReadFile(absPath)
				if err != nil {
					continue
				}

				if !IsTextFile(data) {
					continue
				}

				content := string(data)
				lines := strings.Split(content, "\n")

				for _, rule := range rules {
					switch rule.Type {
					case RuleTypeBanImport:
						allIssues = append(allIssues, matchBanImport(rule, file, lines)...)
					case RuleTypeBanPattern:
						allIssues = append(allIssues, matchBanPattern(rule, file, lines)...)
					case RuleTypeRequireImport:
						allIssues = append(allIssues, matchRequireImport(rule, file, content)...)
					case RuleTypeEnforcePattern:
						allIssues = append(allIssues, matchEnforcePattern(rule, file, content)...)
					case RuleTypeMaxLines:
						allIssues = append(allIssues, matchMaxLines(rule, file, lines)...)
					}
				}
			}
		}
	}

	result.Issues = allIssues
	result.Duration = time.Since(start)
	result.Details = details
	advisoryViolations := 0
	blockingViolations := 0
	for _, issue := range allIssues {
		if issue.Advisory {
			advisoryViolations++
			continue
		}
		blockingViolations++
	}
	result.Metrics = map[string]float64{
		"rules":               float64(len(compiled)),
		"violations":          float64(len(allIssues)),
		"blocking_violations": float64(blockingViolations),
		"advisory_violations": float64(advisoryViolations),
	}

	// Build set of advisory rule IDs for status calculation
	advisoryRules := map[string]bool{}
	for _, rule := range compiled {
		if rule.Advisory {
			advisoryRules[rule.Raw.ID] = true
		}
	}

	// Determine status — advisory issues don't affect gate
	blockingIssues := 0
	hasBlocker := false
	for _, issue := range allIssues {
		if advisoryRules[issue.RuleID] {
			continue
		}
		blockingIssues++
		if issue.Severity == domain.SeverityBlocker || issue.Severity == domain.SeverityCritical {
			hasBlocker = true
		}
	}

	// Semgrep rules that could not be scanned (binary unavailable or the
	// subprocess failed) must never be reported as "checked". The skip/failure
	// details list rule IDs verbatim — count the non-advisory ones that were
	// actually skipped; they force a warning instead of a pass.
	pendingSemgrep := 0
	if len(details) > 0 {
		ids := map[string]bool{}
		for _, rule := range semgrepRules {
			ids[rule.Raw.ID] = true
		}
		skipped := map[string]bool{}
		for _, d := range details {
			if !strings.HasPrefix(d, "semgrep unavailable") && !strings.HasPrefix(d, "semgrep failed") {
				continue
			}
			rest := d
			if i := strings.Index(rest, ": "); i >= 0 {
				rest = rest[i+2:]
			}
			for _, tok := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' }) {
				tok = strings.Trim(tok, ":")
				if ids[tok] {
					skipped[tok] = true
				}
			}
		}
		for _, rule := range semgrepRules {
			if !rule.Advisory && skipped[rule.Raw.ID] {
				pendingSemgrep++
			}
		}
	}

	if blockingIssues == 0 && pendingSemgrep == 0 {
		result.Status = domain.StatusPassed
		if len(allIssues) == 0 {
			result.Summary = fmt.Sprintf("%d rules checked · no violations", len(compiled))
		} else {
			result.Summary = fmt.Sprintf("%d rules checked · %d advisory-only violations", len(compiled), len(allIssues))
		}
	} else if blockingIssues == 0 && pendingSemgrep > 0 {
		// A security control that reports success without having scanned is
		// worse than the absence of the control: surface the skip as a warning.
		result.Status = domain.StatusWarning
		result.Summary = fmt.Sprintf("%d rules checked · %d semgrep rules SKIPPED (semgrep unavailable)", len(compiled), pendingSemgrep)
	} else {
		if hasBlocker {
			result.Status = domain.StatusFailed
		} else {
			result.Status = domain.StatusWarning
		}
		result.Summary = fmt.Sprintf("%d violations across %d rules", len(allIssues), len(compiled))
	}

	// Build details
	ruleViolations := map[string]int{}
	for _, issue := range allIssues {
		ruleViolations[issue.RuleID]++
	}
	for _, rule := range compiled {
		count := ruleViolations[rule.Raw.ID]
		if count > 0 {
			result.Details = append(result.Details,
				fmt.Sprintf("  %s (%s): %d violations", rule.Raw.ID, rule.Type, count))
		}
	}

	return result, nil
}

func filterExistingFiles(projectDir string, files []string) []string {
	result := make([]string, 0, len(files))
	for _, file := range files {
		absPath := filepath.Join(projectDir, file)
		if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
			result = append(result, file)
		}
	}
	return result
}

func filesForGlob(ctx context.Context, projectDir, glob string, exclude []string, scopedFiles []string) ([]string, error) {
	if len(scopedFiles) == 0 {
		return WalkFiles(ctx, projectDir, glob, exclude)
	}

	files := make([]string, 0, len(scopedFiles))
	for _, file := range scopedFiles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if matchGlob(glob, file) {
			files = append(files, file)
		}
	}

	return files, nil
}
