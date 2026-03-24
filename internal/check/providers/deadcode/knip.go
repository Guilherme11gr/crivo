package deadcode

import (
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

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Dead Code" }
func (p *Provider) ID() string   { return "dead-code" }

func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	// Only detect for TypeScript/JS projects
	_, err := os.Stat(filepath.Join(projectDir, "package.json"))
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

	cmd := exec.CommandContext(ctx, npxBin, "knip", "--no-progress", "--reporter=compact")
	cmd.Dir = projectDir
	cmd.Env = check.NodeEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)

	output := stdout.String()
	if runErr != nil && output == "" {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "command not found") || strings.Contains(errMsg, "not recognized") || strings.Contains(errMsg, "ERR!") {
			return &domain.CheckResult{
				Name:     p.Name(),
				ID:       p.ID(),
				Status:   domain.StatusSkipped,
				Summary:  "knip not available",
				Duration: duration,
			}, nil
		}
	}

	// Parse knip compact output
	issues, unusedFiles, unusedExports, unusedDeps := parseKnipOutput(output, projectDir)

	if len(issues) == 0 {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "No dead code detected",
			Duration: duration,
		}, nil
	}

	parts := []string{}
	if unusedFiles > 0 {
		parts = append(parts, strconv.Itoa(unusedFiles)+" unused files")
	}
	if unusedExports > 0 {
		parts = append(parts, strconv.Itoa(unusedExports)+" unused exports")
	}
	if unusedDeps > 0 {
		parts = append(parts, strconv.Itoa(unusedDeps)+" unused deps")
	}

	status := domain.StatusWarning
	if unusedFiles > 5 || unusedExports > 20 {
		status = domain.StatusFailed
	}

	details := make([]string, 0, min(len(issues), 20))
	for i, issue := range issues {
		if i >= 20 {
			break
		}
		details = append(details, issue.File+":"+strconv.Itoa(issue.Line)+" "+issue.Message)
	}

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: strings.Join(parts, " · "),
		Issues:  issues,
		Details: details,
		Duration: duration,
		Metrics: map[string]float64{
			"unused_files":   float64(unusedFiles),
			"unused_exports": float64(unusedExports),
			"unused_deps":    float64(unusedDeps),
		},
	}, nil
}

var (
	fileLineRe   = regexp.MustCompile(`^(.+?)(?::(\d+))?$`)
	sectionRe    = regexp.MustCompile(`^(Unused files|Unused dependencies|Unused exports|Unused types)`)
)

func parseKnipOutput(output string, projectDir string) ([]domain.Issue, int, int, int) {
	var issues []domain.Issue
	unusedFiles := 0
	unusedExports := 0
	unusedDeps := 0

	lines := strings.Split(output, "\n")
	currentSection := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect section headers
		if matches := sectionRe.FindStringSubmatch(trimmed); matches != nil {
			currentSection = matches[1]
			continue
		}

		// Skip non-file lines (except in Unused dependencies where entries are package names)
		if currentSection != "Unused dependencies" &&
			!strings.Contains(trimmed, "/") && !strings.Contains(trimmed, "\\") && !strings.Contains(trimmed, ".") {
			continue
		}

		// Parse file references
		severity := domain.SeverityMinor
		issueType := domain.IssueTypeCodeSmell
		ruleID := "dead-code"
		message := ""
		lineNum := 1

		switch currentSection {
		case "Unused files":
			unusedFiles++
			ruleID = "unused-file"
			message = "File is not imported anywhere"
			severity = domain.SeverityMajor
		case "Unused exports":
			unusedExports++
			ruleID = "unused-export"
			message = "Export is not used"
		case "Unused types":
			unusedExports++
			ruleID = "unused-type"
			message = "Type export is not used"
		case "Unused dependencies":
			unusedDeps++
			ruleID = "unused-dependency"
			message = "Dependency is not imported: " + trimmed
			// deps don't have file paths
			issues = append(issues, domain.Issue{
				RuleID:   ruleID,
				Message:  message,
				File:     "package.json",
				Line:     1,
				Column:   1,
				Severity: severity,
				Type:     issueType,
				Source:   "knip",
				Effort:   "5min",
			})
			continue
		default:
			continue
		}

		// Extract file:line
		if m := fileLineRe.FindStringSubmatch(trimmed); m != nil {
			file := m[1]
			if m[2] != "" {
				lineNum, _ = strconv.Atoi(m[2])
			}
			relPath := file
			if rel, err := filepath.Rel(projectDir, file); err == nil {
				relPath = rel
			}
			relPath = filepath.ToSlash(relPath)

			issues = append(issues, domain.Issue{
				RuleID:   ruleID,
				Message:  message,
				File:     relPath,
				Line:     lineNum,
				Column:   1,
				Severity: severity,
				Type:     issueType,
				Source:   "knip",
				Effort:   "5min",
			})
		}
	}

	return issues, unusedFiles, unusedExports, unusedDeps
}
