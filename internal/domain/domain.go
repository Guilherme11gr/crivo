package domain

import "time"

// Severity levels for issues (aligned with SonarQube)
type Severity string

const (
	SeverityBlocker  Severity = "blocker"
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
	SeverityInfo     Severity = "info"
)

// IssueType categorizes what kind of problem was found
type IssueType string

const (
	IssueTypeBug           IssueType = "bug"
	IssueTypeVulnerability IssueType = "vulnerability"
	IssueTypeCodeSmell     IssueType = "code_smell"
	IssueTypeSecurityHotspot IssueType = "security_hotspot"
)

// Issue represents a single finding from a check
type Issue struct {
	RuleID   string    `json:"ruleId"`
	Message  string    `json:"message"`
	File     string    `json:"file"`
	Line     int       `json:"line"`
	Column   int       `json:"column"`
	Severity Severity  `json:"severity"`
	Type     IssueType `json:"type"`
	Source   string    `json:"source"` // which provider found it
	Effort   string    `json:"effort"` // estimated fix time e.g. "5min"
}

// CheckStatus represents the outcome of a single check
type CheckStatus string

const (
	StatusPassed  CheckStatus = "passed"
	StatusFailed  CheckStatus = "failed"
	StatusWarning CheckStatus = "warning"
	StatusSkipped CheckStatus = "skipped"
	StatusError   CheckStatus = "error"
)

// CheckResult is what each provider returns
type CheckResult struct {
	Name     string            `json:"name"`
	ID       string            `json:"id"`
	Status   CheckStatus       `json:"status"`
	Summary  string            `json:"summary"`
	Issues   []Issue           `json:"issues,omitempty"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	Duration time.Duration     `json:"duration"`
	Details  []string          `json:"details,omitempty"`
}

// Rating represents A-E quality ratings (SonarQube style)
type Rating string

const (
	RatingA Rating = "A"
	RatingB Rating = "B"
	RatingC Rating = "C"
	RatingD Rating = "D"
	RatingE Rating = "E"
)

// QualityGateStatus is the overall pass/fail
type QualityGateStatus string

const (
	GatePassed QualityGateStatus = "passed"
	GateFailed QualityGateStatus = "failed"
)

// QualityGateCondition is a single threshold check
type QualityGateCondition struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"` // "lt" (less than) or "gt" (greater than)
	Threshold float64 `json:"threshold"`
	Actual    float64 `json:"actual"`
	Passed    bool    `json:"passed"`
}

// AnalysisResult is the top-level result of a full analysis run
type AnalysisResult struct {
	Version       string                `json:"version"`
	ProjectDir    string                `json:"projectDir"`
	Status        QualityGateStatus     `json:"status"`
	Checks        []CheckResult         `json:"checks"`
	Conditions    []QualityGateCondition `json:"conditions"`
	Ratings       map[string]Rating     `json:"ratings"`
	TotalIssues   int                   `json:"totalIssues"`
	TotalDuration time.Duration         `json:"totalDuration"`
	Timestamp     time.Time             `json:"timestamp"`
}

// AllIssues collects issues from all checks
func (r *AnalysisResult) AllIssues() []Issue {
	var all []Issue
	for _, c := range r.Checks {
		all = append(all, c.Issues...)
	}
	return all
}

// CountBySeverity counts issues by severity
func (r *AnalysisResult) CountBySeverity(s Severity) int {
	count := 0
	for _, issue := range r.AllIssues() {
		if issue.Severity == s {
			count++
		}
	}
	return count
}

// CountByType counts issues by type
func (r *AnalysisResult) CountByType(t IssueType) int {
	count := 0
	for _, issue := range r.AllIssues() {
		if issue.Type == t {
			count++
		}
	}
	return count
}
