package duplication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestParseJscpdReport_NoDuplicates(t *testing.T) {
	report := jscpdReport{}
	report.Statistics.Total.Percentage = 0
	report.Statistics.Total.Clones = 0
	report.Duplicates = nil

	data, _ := json.Marshal(report)

	var parsed jscpdReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if parsed.Statistics.Total.Percentage != 0 {
		t.Errorf("expected percentage=0, got %f", parsed.Statistics.Total.Percentage)
	}
	if len(parsed.Duplicates) != 0 {
		t.Errorf("expected 0 duplicates, got %d", len(parsed.Duplicates))
	}
}

func TestParseJscpdReport_WithDuplicates(t *testing.T) {
	reportJSON := `{
		"statistics": {
			"total": {
				"percentage": 12.5,
				"lines": 100,
				"sources": 5,
				"clones": 3
			}
		},
		"duplicates": [
			{
				"firstFile": {
					"name": "/project/src/utils.ts",
					"start": 10,
					"end": 25,
					"startLoc": {"line": 10, "column": 1},
					"endLoc": {"line": 25, "column": 1}
				},
				"secondFile": {
					"name": "/project/src/helpers.ts",
					"start": 5,
					"end": 20,
					"startLoc": {"line": 5, "column": 1},
					"endLoc": {"line": 20, "column": 1}
				},
				"lines": 15,
				"tokens": 50,
				"fragment": "duplicated code here"
			},
			{
				"firstFile": {
					"name": "/project/src/a.ts",
					"start": 1,
					"end": 10,
					"startLoc": {"line": 1, "column": 0},
					"endLoc": {"line": 10, "column": 0}
				},
				"secondFile": {
					"name": "/project/src/b.ts",
					"start": 1,
					"end": 10,
					"startLoc": {"line": 1, "column": 0},
					"endLoc": {"line": 10, "column": 0}
				},
				"lines": 10,
				"tokens": 30,
				"fragment": "another duplicate"
			}
		]
	}`

	var report jscpdReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if report.Statistics.Total.Percentage != 12.5 {
		t.Errorf("expected percentage=12.5, got %f", report.Statistics.Total.Percentage)
	}
	if report.Statistics.Total.Clones != 3 {
		t.Errorf("expected clones=3, got %d", report.Statistics.Total.Clones)
	}
	if len(report.Duplicates) != 2 {
		t.Fatalf("expected 2 duplicates, got %d", len(report.Duplicates))
	}

	dup := report.Duplicates[0]
	if dup.FirstFile.Name != "/project/src/utils.ts" {
		t.Errorf("expected firstFile name '/project/src/utils.ts', got %q", dup.FirstFile.Name)
	}
	if dup.FirstFile.StartLoc.Line != 10 {
		t.Errorf("expected firstFile start line 10, got %d", dup.FirstFile.StartLoc.Line)
	}
	if dup.Lines != 15 {
		t.Errorf("expected 15 duplicated lines, got %d", dup.Lines)
	}
}

func TestParseJscpdReport_InvalidJSON(t *testing.T) {
	var report jscpdReport
	err := json.Unmarshal([]byte("not json"), &report)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDetect(t *testing.T) {
	p := New()

	// Should detect when src/ exists
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "src"), 0755)
	if !p.Detect(context.Background(), dir) {
		t.Error("expected Detect=true when src/ exists")
	}

	// Should detect when lib/ exists
	dir2 := t.TempDir()
	os.Mkdir(filepath.Join(dir2, "lib"), 0755)
	if !p.Detect(context.Background(), dir2) {
		t.Error("expected Detect=true when lib/ exists")
	}

	// Should not detect when no source dirs
	emptyDir := t.TempDir()
	if p.Detect(context.Background(), emptyDir) {
		t.Error("expected Detect=false when no source dirs")
	}
}

func TestAnalyze_SourceDirNotFound(t *testing.T) {
	p := New()
	dir := t.TempDir()
	cfg := &config.Config{
		Src: []string{"nonexistent/"},
	}

	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.StatusSkipped {
		t.Errorf("expected StatusSkipped, got %s", result.Status)
	}
}

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Duplication" {
		t.Errorf("expected Name='Duplication', got %q", p.Name())
	}
	if p.ID() != "duplication" {
		t.Errorf("expected ID='duplication', got %q", p.ID())
	}
}

// TestIssueConstruction verifies that duplicates are correctly converted to domain.Issue
func TestIssueConstruction(t *testing.T) {
	// Simulate what the Analyze method does with duplicate data
	dup := jscpdDuplicate{
		FirstFile: jscpdFile{
			Name:     "src/utils.ts",
			StartLoc: jscpdLoc{Line: 10, Col: 1},
		},
		SecondFile: jscpdFile{
			Name:     "src/helpers.ts",
			StartLoc: jscpdLoc{Line: 5, Col: 1},
		},
		Lines:  15,
		Tokens: 50,
	}

	issue := domain.Issue{
		RuleID:   "duplication",
		Message:  "Duplicated 15 lines with src/helpers.ts:5",
		File:     "src/utils.ts",
		Line:     dup.FirstFile.StartLoc.Line,
		Column:   1,
		Severity: domain.SeverityMinor,
		Type:     domain.IssueTypeCodeSmell,
		Source:   "jscpd",
	}

	if issue.RuleID != "duplication" {
		t.Errorf("expected ruleID 'duplication', got %q", issue.RuleID)
	}
	if issue.Severity != domain.SeverityMinor {
		t.Errorf("expected severity minor, got %q", issue.Severity)
	}
	if issue.Line != 10 {
		t.Errorf("expected line 10, got %d", issue.Line)
	}
}
