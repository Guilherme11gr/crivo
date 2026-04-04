package coverage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestParseCoverageSummary_ValidJSON(t *testing.T) {
	summaryJSON := `{
		"total": {
			"lines": {"total": 100, "covered": 80, "skipped": 0, "pct": 80.0},
			"branches": {"total": 50, "covered": 40, "skipped": 0, "pct": 80.0},
			"functions": {"total": 20, "covered": 18, "skipped": 0, "pct": 90.0},
			"statements": {"total": 120, "covered": 100, "skipped": 0, "pct": 83.33}
		},
		"/project/src/utils.ts": {
			"lines": {"total": 50, "covered": 45, "skipped": 0, "pct": 90.0},
			"branches": {"total": 10, "covered": 8, "skipped": 0, "pct": 80.0},
			"functions": {"total": 5, "covered": 5, "skipped": 0, "pct": 100.0},
			"statements": {"total": 60, "covered": 55, "skipped": 0, "pct": 91.67}
		},
		"/project/src/service.ts": {
			"lines": {"total": 50, "covered": 35, "skipped": 0, "pct": 70.0},
			"branches": {"total": 40, "covered": 32, "skipped": 0, "pct": 80.0},
			"functions": {"total": 15, "covered": 13, "skipped": 0, "pct": 86.67},
			"statements": {"total": 60, "covered": 45, "skipped": 0, "pct": 75.0}
		}
	}`

	var summary coverageSummary
	if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	total, ok := summary["total"]
	if !ok {
		t.Fatal("expected 'total' key in summary")
	}
	if total.Lines.Pct != 80.0 {
		t.Errorf("expected lines pct=80.0, got %f", total.Lines.Pct)
	}
	if total.Branches.Pct != 80.0 {
		t.Errorf("expected branches pct=80.0, got %f", total.Branches.Pct)
	}
	if total.Functions.Pct != 90.0 {
		t.Errorf("expected functions pct=90.0, got %f", total.Functions.Pct)
	}
	if total.Statements.Pct != 83.33 {
		t.Errorf("expected statements pct=83.33, got %f", total.Statements.Pct)
	}
	if total.Lines.Total != 100 {
		t.Errorf("expected lines total=100, got %d", total.Lines.Total)
	}
	if total.Lines.Covered != 80 {
		t.Errorf("expected lines covered=80, got %d", total.Lines.Covered)
	}

	// Check per-file entries
	if len(summary) != 3 {
		t.Errorf("expected 3 entries (total + 2 files), got %d", len(summary))
	}
}

func TestParseCoverageSummary_EmptyTotal(t *testing.T) {
	summaryJSON := `{
		"total": {
			"lines": {"total": 0, "covered": 0, "skipped": 0, "pct": 0},
			"branches": {"total": 0, "covered": 0, "skipped": 0, "pct": 0},
			"functions": {"total": 0, "covered": 0, "skipped": 0, "pct": 0},
			"statements": {"total": 0, "covered": 0, "skipped": 0, "pct": 0}
		}
	}`

	var summary coverageSummary
	if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	total := summary["total"]
	if total.Lines.Pct != 0 {
		t.Errorf("expected lines pct=0, got %f", total.Lines.Pct)
	}
}

func TestParseCoverageSummary_InvalidJSON(t *testing.T) {
	var summary coverageSummary
	err := json.Unmarshal([]byte("invalid json"), &summary)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSeverityForCoverage(t *testing.T) {
	tests := []struct {
		name      string
		actual    float64
		threshold float64
		want      domain.Severity
	}{
		{"minor - close to threshold", 75.0, 80.0, domain.SeverityMinor},
		{"minor - at 50%+ of threshold", 45.0, 80.0, domain.SeverityMinor},
		{"major - below 50% of threshold", 39.0, 80.0, domain.SeverityMajor},
		{"major - at 25%+ of threshold", 20.0, 80.0, domain.SeverityMajor},
		{"critical - below 25% of threshold", 19.0, 80.0, domain.SeverityCritical},
		{"critical - zero coverage", 0.0, 80.0, domain.SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := severityForCoverage(tt.actual, tt.threshold)
			if got != tt.want {
				t.Errorf("severityForCoverage(%f, %f) = %q, want %q", tt.actual, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestExtractTestFailures(t *testing.T) {
	output := `PASS src/utils.test.ts
FAIL src/service.test.ts
  ● should handle errors
    expect(received).toBe(expected)
FAIL src/auth.test.ts
  ● should authenticate user
  ● should reject invalid tokens
Test Suites: 2 failed, 1 passed, 3 total`

	failures := extractTestFailures(output)

	if len(failures) == 0 {
		t.Fatal("expected some failures extracted")
	}

	// Should contain lines with FAIL or bullet
	hasFailLine := false
	hasBulletLine := false
	for _, f := range failures {
		if len(f) > 0 {
			for _, c := range f {
				if c == 'F' {
					hasFailLine = true
					break
				}
			}
		}
		for _, c := range f {
			if c == '\u25cf' { // ●
				hasBulletLine = true
				break
			}
		}
	}
	if !hasFailLine {
		t.Error("expected at least one line containing FAIL")
	}
	if !hasBulletLine {
		t.Error("expected at least one line containing bullet")
	}
}

func TestExtractTestFailures_TruncatesAt15(t *testing.T) {
	output := ""
	for i := 0; i < 20; i++ {
		output += "FAIL test" + string(rune('A'+i)) + "\n"
	}

	failures := extractTestFailures(output)
	if len(failures) > 15 {
		t.Errorf("expected max 15 failures, got %d", len(failures))
	}
}

func TestExtractTestFailures_Empty(t *testing.T) {
	failures := extractTestFailures("")
	if len(failures) != 0 {
		t.Errorf("expected 0 failures for empty output, got %d", len(failures))
	}
}

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Coverage" {
		t.Errorf("expected Name='Coverage', got %q", p.Name())
	}
	if p.ID() != "coverage" {
		t.Errorf("expected ID='coverage', got %q", p.ID())
	}
}

func TestDetectTestRunner(t *testing.T) {
	tests := []struct {
		name    string
		pkgJSON string
		want    string
	}{
		{
			name:    "vitest in devDependencies",
			pkgJSON: `{"devDependencies":{"vitest":"^1.0.0"}}`,
			want:    "vitest",
		},
		{
			name:    "jest in devDependencies",
			pkgJSON: `{"devDependencies":{"jest":"^29.0.0"}}`,
			want:    "jest",
		},
		{
			name:    "vitest takes priority over jest",
			pkgJSON: `{"devDependencies":{"vitest":"^1.0.0","jest":"^29.0.0"}}`,
			want:    "vitest",
		},
		{
			name:    "vitest in dependencies (not devDeps)",
			pkgJSON: `{"dependencies":{"vitest":"^1.0.0"}}`,
			want:    "vitest",
		},
		{
			name:    "vitest in test script",
			pkgJSON: `{"scripts":{"test":"vitest run"},"devDependencies":{}}`,
			want:    "vitest",
		},
		{
			name:    "jest in test script",
			pkgJSON: `{"scripts":{"test":"jest --coverage"},"devDependencies":{}}`,
			want:    "jest",
		},
		{
			name:    "no test runner detected",
			pkgJSON: `{"scripts":{"build":"tsc"},"dependencies":{}}`,
			want:    "",
		},
		{
			name:    "no package.json",
			pkgJSON: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.pkgJSON != "" {
				dir = t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tt.pkgJSON), 0644); err != nil {
					t.Fatal(err)
				}
			} else {
				dir = t.TempDir() // exists but no package.json
			}

			got := detectTestRunner(dir)
			if got != tt.want {
				t.Errorf("detectTestRunner() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTestRunnerMetric(t *testing.T) {
	if testRunnerMetric("vitest") != 1 {
		t.Error("expected vitest = 1")
	}
	if testRunnerMetric("jest") != 2 {
		t.Error("expected jest = 2")
	}
	if testRunnerMetric("unknown") != 0 {
		t.Error("expected unknown = 0")
	}
	if testRunnerMetric("") != 0 {
		t.Error("expected empty = 0")
	}
}
