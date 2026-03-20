package domain

import "testing"

func TestAnalysisResult_CountBySeverity(t *testing.T) {
	result := &AnalysisResult{
		Checks: []CheckResult{
			{
				Issues: []Issue{
					{Severity: SeverityMajor, Type: IssueTypeBug},
					{Severity: SeverityMinor, Type: IssueTypeBug},
					{Severity: SeverityMajor, Type: IssueTypeCodeSmell},
				},
			},
			{
				Issues: []Issue{
					{Severity: SeverityCritical, Type: IssueTypeVulnerability},
				},
			},
		},
	}

	if got := result.CountBySeverity(SeverityMajor); got != 2 {
		t.Errorf("CountBySeverity(Major) = %d, want 2", got)
	}
	if got := result.CountBySeverity(SeverityCritical); got != 1 {
		t.Errorf("CountBySeverity(Critical) = %d, want 1", got)
	}
	if got := result.CountBySeverity(SeverityBlocker); got != 0 {
		t.Errorf("CountBySeverity(Blocker) = %d, want 0", got)
	}
}

func TestAnalysisResult_CountByType(t *testing.T) {
	result := &AnalysisResult{
		Checks: []CheckResult{
			{
				Issues: []Issue{
					{Type: IssueTypeBug},
					{Type: IssueTypeBug},
					{Type: IssueTypeCodeSmell},
				},
			},
		},
	}

	if got := result.CountByType(IssueTypeBug); got != 2 {
		t.Errorf("CountByType(Bug) = %d, want 2", got)
	}
	if got := result.CountByType(IssueTypeVulnerability); got != 0 {
		t.Errorf("CountByType(Vulnerability) = %d, want 0", got)
	}
}

func TestAnalysisResult_AllIssues(t *testing.T) {
	result := &AnalysisResult{
		Checks: []CheckResult{
			{Issues: []Issue{{Message: "a"}, {Message: "b"}}},
			{Issues: []Issue{{Message: "c"}}},
			{Issues: nil},
		},
	}

	all := result.AllIssues()
	if len(all) != 3 {
		t.Errorf("AllIssues() returned %d issues, want 3", len(all))
	}
}
