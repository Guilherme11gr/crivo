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

	"github.com/anthropics/quality-gate/internal/config"
	"github.com/anthropics/quality-gate/internal/domain"
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

	// Try npx tsc --noEmit
	cmd := exec.CommandContext(ctx, "npx", "tsc", "--noEmit", "--pretty", "false")
	cmd.Dir = projectDir

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

	issues := parseTscOutput(output, projectDir)

	errorCount := len(issues)
	summary := strconv.Itoa(errorCount) + " error"
	if errorCount != 1 {
		summary += "s"
	}

	details := make([]string, 0, min(len(issues), 20))
	for i, issue := range issues {
		if i >= 20 {
			break
		}
		details = append(details, issue.File+":"+strconv.Itoa(issue.Line)+" "+issue.Message)
	}

	return &domain.CheckResult{
		Name:     p.Name(),
		ID:       p.ID(),
		Status:   domain.StatusFailed,
		Summary:  summary,
		Issues:   issues,
		Details:  details,
		Duration: duration,
		Metrics: map[string]float64{
			"errors": float64(errorCount),
		},
	}, nil
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
			RuleID:   ruleID,
			Message:  message,
			File:     file,
			Line:     lineNum,
			Column:   col,
			Severity: domain.SeverityMajor,
			Type:     domain.IssueTypeBug,
			Source:   "tsc",
			Effort:   "10min",
		})
	}

	return issues
}
