package output

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/guilherme11gr/crivo/internal/domain"
)

// JSONReport is the structured output for AI agents
type JSONReport struct {
	Version     string              `json:"version"`
	Status      domain.QualityGateStatus `json:"status"`
	ProjectDir  string              `json:"projectDir"`
	Timestamp   time.Time           `json:"timestamp"`
	Duration    string              `json:"duration"`
	TotalIssues int                 `json:"totalIssues"`
	Ratings     map[string]domain.Rating `json:"ratings"`
	Conditions  []ConditionJSON     `json:"conditions"`
	Checks      []CheckJSON         `json:"checks"`
	Summary     SummaryJSON         `json:"summary"`
}

type ConditionJSON struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	Actual    float64 `json:"actual"`
	Passed    bool    `json:"passed"`
}

type CheckJSON struct {
	Name     string             `json:"name"`
	ID       string             `json:"id"`
	Status   domain.CheckStatus `json:"status"`
	Summary  string             `json:"summary"`
	Duration string             `json:"duration"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	Issues   []IssueJSON        `json:"issues,omitempty"`
}

type IssueJSON struct {
	RuleID      string          `json:"ruleId"`
	Message     string          `json:"message"`
	File        string          `json:"file"`
	Line        int             `json:"line"`
	Column      int             `json:"column"`
	Severity    domain.Severity `json:"severity"`
	Type        domain.IssueType `json:"type"`
	Source      string          `json:"source"`
	Effort      string          `json:"effort"`
	Remediation string          `json:"remediation,omitempty"`
}

type SummaryJSON struct {
	Bugs            int     `json:"bugs"`
	Vulnerabilities int     `json:"vulnerabilities"`
	CodeSmells      int     `json:"codeSmells"`
	CoveragePercent float64 `json:"coveragePercent"`
	DuplicationPercent float64 `json:"duplicationPercent"`
}

// PrintJSON outputs structured JSON to stdout
func PrintJSON(result *domain.AnalysisResult) error {
	report := toJSONReport(result)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// ToJSON returns the JSON bytes
func ToJSON(result *domain.AnalysisResult) ([]byte, error) {
	return json.MarshalIndent(toJSONReport(result), "", "  ")
}

func toJSONReport(result *domain.AnalysisResult) JSONReport {
	report := JSONReport{
		Version:     result.Version,
		Status:      result.Status,
		ProjectDir:  result.ProjectDir,
		Timestamp:   result.Timestamp,
		Duration:    formatDuration(result.TotalDuration),
		TotalIssues: result.TotalIssues,
		Ratings:     result.Ratings,
	}

	// Conditions
	for _, c := range result.Conditions {
		report.Conditions = append(report.Conditions, ConditionJSON{
			Metric:    c.Metric,
			Operator:  c.Operator,
			Threshold: c.Threshold,
			Actual:    c.Actual,
			Passed:    c.Passed,
		})
	}

	// Checks
	for _, check := range result.Checks {
		cj := CheckJSON{
			Name:     check.Name,
			ID:       check.ID,
			Status:   check.Status,
			Summary:  check.Summary,
			Duration: formatDuration(check.Duration),
			Metrics:  check.Metrics,
		}

		for _, issue := range check.Issues {
			cj.Issues = append(cj.Issues, IssueJSON{
				RuleID:      issue.RuleID,
				Message:     issue.Message,
				File:        issue.File,
				Line:        issue.Line,
				Column:      issue.Column,
				Severity:    issue.Severity,
				Type:        issue.Type,
				Source:      issue.Source,
				Effort:      issue.Effort,
				Remediation: issue.Remediation,
			})
		}

		report.Checks = append(report.Checks, cj)
	}

	// Summary
	report.Summary = SummaryJSON{
		Bugs:            result.CountByType(domain.IssueTypeBug),
		Vulnerabilities: result.CountByType(domain.IssueTypeVulnerability),
		CodeSmells:      result.CountByType(domain.IssueTypeCodeSmell),
	}

	// Extract coverage/duplication from check metrics
	for _, check := range result.Checks {
		if check.ID == "coverage" && check.Metrics != nil {
			if v, ok := check.Metrics["lines"]; ok {
				report.Summary.CoveragePercent = v
			}
		}
		if check.ID == "duplication" && check.Metrics != nil {
			if v, ok := check.Metrics["percentage"]; ok {
				report.Summary.DuplicationPercent = v
			}
		}
	}

	return report
}
