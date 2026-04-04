package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
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
	path, err := check.EnsureTool("gitleaks")
	return err == nil && path != ""
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, _ *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	// On Windows, /dev/stdout doesn't exist — use a temp file instead
	var reportPath string
	var tmpFile *os.File
	if runtime.GOOS == "windows" {
		var err error
		tmpFile, err = os.CreateTemp("", "gitleaks-*.json")
		if err != nil {
			return &domain.CheckResult{
				Name:     p.Name(),
				ID:       p.ID(),
				Status:   domain.StatusError,
				Summary:  "Failed to create temp file for gitleaks",
				Details:  []string{err.Error()},
				Duration: time.Since(start),
			}, nil
		}
		tmpFile.Close()
		reportPath = tmpFile.Name()
		defer os.Remove(reportPath)
	} else {
		reportPath = "/dev/stdout"
	}

	gitleaksBin, err := check.EnsureTool("gitleaks")
	if err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusSkipped,
			Summary:  fmt.Sprintf("gitleaks not available: %v", err),
			Duration: time.Since(start),
			Details:  []string{"Install manually: https://github.com/gitleaks/gitleaks#installing"},
		}, nil
	}

	cmd := exec.CommandContext(ctx, gitleaksBin, "detect",
		"--source="+projectDir,
		"--report-format=json",
		"--report-path="+reportPath,
		"--no-git",
		"--no-banner",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)

	var output []byte
	if runtime.GOOS == "windows" {
		output, _ = os.ReadFile(reportPath)
	} else {
		output = stdout.Bytes()
	}

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
			RuleID:      "secret/" + r.RuleID,
			Message:     r.Description + ": " + maskedMatch,
			File:        relPath,
			Line:        r.StartLine,
			Column:      r.StartColumn,
			Severity:    domain.SeverityBlocker,
			Type:        domain.IssueTypeVulnerability,
			Source:      "gitleaks",
			Effort:      "15min",
			Remediation: domain.SecretRemediation("secret/" + r.RuleID),
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
