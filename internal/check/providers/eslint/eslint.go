package eslint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anthropics/quality-gate/internal/check"
	"github.com/anthropics/quality-gate/internal/config"
	"github.com/anthropics/quality-gate/internal/domain"
)

// eslintResult matches ESLint's JSON output format
type eslintResult struct {
	FilePath string         `json:"filePath"`
	Messages []eslintMessage `json:"messages"`
}

type eslintMessage struct {
	RuleID   *string `json:"ruleId"`
	Severity int     `json:"severity"` // 1=warning, 2=error
	Message  string  `json:"message"`
	Line     int     `json:"line"`
	Column   int     `json:"column"`
}

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Code Quality" }
func (p *Provider) ID() string   { return "eslint" }

func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	// Check for ESLint config files or eslint in package.json
	configs := []string{
		".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml",
		".eslintrc.yaml", ".eslintrc", "eslint.config.js", "eslint.config.mjs",
		"eslint.config.cjs", "eslint.config.ts",
	}

	for _, c := range configs {
		if _, err := os.Stat(filepath.Join(projectDir, c)); err == nil {
			return true
		}
	}

	// Check package.json for eslintConfig
	pkgPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}

	var pkg map[string]json.RawMessage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	_, hasConfig := pkg["eslintConfig"]
	return hasConfig
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	// Build ESLint command with JSON output
	args := []string{"eslint", "--format", "json", "--no-error-on-unmatched-pattern"}

	// Add source directories
	if len(cfg.Src) > 0 {
		args = append(args, cfg.Src...)
	} else {
		args = append(args, "src/")
	}

	npxBin := check.FindNpx()
	if npxBin == "" {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusSkipped,
			Summary:  "npx not found",
			Duration: time.Since(start),
		}, nil
	}

	cmd := exec.CommandContext(ctx, npxBin, args...)
	cmd.Dir = projectDir
	cmd.Env = check.NodeEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// ESLint exits non-zero when it finds issues — that's expected
	_ = cmd.Run()
	duration := time.Since(start)

	output := stdout.Bytes()
	if len(output) == 0 {
		// No JSON output — ESLint might have crashed
		errMsg := stderr.String()
		if errMsg != "" {
			return &domain.CheckResult{
				Name:     p.Name(),
				ID:       p.ID(),
				Status:   domain.StatusError,
				Summary:  "ESLint error",
				Details:  []string{truncate(errMsg, 500)},
				Duration: duration,
			}, nil
		}

		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "Clean",
			Duration: duration,
		}, nil
	}

	var results []eslintResult
	if err := json.Unmarshal(output, &results); err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusError,
			Summary:  "Failed to parse ESLint output",
			Details:  []string{err.Error()},
			Duration: duration,
		}, nil
	}

	// Convert to domain issues
	var issues []domain.Issue
	errors := 0
	warnings := 0

	for _, result := range results {
		relPath := result.FilePath
		if rel, err := filepath.Rel(projectDir, result.FilePath); err == nil {
			relPath = rel
		}
		relPath = filepath.ToSlash(relPath)

		for _, msg := range result.Messages {
			// Skip "rule not found" noise
			if msg.RuleID == nil {
				continue
			}

			severity := domain.SeverityMinor
			issueType := domain.IssueTypeCodeSmell
			effort := "5min"

			if msg.Severity == 2 {
				errors++
				severity = domain.SeverityMajor
				effort = "10min"

				// Classify sonarjs bug-detection rules
				ruleID := *msg.RuleID
				if isBugRule(ruleID) {
					issueType = domain.IssueTypeBug
					severity = domain.SeverityCritical
					effort = "15min"
				} else if isSecurityRule(ruleID) {
					issueType = domain.IssueTypeVulnerability
					severity = domain.SeverityCritical
					effort = "30min"
				}
			} else {
				warnings++
			}

			issues = append(issues, domain.Issue{
				RuleID:   *msg.RuleID,
				Message:  msg.Message,
				File:     relPath,
				Line:     msg.Line,
				Column:   msg.Column,
				Severity: severity,
				Type:     issueType,
				Source:   "eslint",
				Effort:   effort,
			})
		}
	}

	// Determine status
	status := domain.StatusPassed
	if errors > 0 {
		status = domain.StatusFailed
	} else if warnings > 0 {
		status = domain.StatusWarning
	}

	// Build summary
	parts := []string{}
	if errors > 0 {
		parts = append(parts, strconv.Itoa(errors)+" error")
		if errors != 1 {
			parts[len(parts)-1] += "s"
		}
	}
	if warnings > 0 {
		s := strconv.Itoa(warnings) + " warning"
		if warnings != 1 {
			s += "s"
		}
		parts = append(parts, s)
	}

	summary := "Clean"
	if len(parts) > 0 {
		summary = join(parts, " · ")
	}

	// Build details (top 20 errors first)
	details := make([]string, 0, min(len(issues), 20))
	for i, issue := range issues {
		if i >= 20 {
			break
		}
		details = append(details, issue.File+":"+strconv.Itoa(issue.Line)+" "+issue.Message+" ("+issue.RuleID+")")
	}

	return &domain.CheckResult{
		Name:     p.Name(),
		ID:       p.ID(),
		Status:   status,
		Summary:  summary,
		Issues:   issues,
		Details:  details,
		Duration: duration,
		Metrics: map[string]float64{
			"errors":   float64(errors),
			"warnings": float64(warnings),
		},
	}, nil
}

func isBugRule(ruleID string) bool {
	bugRules := map[string]bool{
		"sonarjs/no-all-duplicated-branches": true,
		"sonarjs/no-element-overwrite":       true,
		"sonarjs/no-identical-conditions":    true,
		"sonarjs/no-identical-expressions":   true,
		"sonarjs/no-one-iteration-loop":      true,
		"sonarjs/no-use-of-empty-return-value": true,
		"sonarjs/no-unused-collection":       true,
	}
	return bugRules[ruleID]
}

func isSecurityRule(ruleID string) bool {
	secRules := map[string]bool{
		"security/detect-eval-with-expression":            true,
		"security/detect-child-process":                   true,
		"security/detect-no-csrf-before-method-override":  true,
		"security/detect-non-literal-fs-filename":         true,
		"security/detect-non-literal-require":             true,
		"security/detect-possible-timing-attacks":         true,
	}
	return secRules[ruleID]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func join(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
