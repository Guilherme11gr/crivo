package coverage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

// coverageSummary matches Jest's coverage-summary.json format
// Keys are file paths (plus "total" for the aggregate)
type coverageSummary map[string]coverageEntry

type coverageEntry struct {
	Lines      coverageMetric `json:"lines"`
	Branches   coverageMetric `json:"branches"`
	Functions  coverageMetric `json:"functions"`
	Statements coverageMetric `json:"statements"`
}

type coverageMetric struct {
	Total   int     `json:"total"`
	Covered int     `json:"covered"`
	Skipped int     `json:"skipped"`
	Pct     float64 `json:"pct"`
}

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Coverage" }
func (p *Provider) ID() string   { return "coverage" }

// detectTestRunner determines which test runner is available in the project.
// Returns "vitest" or "jest". Vitest takes priority if both are present.
func detectTestRunner(projectDir string) string {
	// Check package.json for test runner dependencies
	pkgPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return ""
	}

	var pkg map[string]json.RawMessage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	// Check for test script
	if scripts, ok := pkg["scripts"]; ok {
		var s map[string]string
		if json.Unmarshal(scripts, &s) == nil {
			if testCmd, hasTest := s["test"]; hasTest {
				if strings.Contains(testCmd, "vitest") {
					return "vitest"
				}
				if strings.Contains(testCmd, "jest") {
					return "jest"
				}
			}
		}
	}

	// Check for test runner in dependencies (vitest takes priority)
	for _, key := range []string{"devDependencies", "dependencies"} {
		if deps, ok := pkg[key]; ok {
			var d map[string]string
			if json.Unmarshal(deps, &d) == nil {
				if _, hasVitest := d["vitest"]; hasVitest {
					return "vitest"
				}
				if _, hasJest := d["jest"]; hasJest {
					return "jest"
				}
			}
		}
	}

	return ""
}

func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	return detectTestRunner(projectDir) != ""
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	runner := detectTestRunner(projectDir)

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

	// Build command based on detected test runner
	var args []string
	switch runner {
	case "vitest":
		args = []string{"vitest", "run",
			"--coverage",
			"--coverage.reporter=json-summary",
			"--passWithNoTests",
		}
	default: // jest
		args = []string{"jest",
			"--coverage",
			"--coverageReporters=json-summary",
			"--passWithNoTests",
			"--silent",
		}
	}

	cmd := exec.CommandContext(ctx, npxBin, args...)
	cmd.Dir = projectDir
	cmd.Env = append(check.NodeEnv(), "CI=true", "NODE_ENV=test")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)

	// Read coverage summary (same format for both jest and vitest)
	summaryPath := filepath.Join(projectDir, "coverage", "coverage-summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		// No coverage generated
		if runErr != nil {
			output := stdout.String() + stderr.String()
			return &domain.CheckResult{
				Name:     p.Name(),
				ID:       p.ID(),
				Status:   domain.StatusFailed,
				Summary:  fmt.Sprintf("Tests failed, no coverage generated (runner: %s)", runner),
				Details:  extractTestFailures(output),
				Duration: duration,
			}, nil
		}

		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusWarning,
			Summary:  "No coverage data generated",
			Duration: duration,
		}, nil
	}

	var summary coverageSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusError,
			Summary:  "Failed to parse coverage data",
			Details:  []string{err.Error()},
			Duration: duration,
		}, nil
	}

	total, ok := summary["total"]
	if !ok {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusError,
			Summary:  "No total entry in coverage data",
			Duration: duration,
		}, nil
	}

	metrics := map[string]float64{
		"lines":      total.Lines.Pct,
		"branches":   total.Branches.Pct,
		"functions":  total.Functions.Pct,
		"statements": total.Statements.Pct,
		"test_runner": testRunnerMetric(runner),
	}

	// Check thresholds
	var failures []string
	if total.Lines.Pct < cfg.Coverage.Lines {
		failures = append(failures, fmt.Sprintf("Lines: %.1f%% < %.1f%%", total.Lines.Pct, cfg.Coverage.Lines))
	}
	if total.Branches.Pct < cfg.Coverage.Branches {
		failures = append(failures, fmt.Sprintf("Branches: %.1f%% < %.1f%%", total.Branches.Pct, cfg.Coverage.Branches))
	}
	if total.Functions.Pct < cfg.Coverage.Functions {
		failures = append(failures, fmt.Sprintf("Functions: %.1f%% < %.1f%%", total.Functions.Pct, cfg.Coverage.Functions))
	}
	if total.Statements.Pct < cfg.Coverage.Statements {
		failures = append(failures, fmt.Sprintf("Statements: %.1f%% < %.1f%%", total.Statements.Pct, cfg.Coverage.Statements))
	}

	status := domain.StatusPassed
	if len(failures) > 0 {
		status = domain.StatusFailed
	}

	details := []string{
		fmt.Sprintf("Lines:      %.1f%% (min: %.1f%%)", total.Lines.Pct, cfg.Coverage.Lines),
		fmt.Sprintf("Branches:   %.1f%% (min: %.1f%%)", total.Branches.Pct, cfg.Coverage.Branches),
		fmt.Sprintf("Functions:  %.1f%% (min: %.1f%%)", total.Functions.Pct, cfg.Coverage.Functions),
		fmt.Sprintf("Statements: %.1f%% (min: %.1f%%)", total.Statements.Pct, cfg.Coverage.Statements),
	}
	if len(failures) > 0 {
		details = append(details, "")
		for _, f := range failures {
			details = append(details, "FAILED: "+f)
		}
	}

	// Per-file issues: flag files with lines coverage below threshold
	type fileIssue struct {
		issue     domain.Issue
		uncovered int
	}
	var fileIssues []fileIssue
	for filePath, entry := range summary {
		if filePath == "total" {
			continue
		}

		relPath := filePath
		if rel, err := filepath.Rel(projectDir, filePath); err == nil {
			relPath = rel
		}
		relPath = filepath.ToSlash(relPath)

		// Skip tiny files (barrel/index files, single-line re-exports)
		if entry.Lines.Total < 5 {
			continue
		}

		if entry.Lines.Pct < cfg.Coverage.Lines && entry.Lines.Total > 0 {
			uncovered := entry.Lines.Total - entry.Lines.Covered
			fileIssues = append(fileIssues, fileIssue{
				uncovered: uncovered,
				issue: domain.Issue{
					RuleID:   "low-coverage",
					Message:  fmt.Sprintf("%.1f%% lines (%d/%d covered, %d to cover)", entry.Lines.Pct, entry.Lines.Covered, entry.Lines.Total, uncovered),
					File:     relPath,
					Line:     1,
					Column:   1,
					Severity: severityForCoverage(entry.Lines.Pct, cfg.Coverage.Lines),
					Type:     domain.IssueTypeCodeSmell,
					Source:   "coverage",
					Effort:   fmt.Sprintf("%dmin", uncovered*2),
				},
			})
		}
	}

	// Sort by uncovered lines descending (most impactful first)
	sort.Slice(fileIssues, func(i, j int) bool {
		return fileIssues[i].uncovered > fileIssues[j].uncovered
	})

	issues := make([]domain.Issue, len(fileIssues))
	for i, fi := range fileIssues {
		issues[i] = fi.issue
	}

	// Add per-file summary count to details
	if len(issues) > 0 {
		details = append(details, "")
		details = append(details, fmt.Sprintf("%d files below %.0f%% line coverage", len(issues), cfg.Coverage.Lines))
	}

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: fmt.Sprintf("%.1f%% lines · %.1f%% branches (min: %.0f%%/%.0f%%)", total.Lines.Pct, total.Branches.Pct, cfg.Coverage.Lines, cfg.Coverage.Branches),
		Metrics: metrics,
		Issues:  issues,
		Details: details,
		Duration: duration,
	}, nil
}

func severityForCoverage(actual, threshold float64) domain.Severity {
	ratio := actual / threshold
	if ratio < 0.25 {
		return domain.SeverityCritical
	}
	if ratio < 0.5 {
		return domain.SeverityMajor
	}
	return domain.SeverityMinor
}

func extractTestFailures(output string) []string {
	var failures []string
	lines := bytes.Split([]byte(output), []byte("\n"))
	for _, line := range lines {
		s := string(line)
		if bytes.Contains(line, []byte("FAIL")) || bytes.Contains(line, []byte("●")) {
			failures = append(failures, s)
		}
	}
	if len(failures) > 15 {
		failures = failures[:15]
	}
	return failures
}

// testRunnerMetric returns a numeric code for the test runner (1=vitest, 2=jest)
func testRunnerMetric(runner string) float64 {
	switch runner {
	case "vitest":
		return 1
	case "jest":
		return 2
	default:
		return 0
	}
}
