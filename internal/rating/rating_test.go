package rating

import (
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestCalculateReliability(t *testing.T) {
	tests := []struct {
		name   string
		issues []domain.Issue
		want   domain.Rating
	}{
		{"no bugs", nil, domain.RatingA},
		{"minor bug", []domain.Issue{{Type: domain.IssueTypeBug, Severity: domain.SeverityMinor}}, domain.RatingB},
		{"major bug", []domain.Issue{{Type: domain.IssueTypeBug, Severity: domain.SeverityMajor}}, domain.RatingC},
		{"critical bug", []domain.Issue{{Type: domain.IssueTypeBug, Severity: domain.SeverityCritical}}, domain.RatingD},
		{"blocker bug", []domain.Issue{{Type: domain.IssueTypeBug, Severity: domain.SeverityBlocker}}, domain.RatingE},
		{"code smell ignored", []domain.Issue{{Type: domain.IssueTypeCodeSmell, Severity: domain.SeverityBlocker}}, domain.RatingA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateReliability(tt.issues)
			if got != tt.want {
				t.Errorf("CalculateReliability() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCalculateSecurity(t *testing.T) {
	tests := []struct {
		name   string
		issues []domain.Issue
		want   domain.Rating
	}{
		{"no vulns", nil, domain.RatingA},
		{"minor vuln", []domain.Issue{{Type: domain.IssueTypeVulnerability, Severity: domain.SeverityMinor}}, domain.RatingB},
		{"critical vuln", []domain.Issue{{Type: domain.IssueTypeVulnerability, Severity: domain.SeverityCritical}}, domain.RatingD},
		{"hotspot counted", []domain.Issue{{Type: domain.IssueTypeSecurityHotspot, Severity: domain.SeverityMajor}}, domain.RatingC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSecurity(tt.issues)
			if got != tt.want {
				t.Errorf("CalculateSecurity() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCalculateMaintainability(t *testing.T) {
	tests := []struct {
		name       string
		issues     []domain.Issue
		totalLines int
		want       domain.Rating
	}{
		{"no smells", nil, 1000, domain.RatingA},
		{"zero lines", nil, 0, domain.RatingA},
		{"low debt", []domain.Issue{
			{Type: domain.IssueTypeCodeSmell, Effort: "5min"},
		}, 1000, domain.RatingA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateMaintainability(tt.issues, tt.totalLines)
			if got != tt.want {
				t.Errorf("CalculateMaintainability() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseEffort(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"5min", 5},
		{"10min", 10},
		{"1h", 60},
		{"2h", 120},
		{"", 5},
		{"30m", 30},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseEffort(tt.input)
			if got != tt.want {
				t.Errorf("parseEffort(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestEvaluateQualityGate_ReleaseBlocksCustomRules(t *testing.T) {
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				ID:      "custom-rules",
				Status:  domain.StatusFailed,
				Metrics: map[string]float64{"blocking_violations": 2},
			},
		},
	}

	EvaluateQualityGate(result, "release")

	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed", result.Status)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("conditions = %d, want 1", len(result.Conditions))
	}
	if result.Conditions[0].Metric != "custom_rules_blocking" {
		t.Fatalf("metric = %q", result.Conditions[0].Metric)
	}
	if result.Conditions[0].Passed {
		t.Fatal("expected custom rules condition to fail")
	}
}

func TestEvaluateQualityGate_ReleaseBlocksDuplication(t *testing.T) {
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				ID:      "duplication",
				Status:  domain.StatusFailed,
				Metrics: map[string]float64{"percentage": 8.5},
			},
		},
	}

	EvaluateQualityGate(result, "release")

	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed", result.Status)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("conditions = %d, want 1", len(result.Conditions))
	}
	if result.Conditions[0].Metric != "duplication_pct" {
		t.Fatalf("metric = %q", result.Conditions[0].Metric)
	}
	if result.Conditions[0].Passed {
		t.Fatal("expected duplication condition to fail")
	}
}

func TestEvaluateQualityGate_DuplicationConditionUsesPercentageThreshold(t *testing.T) {
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				ID:     "duplication",
				Status: domain.StatusFailed,
				Metrics: map[string]float64{
					"percentage":      1.2,
					"semantic_clones": 20,
				},
			},
		},
	}

	EvaluateQualityGate(result, "release")

	if result.Status != domain.GatePassed {
		t.Fatalf("status = %s, want passed", result.Status)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("conditions = %d, want 1", len(result.Conditions))
	}
	condition := result.Conditions[0]
	if condition.Metric != "duplication_pct" {
		t.Fatalf("metric = %q, want duplication_pct", condition.Metric)
	}
	if !condition.Passed {
		t.Fatalf("duplication_pct condition failed with actual %.1f under threshold %.1f", condition.Actual, condition.Threshold)
	}
}
