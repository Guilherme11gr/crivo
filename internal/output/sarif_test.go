package output

import (
	"encoding/json"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestToSARIF(t *testing.T) {
	result := &domain.AnalysisResult{
		Checks: []domain.CheckResult{
			{
				Issues: []domain.Issue{
					{
						RuleID:   "TS2322",
						Message:  "Type error",
						File:     "src/foo.ts",
						Line:     10,
						Column:   5,
						Severity: domain.SeverityMajor,
						Type:     domain.IssueTypeBug,
						Source:   "tsc",
					},
				},
			},
		},
	}

	data, err := ToSARIF(result)
	if err != nil {
		t.Fatalf("ToSARIF: %v", err)
	}

	// Validate it's valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Check SARIF version
	if v, ok := parsed["version"]; !ok || v != "2.1.0" {
		t.Errorf("SARIF version = %v, want 2.1.0", v)
	}

	// Check runs
	runs, ok := parsed["runs"].([]any)
	if !ok || len(runs) == 0 {
		t.Fatal("Expected at least 1 run")
	}

	run := runs[0].(map[string]any)
	results, ok := run["results"].([]any)
	if !ok || len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}
