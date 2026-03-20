package duplication

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/anthropics/quality-gate/internal/config"
	"github.com/anthropics/quality-gate/internal/domain"
)

type jscpdReport struct {
	Statistics struct {
		Total struct {
			Percentage float64 `json:"percentage"`
			Lines      int     `json:"lines"`
			Sources    int     `json:"sources"`
			Clones     int     `json:"clones"`
		} `json:"total"`
	} `json:"statistics"`
	Duplicates []jscpdDuplicate `json:"duplicates"`
}

type jscpdDuplicate struct {
	FirstFile  jscpdFile `json:"firstFile"`
	SecondFile jscpdFile `json:"secondFile"`
	Lines      int       `json:"lines"`
	Tokens     int       `json:"tokens"`
	Fragment   string    `json:"fragment"`
}

type jscpdFile struct {
	Name      string `json:"name"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	StartLoc  jscpdLoc `json:"startLoc"`
	EndLoc    jscpdLoc `json:"endLoc"`
}

type jscpdLoc struct {
	Line int `json:"line"`
	Col  int `json:"column"`
}

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Duplication" }
func (p *Provider) ID() string   { return "duplication" }

func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	// Check if any source files exist
	for _, src := range []string{"src", "lib", "app"} {
		if _, err := os.Stat(filepath.Join(projectDir, src)); err == nil {
			return true
		}
	}
	return false
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	reportDir := filepath.Join(projectDir, ".qualitygate-temp")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(reportDir)

	// Determine source directory
	srcDir := "src/"
	if len(cfg.Src) > 0 {
		srcDir = cfg.Src[0]
	}

	srcPath := filepath.Join(projectDir, srcDir)
	if _, err := os.Stat(srcPath); err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusSkipped,
			Summary:  fmt.Sprintf("Source directory %q not found", srcDir),
			Duration: time.Since(start),
		}, nil
	}

	args := []string{
		"jscpd",
		srcPath,
		fmt.Sprintf("--min-lines=%d", cfg.Duplication.MinLines),
		fmt.Sprintf("--min-tokens=%d", cfg.Duplication.MinTokens),
		"--reporters=json",
		"--output=" + reportDir,
		"--silent",
	}

	// Add ignore patterns
	for _, exc := range cfg.Exclude {
		args = append(args, "--ignore="+exc)
	}

	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// jscpd exits non-zero when duplicates found
	_ = cmd.Run()
	duration := time.Since(start)

	reportPath := filepath.Join(reportDir, "jscpd-report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "0% duplication",
			Duration: duration,
			Metrics:  map[string]float64{"percentage": 0, "clones": 0},
		}, nil
	}

	var report jscpdReport
	if err := json.Unmarshal(data, &report); err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusError,
			Summary:  "Failed to parse jscpd output",
			Details:  []string{err.Error()},
			Duration: duration,
		}, nil
	}

	pct := report.Statistics.Total.Percentage
	cloneCount := len(report.Duplicates)

	status := domain.StatusPassed
	if pct > cfg.Duplication.Threshold {
		status = domain.StatusFailed
	} else if pct > cfg.Duplication.Threshold*0.8 {
		status = domain.StatusWarning
	}

	// Build details
	details := make([]string, 0, min(cloneCount, 10))
	for i, dup := range report.Duplicates {
		if i >= 10 {
			break
		}
		first, _ := filepath.Rel(projectDir, dup.FirstFile.Name)
		second, _ := filepath.Rel(projectDir, dup.SecondFile.Name)
		first = filepath.ToSlash(first)
		second = filepath.ToSlash(second)
		details = append(details, fmt.Sprintf("%s:%d <-> %s:%d (%d lines)",
			first, dup.FirstFile.StartLoc.Line,
			second, dup.SecondFile.StartLoc.Line,
			dup.Lines))
	}

	// Create issues for each duplicate
	var issues []domain.Issue
	for _, dup := range report.Duplicates {
		first, _ := filepath.Rel(projectDir, dup.FirstFile.Name)
		second, _ := filepath.Rel(projectDir, dup.SecondFile.Name)
		first = filepath.ToSlash(first)
		second = filepath.ToSlash(second)

		issues = append(issues, domain.Issue{
			RuleID:   "duplication",
			Message:  fmt.Sprintf("Duplicated %d lines with %s:%d", dup.Lines, second, dup.SecondFile.StartLoc.Line),
			File:     first,
			Line:     dup.FirstFile.StartLoc.Line,
			Column:   1,
			Severity: domain.SeverityMinor,
			Type:     domain.IssueTypeCodeSmell,
			Source:   "jscpd",
			Effort:   fmt.Sprintf("%dmin", dup.Lines*2),
		})
	}

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: fmt.Sprintf("%.1f%% (max: %.1f%%)", pct, cfg.Duplication.Threshold),
		Issues:  issues,
		Details: details,
		Duration: duration,
		Metrics: map[string]float64{
			"percentage": pct,
			"clones":     float64(cloneCount),
		},
	}, nil
}
