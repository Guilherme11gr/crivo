package rating

import (
	"testing"

	"github.com/anthropics/quality-gate/internal/domain"
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
