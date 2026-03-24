package output

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/guilherme11gr/crivo/internal/domain"
)

// newTestResult creates a realistic AnalysisResult for testing.
func newTestResult() *domain.AnalysisResult {
	return &domain.AnalysisResult{
		Version:    "3.0.0",
		ProjectDir: "/home/user/project",
		Status:     domain.GatePassed,
		Timestamp:  time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
		TotalDuration: 2500 * time.Millisecond,
		TotalIssues:   3,
		Ratings: map[string]domain.Rating{
			"reliability":    domain.RatingA,
			"security":       domain.RatingB,
			"maintainability": domain.RatingA,
		},
		Conditions: []domain.QualityGateCondition{
			{Metric: "coverage", Operator: "lt", Threshold: 80, Actual: 85.5, Passed: true},
			{Metric: "duplication", Operator: "gt", Threshold: 5, Actual: 2.1, Passed: true},
		},
		Checks: []domain.CheckResult{
			{
				Name:     "TypeScript",
				ID:       "tsc",
				Status:   domain.StatusPassed,
				Summary:  "0 errors",
				Duration: 800 * time.Millisecond,
			},
			{
				Name:     "Coverage",
				ID:       "coverage",
				Status:   domain.StatusPassed,
				Summary:  "85.5% line coverage",
				Duration: 1200 * time.Millisecond,
				Metrics: map[string]float64{
					"lines":      85.5,
					"branches":   72.3,
					"functions":  90.1,
					"statements": 84.0,
				},
			},
			{
				Name:     "Duplication",
				ID:       "duplication",
				Status:   domain.StatusWarning,
				Summary:  "2.1% duplication",
				Duration: 500 * time.Millisecond,
				Metrics:  map[string]float64{"percentage": 2.1},
				Details:  []string{"src/utils.ts:10-25 duplicates src/helpers.ts:5-20"},
			},
			{
				Name:     "Security (Semgrep)",
				ID:       "semgrep",
				Status:   domain.StatusFailed,
				Summary:  "1 vulnerability",
				Duration: 3 * time.Second,
				Issues: []domain.Issue{
					{
						RuleID:   "sql-injection",
						Message:  "Possible SQL injection",
						File:     "src/db.ts",
						Line:     42,
						Column:   5,
						Severity: domain.SeverityCritical,
						Type:     domain.IssueTypeVulnerability,
						Source:   "semgrep",
						Effort:   "30min",
					},
				},
			},
		},
	}
}

func newFailedResult() *domain.AnalysisResult {
	r := newTestResult()
	r.Status = domain.GateFailed
	r.Conditions = []domain.QualityGateCondition{
		{Metric: "coverage", Operator: "lt", Threshold: 80, Actual: 65.0, Passed: false},
	}
	return r
}

func TestToJSON_ValidJSON(t *testing.T) {
	result := newTestResult()
	data, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("ToJSON produced invalid JSON: %v", err)
	}
}

func TestToJSON_Structure(t *testing.T) {
	result := newTestResult()
	data, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var report JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("failed to unmarshal into JSONReport: %v", err)
	}

	if report.Version != "3.0.0" {
		t.Errorf("version = %q, want %q", report.Version, "3.0.0")
	}
	if report.Status != domain.GatePassed {
		t.Errorf("status = %q, want %q", report.Status, domain.GatePassed)
	}
	if report.ProjectDir != "/home/user/project" {
		t.Errorf("projectDir = %q, want %q", report.ProjectDir, "/home/user/project")
	}
	if report.TotalIssues != 3 {
		t.Errorf("totalIssues = %d, want 3", report.TotalIssues)
	}
	if report.Duration != "2.5s" {
		t.Errorf("duration = %q, want %q", report.Duration, "2.5s")
	}
}

func TestToJSON_Checks(t *testing.T) {
	result := newTestResult()
	data, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var report JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(report.Checks) != 4 {
		t.Fatalf("expected 4 checks, got %d", len(report.Checks))
	}

	// Verify semgrep check has issues
	semgrepCheck := report.Checks[3]
	if semgrepCheck.ID != "semgrep" {
		t.Errorf("check[3].id = %q, want %q", semgrepCheck.ID, "semgrep")
	}
	if len(semgrepCheck.Issues) != 1 {
		t.Fatalf("expected 1 issue in semgrep check, got %d", len(semgrepCheck.Issues))
	}
	if semgrepCheck.Issues[0].RuleID != "sql-injection" {
		t.Errorf("issue ruleId = %q, want %q", semgrepCheck.Issues[0].RuleID, "sql-injection")
	}
}

func TestToJSON_Conditions(t *testing.T) {
	result := newTestResult()
	data, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var report JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(report.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(report.Conditions))
	}
	if report.Conditions[0].Metric != "coverage" {
		t.Errorf("condition[0].metric = %q, want %q", report.Conditions[0].Metric, "coverage")
	}
	if !report.Conditions[0].Passed {
		t.Error("condition[0].passed = false, want true")
	}
}

func TestToJSON_Summary(t *testing.T) {
	result := newTestResult()
	data, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var report JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if report.Summary.Vulnerabilities != 1 {
		t.Errorf("summary.vulnerabilities = %d, want 1", report.Summary.Vulnerabilities)
	}
	if report.Summary.CoveragePercent != 85.5 {
		t.Errorf("summary.coveragePercent = %f, want 85.5", report.Summary.CoveragePercent)
	}
	if report.Summary.DuplicationPercent != 2.1 {
		t.Errorf("summary.duplicationPercent = %f, want 2.1", report.Summary.DuplicationPercent)
	}
}

func TestToJSON_Ratings(t *testing.T) {
	result := newTestResult()
	data, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var report JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(report.Ratings) != 3 {
		t.Errorf("expected 3 ratings, got %d", len(report.Ratings))
	}
	if report.Ratings["reliability"] != domain.RatingA {
		t.Errorf("reliability rating = %q, want %q", report.Ratings["reliability"], domain.RatingA)
	}
	if report.Ratings["security"] != domain.RatingB {
		t.Errorf("security rating = %q, want %q", report.Ratings["security"], domain.RatingB)
	}
}

// --- Markdown tests ---

func TestGenerateMarkdown_PassedGate(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	if !strings.Contains(md, "## ") {
		t.Error("markdown should contain a heading")
	}
	if !strings.Contains(md, "Passed") {
		t.Error("markdown should contain 'Passed' for passed gate")
	}
	if strings.Contains(md, "Failed") {
		t.Error("markdown should not contain 'Failed' for passed gate")
	}
}

func TestGenerateMarkdown_FailedGate(t *testing.T) {
	result := newFailedResult()
	md := GenerateMarkdown(result)

	if !strings.Contains(md, "Failed") {
		t.Error("markdown should contain 'Failed' for failed gate")
	}
}

func TestGenerateMarkdown_ContainsTable(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	if !strings.Contains(md, "| Check |") {
		t.Error("markdown should contain table header")
	}
	if !strings.Contains(md, "| TypeScript |") {
		t.Error("markdown should contain TypeScript check row")
	}
	if !strings.Contains(md, "| Coverage |") {
		t.Error("markdown should contain Coverage check row")
	}
}

func TestGenerateMarkdown_StatusIcons(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	// Passed checks should have passed icon
	if !strings.Contains(md, statusIcons[domain.StatusPassed]) {
		t.Error("markdown should contain passed status icon")
	}
	// Warning checks should have warning icon
	if !strings.Contains(md, statusIcons[domain.StatusWarning]) {
		t.Error("markdown should contain warning status icon")
	}
	// Failed checks should have failed icon
	if !strings.Contains(md, statusIcons[domain.StatusFailed]) {
		t.Error("markdown should contain failed status icon")
	}
}

func TestGenerateMarkdown_DetailsForNonPassed(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	// Duplication is warning with details, should appear in a <details> block
	if !strings.Contains(md, "<details>") {
		t.Error("markdown should contain details block for non-passed checks")
	}
	if !strings.Contains(md, "Duplication") {
		t.Error("markdown should contain Duplication check details")
	}
}

func TestGenerateMarkdown_CoverageBreakdown(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	if !strings.Contains(md, "Coverage Breakdown") {
		t.Error("markdown should contain coverage breakdown section")
	}
	if !strings.Contains(md, "85.5%") {
		t.Error("markdown should contain line coverage percentage")
	}
}

func TestGenerateMarkdown_Ratings(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	if !strings.Contains(md, "### Ratings") {
		t.Error("markdown should contain Ratings section")
	}
}

func TestGenerateMarkdown_Footer(t *testing.T) {
	result := newTestResult()
	md := GenerateMarkdown(result)

	if !strings.Contains(md, "quality-gate") {
		t.Error("markdown footer should reference quality-gate")
	}
	if !strings.Contains(md, "Total time:") {
		t.Error("markdown footer should contain total time")
	}
}

// --- formatDuration tests ---

func TestFormatDuration_SubSecond(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0ms"},
		{50 * time.Millisecond, "50ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Millisecond, "1ms"},
		{500 * time.Microsecond, "0ms"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{1 * time.Second, "1.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{2500 * time.Millisecond, "2.5s"},
		{10 * time.Second, "10.0s"},
		{65 * time.Second, "65.0s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// --- padRight tests ---

func TestPadRight(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello     "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"},
		{"", 5, "     "},
		{"ab", 4, "ab  "},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := padRight(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}
