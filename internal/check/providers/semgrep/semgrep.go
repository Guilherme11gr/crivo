package semgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

// semgrepOutput matches Semgrep's JSON output
type semgrepOutput struct {
	Results []semgrepResult `json:"results"`
	Errors  []semgrepError  `json:"errors"`
}

type semgrepResult struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Start   struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"start"`
	End struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"end"`
	Extra struct {
		Message  string            `json:"message"`
		Severity string            `json:"severity"`
		Metadata map[string]any    `json:"metadata"`
		Lines    string            `json:"lines"`
	} `json:"extra"`
}

type semgrepError struct {
	Message string `json:"message"`
	Level   string `json:"level"`
}

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Security (Semgrep)" }
func (p *Provider) ID() string   { return "semgrep" }

func (p *Provider) Detect(_ context.Context, _ string) bool {
	_, err := exec.LookPath("semgrep")
	return err == nil
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
	start := time.Now()

	args := []string{
		"scan",
		"--json",
		"--quiet",
		"--config", "auto",
	}

	// Add exclude patterns
	for _, exc := range cfg.Exclude {
		args = append(args, "--exclude", exc)
	}

	// Add source directories
	if len(cfg.Src) > 0 {
		args = append(args, cfg.Src...)
	} else {
		args = append(args, ".")
	}

	cmd := exec.CommandContext(ctx, "semgrep", args...)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()
	duration := time.Since(start)

	output := stdout.Bytes()
	if len(output) == 0 {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "command not found") || strings.Contains(errMsg, "not recognized") {
			return &domain.CheckResult{
				Name:     p.Name(),
				ID:       p.ID(),
				Status:   domain.StatusSkipped,
				Summary:  "semgrep not installed",
				Duration: duration,
			}, nil
		}
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusPassed,
			Summary:  "0 findings",
			Duration: duration,
		}, nil
	}

	var result semgrepOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return &domain.CheckResult{
			Name:     p.Name(),
			ID:       p.ID(),
			Status:   domain.StatusError,
			Summary:  "Failed to parse Semgrep output",
			Details:  []string{err.Error()},
			Duration: duration,
		}, nil
	}

	var issues []domain.Issue
	vulns := 0
	hotspots := 0

	for _, r := range result.Results {
		relPath := r.Path
		if rel, err := filepath.Rel(projectDir, r.Path); err == nil {
			relPath = rel
		}
		relPath = filepath.ToSlash(relPath)

		severity := mapSeverity(r.Extra.Severity)
		issueType := classifyFinding(r.CheckID, r.Extra.Metadata)

		if issueType == domain.IssueTypeVulnerability {
			vulns++
		} else {
			hotspots++
		}

		effort := "15min"
		if severity == domain.SeverityCritical || severity == domain.SeverityBlocker {
			effort = "30min"
		}

		// Extract CWE/OWASP from metadata
		ruleID := r.CheckID
		if cwe, ok := extractCWE(r.Extra.Metadata); ok {
			ruleID += " [" + cwe + "]"
		}

		issues = append(issues, domain.Issue{
			RuleID:   ruleID,
			Message:  r.Extra.Message,
			File:     relPath,
			Line:     r.Start.Line,
			Column:   r.Start.Col,
			Severity: severity,
			Type:     issueType,
			Source:   "semgrep",
			Effort:   effort,
		})
	}

	status := domain.StatusPassed
	if vulns > 0 {
		status = domain.StatusFailed
	} else if hotspots > 0 {
		status = domain.StatusWarning
	}

	parts := []string{}
	if vulns > 0 {
		s := strconv.Itoa(vulns) + " vulnerability"
		if vulns != 1 {
			s = strconv.Itoa(vulns) + " vulnerabilities"
		}
		parts = append(parts, s)
	}
	if hotspots > 0 {
		s := strconv.Itoa(hotspots) + " hotspot"
		if hotspots != 1 {
			s += "s"
		}
		parts = append(parts, s)
	}
	summary := "0 findings"
	if len(parts) > 0 {
		summary = strings.Join(parts, " · ")
	}

	details := make([]string, 0, min(len(issues), 20))
	for i, issue := range issues {
		if i >= 20 {
			break
		}
		details = append(details, issue.File+":"+strconv.Itoa(issue.Line)+" "+issue.Message+" ("+issue.RuleID+")")
	}

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: summary,
		Issues:  issues,
		Details: details,
		Duration: duration,
		Metrics: map[string]float64{
			"vulnerabilities": float64(vulns),
			"hotspots":        float64(hotspots),
		},
	}, nil
}

func mapSeverity(s string) domain.Severity {
	switch strings.ToUpper(s) {
	case "ERROR":
		return domain.SeverityCritical
	case "WARNING":
		return domain.SeverityMajor
	case "INFO":
		return domain.SeverityMinor
	default:
		return domain.SeverityMinor
	}
}

func classifyFinding(checkID string, metadata map[string]any) domain.IssueType {
	// Check metadata for security category
	if cat, ok := metadata["category"]; ok {
		catStr, _ := cat.(string)
		if catStr == "security" {
			return domain.IssueTypeVulnerability
		}
	}

	// Check rule ID patterns
	lower := strings.ToLower(checkID)
	securityPatterns := []string{"sql-injection", "xss", "csrf", "ssrf", "injection", "auth", "crypto", "taint"}
	for _, p := range securityPatterns {
		if strings.Contains(lower, p) {
			return domain.IssueTypeVulnerability
		}
	}

	return domain.IssueTypeSecurityHotspot
}

func extractCWE(metadata map[string]any) (string, bool) {
	if cwe, ok := metadata["cwe"]; ok {
		switch v := cwe.(type) {
		case string:
			return v, true
		case []any:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					return s, true
				}
			}
		}
	}
	return "", false
}
