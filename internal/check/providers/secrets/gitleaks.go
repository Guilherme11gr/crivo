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
	"strings"
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

		// Downgrade severity for test/mock/fixture files
		isTestFile := isTestOrMockFile(relPath)
		severity := domain.SeverityBlocker
		issueType := domain.IssueTypeVulnerability
		remediation := domain.SecretRemediation("secret/" + r.RuleID)
		if isTestFile {
			severity = domain.SeverityInfo
			issueType = domain.IssueTypeCodeSmell
			remediation = "Hardcoded secret in test file. Replace with environment variables, test fixtures, or mock services. Add this file to .gitleaksignore if the secret is intentionally fake."
		}

		issues = append(issues, domain.Issue{
			RuleID:      "secret/" + r.RuleID,
			Message:     r.Description + ": " + maskedMatch,
			File:        relPath,
			Line:        r.StartLine,
			Column:      r.StartColumn,
			Severity:    severity,
			Type:        issueType,
			Source:      "gitleaks",
			Effort:      "15min",
			Remediation: remediation,
		})
	}

	count := len(issues)
	// Count only non-info secrets for policy blocking (test file secrets are downgraded to info)
	realSecretCount := 0
	for _, iss := range issues {
		if iss.Severity != domain.SeverityInfo {
			realSecretCount++
		}
	}

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
			"secrets": float64(realSecretCount),
		},
	}, nil
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// isTestOrMockFile checks if a file path looks like a test, mock, or fixture file.
// These are common patterns for non-production code where hardcoded secrets are expected.
func isTestOrMockFile(path string) bool {
	lower := strings.ToLower(path)

	// Go test files: *_test.go
	if strings.HasSuffix(lower, "_test.go") {
		return true
	}

	// JS/TS test files: .test.ts, .spec.ts, .test.tsx, .spec.tsx, .test.js, .spec.js
	testExts := []string{
		".test.ts", ".spec.ts", ".test.tsx", ".spec.tsx",
		".test.js", ".spec.js", ".test.mjs", ".spec.mjs",
		".test.cjs", ".spec.cjs",
	}
	for _, ext := range testExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}

	// File patterns: *.mock.*, *.fixture.*, *.stub.*, *.stories.*, *.story.*
	mockPatterns := []string{".mock.", ".fixture.", ".stub.", ".stories.", ".story."}
	for _, pat := range mockPatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}

	// Directory patterns: __tests__/, __mocks__/
	dirPatterns := []string{"__tests__", "__mocks__"}
	for _, pat := range dirPatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}

	return false
}
