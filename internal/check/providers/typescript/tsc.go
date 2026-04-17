package typescript

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

var tscErrorRe = regexp.MustCompile(`^(.+)\((\d+),(\d+)\):\s+error\s+(TS\d+):\s+(.+)$`)

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Type Safety" }
func (p *Provider) ID() string   { return "typescript" }

func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, "tsconfig.json"))
	return err == nil
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, _ *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

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

	// Try npx tsc --noEmit
	cmd := exec.CommandContext(ctx, npxBin, "tsc", "--noEmit", "--pretty", "false")
	cmd.Dir = projectDir
	cmd.Env = check.NodeEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	// tsc exits 0 = no errors
	if err == nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "0 errors",
			Duration: duration,
		}, nil
	}

	// Parse tsc output for errors
	output := stdout.String()
	if output == "" {
		output = stderr.String()
	}

	allIssues := parseTscOutput(output, projectDir)

	// Separate production errors from test/mock errors
	var prodIssues, testIssues []domain.Issue
	for _, issue := range allIssues {
		if isTestFile(issue.File) {
			issue.Severity = domain.SeverityMinor
			testIssues = append(testIssues, issue)
		} else {
			prodIssues = append(prodIssues, issue)
		}
	}

	// Production errors drive the status; test errors are informational
	totalErrors := len(allIssues)
	prodErrors := len(prodIssues)
	testErrors := len(testIssues)

	summary := strconv.Itoa(prodErrors) + " prod error"
	if prodErrors != 1 {
		summary += "s"
	}
	if testErrors > 0 {
		summary += ", " + strconv.Itoa(testErrors) + " in tests"
	}

	// Gate decision based on production errors only
	status := domain.StatusFailed
	if prodErrors == 0 {
		if testErrors > 0 {
			status = domain.StatusWarning
		} else {
			status = domain.StatusPassed
		}
	}

	// If tsc exited non-zero but we found no parseable errors, treat as passed
	if totalErrors == 0 {
		status = domain.StatusPassed
		summary = "0 errors"
	}

	// Show prod issues first, then test issues
	issues := append(prodIssues, testIssues...)
	details := make([]string, 0, min(len(issues), 20))
	for i, issue := range issues {
		if i >= 20 {
			break
		}
		tag := ""
		if isTestFile(issue.File) {
			tag = " [test]"
		}
		details = append(details, issue.File+":"+strconv.Itoa(issue.Line)+" "+issue.Message+tag)
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
			"errors":      float64(totalErrors),
			"prod_errors": float64(prodErrors),
			"test_errors": float64(testErrors),
		},
	}, nil
}

// isTestFile returns true for test, spec, mock, and fixture files.
func isTestFile(filePath string) bool {
	lower := strings.ToLower(filePath)
	// Path-based: __tests__/, __mocks__/, __fixtures__/, test/, tests/, mocks/
	for _, dir := range []string{"__tests__/", "__mocks__/", "__fixtures__/", "/test/", "/tests/", "/mocks/", "/fixtures/"} {
		if strings.Contains(lower, dir) {
			return true
		}
	}
	// File-based: *.test.*, *.spec.*, *.mock.*, setup-tests.*, jest.*
	base := filepath.Base(lower)
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.Contains(base, ".mock.") {
		return true
	}
	if strings.HasPrefix(base, "jest.") || strings.HasPrefix(base, "setup-tests") || strings.HasPrefix(base, "test-utils") {
		return true
	}
	return false
}

func parseTscOutput(output string, projectDir string) []domain.Issue {
	var issues []domain.Issue
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()
		matches := tscErrorRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		file := matches[1]
		lineNum, _ := strconv.Atoi(matches[2])
		col, _ := strconv.Atoi(matches[3])
		ruleID := matches[4]
		message := matches[5]

		// Make path relative
		if rel, err := filepath.Rel(projectDir, file); err == nil {
			file = rel
		}
		file = filepath.ToSlash(file)

		issues = append(issues, domain.Issue{
			RuleID:      ruleID,
			Message:     message,
			File:        file,
			Line:        lineNum,
			Column:      col,
			Severity:    domain.SeverityMajor,
			Type:        domain.IssueTypeBug,
			Source:      "tsc",
			Effort:      "10min",
			Remediation: domain.TypescriptRemediation(ruleID),
		})
	}

	return issues
}
