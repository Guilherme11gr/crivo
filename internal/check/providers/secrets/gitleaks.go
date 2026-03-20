package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anthropics/quality-gate/internal/config"
	"github.com/anthropics/quality-gate/internal/domain"
)

type gitleaksResult struct {
	Description string `json:"Description"`
	StartLine   int    `json:"StartLine"`
	EndLine     int    `json:"EndLine"`
	StartColumn int    `json:"StartColumn"`
	EndColumn   int    `json:"EndColumn"`
	File        string `json:"File"`
	Entropy     float64 `json:"Entropy"`
	RuleID      string `json:"RuleID"`
	Fingerprint string `json:"Fingerprint"`
	Match       string `json:"Match"`
}

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Secrets" }
func (p *Provider) ID() string   { return "secrets" }

func (p *Provider) Detect(_ context.Context, _ string) bool {
	_, err := exec.LookPath("gitleaks")
	return err == nil
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, _ *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	cmd := exec.CommandContext(ctx, "gitleaks", "detect",
		"--source="+projectDir,
		"--report-format=json",
		"--report-path=/dev/stdout",
		"--no-git",
		"--quiet",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)

	output := stdout.Bytes()

	// Exit 0 = no leaks, exit 1 = leaks found
	if runErr == nil && len(output) == 0 {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "0 secrets detected",
			Duration: duration,
		}, nil
	}

	var results []gitleaksResult
	if len(output) > 0 {
		if err := json.Unmarshal(output, &results); err != nil {
			// Try if it's empty array
			if string(output) == "[]" || string(output) == "[]\n" {
				return &domain.CheckResult{
					Name:     p.Name(),
					ID:       p.ID(),
					Status:   domain.StatusPassed,
					Summary:  "0 secrets detected",
					Duration: duration,
				}, nil
			}
			return &domain.CheckResult{
				Name:     p.Name(),
				ID:       p.ID(),
				Status:   domain.StatusError,
				Summary:  "Failed to parse gitleaks output",
				Details:  []string{err.Error()},
				Duration: duration,
			}, nil
		}
	}

	if len(results) == 0 {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "0 secrets detected",
			Duration: duration,
		}, nil
	}

	var issues []domain.Issue
	for _, r := range results {
		relPath := r.File
		if rel, err := filepath.Rel(projectDir, r.File); err == nil {
			relPath = rel
		}
		relPath = filepath.ToSlash(relPath)

		// Mask the match for safety
		maskedMatch := maskSecret(r.Match)

		issues = append(issues, domain.Issue{
			RuleID:   "secret/" + r.RuleID,
			Message:  r.Description + ": " + maskedMatch,
			File:     relPath,
			Line:     r.StartLine,
			Column:   r.StartColumn,
			Severity: domain.SeverityBlocker,
			Type:     domain.IssueTypeVulnerability,
			Source:   "gitleaks",
			Effort:   "15min",
		})
	}

	count := len(issues)
	summary := strconv.Itoa(count) + " secret"
	if count != 1 {
		summary += "s"
	}
	summary += " detected"

	details := make([]string, 0, min(count, 20))
	for i, issue := range issues {
		if i >= 20 {
			break
		}
		details = append(details, issue.File+":"+strconv.Itoa(issue.Line)+" "+issue.Message)
	}

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  domain.StatusFailed,
		Summary: summary,
		Issues:  issues,
		Details: details,
		Duration: duration,
		Metrics: map[string]float64{
			"secrets": float64(count),
		},
	}, nil
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
