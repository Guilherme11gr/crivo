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

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
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

	// Use forward slashes for jscpd compatibility on Windows
	srcPath = filepath.ToSlash(srcPath)

	args := []string{
		"jscpd",
		srcPath,
		fmt.Sprintf("--min-lines=%d", cfg.Duplication.MinLines),
		fmt.Sprintf("--min-tokens=%d", cfg.Duplication.MinTokens),
		"--reporters=json",
		"--output=" + filepath.ToSlash(reportDir),
	}

	// Add ignore patterns
	for _, exc := range cfg.Exclude {
		args = append(args, "--ignore="+exc)
	}

	npxBin := check.FindNpx()
	if npxBin == "" {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusSkipped,
			Summary:  "npx not found (install Node.js)",
			Duration: time.Since(start),
		}, nil
	}

	cmd := exec.CommandContext(ctx, npxBin, args...)
	cmd.Dir = projectDir
	cmd.Env = check.NodeEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// jscpd exits non-zero when duplicates found
	_ = cmd.Run()
	duration := time.Since(start)

	reportPath := filepath.Join(reportDir, "jscpd-report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		// Check if jscpd actually ran — if stderr has content, report the error
		stderrStr := stderr.String()
		stdoutStr := stdout.String()
		errMsg := "No jscpd report generated"
		if stderrStr != "" {
			errMsg += ": " + stderrStr
		} else if stdoutStr != "" {
			errMsg += ": " + stdoutStr
		}
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusWarning,
			Summary:  errMsg,
			Duration: duration,
			Details:  []string{"jscpd report not found at " + reportPath, "stderr: " + stderrStr},
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
		first := normalizePath(projectDir, dup.FirstFile.Name)
		second := normalizePath(projectDir, dup.SecondFile.Name)
		details = append(details, fmt.Sprintf("%s:%d <-> %s:%d (%d lines)",
			first, dup.FirstFile.StartLoc.Line,
			second, dup.SecondFile.StartLoc.Line,
			dup.Lines))
	}

	// Create issues for each duplicate
	var issues []domain.Issue
	for _, dup := range report.Duplicates {
		// Normalize paths: jscpd may return absolute or relative paths
		first := normalizePath(projectDir, dup.FirstFile.Name)
		second := normalizePath(projectDir, dup.SecondFile.Name)

		issues = append(issues, domain.Issue{
			RuleID:      "duplication",
			Message:     fmt.Sprintf("Duplicated %d lines with %s:%d", dup.Lines, second, dup.SecondFile.StartLoc.Line),
			File:        first,
			Line:        dup.FirstFile.StartLoc.Line,
			Column:      1,
			Severity:    domain.SeverityMinor,
			Type:        domain.IssueTypeCodeSmell,
			Source:      "jscpd",
			Effort:      fmt.Sprintf("%dmin", dup.Lines*2),
			Remediation: domain.DuplicationRemediation("duplication"),
		})
	}

	// --- Semantic duplication detection (built-in, no external tool) ---
	semanticCloneCount := 0
	if cfg.Duplication.Semantic {
		srcDirs := cfg.Src
		if len(srcDirs) == 0 {
			srcDirs = []string{"src/"}
		}
		minFuncLines := cfg.Duplication.SemanticMinLines
		if minFuncLines <= 0 {
			minFuncLines = 5
		}
		simThreshold := cfg.Duplication.SimilarityThreshold
		if simThreshold <= 0 {
			simThreshold = 0.85
		}

		clones := findSemanticClones(projectDir, srcDirs, cfg.Exclude, minFuncLines, simThreshold, cfg.Duplication.SemanticExclude)
		semanticCloneCount = len(clones)

		for i, clone := range clones {
			if i >= 20 {
				break // cap reported issues
			}
			label := "Exact semantic clone"
			sev := domain.SeverityMajor
			if clone.Similarity < 1.0 {
				label = fmt.Sprintf("Similar clone (%.0f%%)", clone.Similarity*100)
				sev = domain.SeverityMinor
			}
			issues = append(issues, domain.Issue{
				RuleID:      "semantic-duplication",
				Message:     fmt.Sprintf("%s: %s:%d (%s) ≈ %s:%d (%s)", label, clone.A.File, clone.A.Line, clone.A.Name, clone.B.File, clone.B.Line, clone.B.Name),
				File:        clone.A.File,
				Line:        clone.A.Line,
				Column:      1,
				Severity:    sev,
				Type:        domain.IssueTypeCodeSmell,
				Source:      "semantic",
				Effort:      fmt.Sprintf("%dmin", clone.A.BodyLines*3),
				Remediation: domain.DuplicationRemediation("semantic-duplication"),
			})

			details = append(details, fmt.Sprintf("[semantic] %s:%d %s ≈ %s:%d %s (%.0f%%)",
				clone.A.File, clone.A.Line, clone.A.Name,
				clone.B.File, clone.B.Line, clone.B.Name,
				clone.Similarity*100))
		}

		// Update status if semantic clones found
		if semanticCloneCount > 0 && status == domain.StatusPassed {
			status = domain.StatusWarning
		}
		if semanticCloneCount > 5 && status != domain.StatusFailed {
			status = domain.StatusFailed
		}
	}

	semanticStr := ""
	if cfg.Duplication.Semantic {
		semanticStr = fmt.Sprintf(" · %d semantic clones", semanticCloneCount)
	}

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: fmt.Sprintf("%.1f%% (max: %.1f%%)%s", pct, cfg.Duplication.Threshold, semanticStr),
		Issues:  issues,
		Details: details,
		Duration: duration,
		Metrics: map[string]float64{
			"percentage":      pct,
			"clones":          float64(cloneCount),
			"semantic_clones": float64(semanticCloneCount),
		},
	}, nil
}

// normalizePath converts a path (absolute or relative) to a relative path from projectDir.
// jscpd may return either absolute or relative paths depending on the OS and version.
func normalizePath(projectDir, p string) string {
	if p == "" {
		return ""
	}
	// If relative, make absolute first so filepath.Rel works correctly
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectDir, p)
	}
	rel, err := filepath.Rel(projectDir, p)
	if err != nil || rel == "" {
		return filepath.ToSlash(filepath.Base(p))
	}
	return filepath.ToSlash(rel)
}

