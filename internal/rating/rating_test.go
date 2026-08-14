package rating

import (
	"testing"

	"github.com/guilherme11gr/crivo/internal/config"
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

	EvaluateQualityGate(result, "release", DefaultGateThresholds())

	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed", result.Status)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("conditions = %d, want 2 (custom_rules_blocking + errored_checks)", len(result.Conditions))
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

	EvaluateQualityGate(result, "release", DefaultGateThresholds())

	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed", result.Status)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("conditions = %d, want 2 (duplication_pct + errored_checks)", len(result.Conditions))
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

	EvaluateQualityGate(result, "release", DefaultGateThresholds())

	if result.Status != domain.GatePassed {
		t.Fatalf("status = %s, want passed", result.Status)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("conditions = %d, want 2 (duplication_pct + errored_checks)", len(result.Conditions))
	}
	condition := result.Conditions[0]
	if condition.Metric != "duplication_pct" {
		t.Fatalf("metric = %q, want duplication_pct", condition.Metric)
	}
	if !condition.Passed {
		t.Fatalf("duplication_pct condition failed with actual %.1f under threshold %.1f", condition.Actual, condition.Threshold)
	}
}

func TestEvaluateQualityGate_TscCrashedBlocksRelease(t *testing.T) {
	// A crashed tsc (StatusError) with nil metrics must never emit a passing
	// condition, and must block the release gate fail-closed via
	// errored_checks. This is the "tsc crashado ⇒ gate reprova" guarantee.
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				ID:     "typescript",
				Status: domain.StatusError,
				// Metrics deliberately nil — the old code would read 0 errors
				// and emit type_errors Actual=0 Passed=true.
			},
		},
	}

	EvaluateQualityGate(result, "release", DefaultGateThresholds())

	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed (errored check must block release)", result.Status)
	}
	for _, c := range result.Conditions {
		if c.Metric == "type_errors" {
			t.Fatal("no type_errors condition may be built from an errored check")
		}
	}
	found := false
	for _, c := range result.Conditions {
		if c.Metric == "errored_checks" {
			found = true
			if c.Passed {
				t.Fatalf("errored_checks condition must fail, got Actual=%.0f", c.Actual)
			}
		}
	}
	if !found {
		t.Fatal("expected errored_checks condition in results")
	}
}

func TestEvaluateQualityGate_ErroredCheckDoesNotBlockInformational(t *testing.T) {
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{ID: "typescript", Status: domain.StatusError},
		},
	}

	EvaluateQualityGate(result, "informational", DefaultGateThresholds())

	if result.Status != domain.GatePassed {
		t.Fatalf("status = %s, want passed (informational never blocks)", result.Status)
	}
}

func TestEvaluateQualityGate_ConfigThresholdsChangeCondition(t *testing.T) {
	// .qualitygate.yaml with quality-gate.overall.coverage: 70 must change the
	// threshold of the coverage_lines condition.
	cfg := config.DefaultConfig()
	cfg.QualityGate.Overall.Coverage = 70
	cfg.QualityGate.Overall.Duplications = 8

	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				ID:     "coverage",
				Status: domain.StatusFailed,
				Metrics: map[string]float64{
					"lines": 65,
				},
			},
		},
	}

	// strict policy blocks on coverage_lines, so the config threshold is
	// observable in both the condition and the gate outcome.
	EvaluateQualityGate(result, "strict", ThresholdsFromConfig(cfg))

	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed (65 < 70)", result.Status)
	}
	var covCond *domain.QualityGateCondition
	for i := range result.Conditions {
		if result.Conditions[i].Metric == "coverage_lines" {
			covCond = &result.Conditions[i]
		}
	}
	if covCond == nil {
		t.Fatal("expected coverage_lines condition")
	}
	if covCond.Threshold != 70 {
		t.Fatalf("coverage_lines threshold = %.0f, want 70 from config", covCond.Threshold)
	}
	if covCond.Passed {
		t.Fatal("coverage_lines must fail with 65 < 70")
	}
}

func TestEvaluateQualityGate_ErroredCheckWithMetricsNilNeverEmitsCondition(t *testing.T) {
	// Regression guard: an errored check carrying stale metrics must still not
	// emit conditions — the StatusError check happens before the switch.
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				ID:     "secrets",
				Status: domain.StatusError,
				Metrics: map[string]float64{
					"secrets": 0, // stale/fabricated value
				},
			},
		},
	}

	EvaluateQualityGate(result, "release", DefaultGateThresholds())

	for _, c := range result.Conditions {
		if c.Metric == "secrets" {
			t.Fatal("no secrets condition may be built from an errored check")
		}
	}
	if result.Status != domain.GateFailed {
		t.Fatalf("status = %s, want failed", result.Status)
	}
}
