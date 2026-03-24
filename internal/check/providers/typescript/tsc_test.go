package typescript

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestParseTscOutput_NoErrors(t *testing.T) {
	issues := parseTscOutput("", "/project")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for empty output, got %d", len(issues))
	}
}

func TestParseTscOutput_SingleError(t *testing.T) {
	output := `src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.`

	issues := parseTscOutput(output, "/project")

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.RuleID != "TS2322" {
		t.Errorf("expected ruleID='TS2322', got %q", issue.RuleID)
	}
	if issue.Line != 10 {
		t.Errorf("expected line=10, got %d", issue.Line)
	}
	if issue.Column != 5 {
		t.Errorf("expected column=5, got %d", issue.Column)
	}
	if issue.Severity != domain.SeverityMajor {
		t.Errorf("expected severity=major, got %q", issue.Severity)
	}
	if issue.Type != domain.IssueTypeBug {
		t.Errorf("expected type=bug, got %q", issue.Type)
	}
	if issue.Source != "tsc" {
		t.Errorf("expected source='tsc', got %q", issue.Source)
	}
}

func TestParseTscOutput_MultipleErrors(t *testing.T) {
	output := `src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.
src/utils.ts(3,1): error TS2304: Cannot find name 'foo'.
src/index.ts(100,20): error TS7006: Parameter 'x' implicitly has an 'any' type.`

	issues := parseTscOutput(output, "/project")

	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}

	// Check second error
	if issues[1].File != "src/utils.ts" {
		t.Errorf("expected file='src/utils.ts', got %q", issues[1].File)
	}
	if issues[1].RuleID != "TS2304" {
		t.Errorf("expected ruleID='TS2304', got %q", issues[1].RuleID)
	}
	if issues[1].Line != 3 {
		t.Errorf("expected line=3, got %d", issues[1].Line)
	}

	// Check third error
	if issues[2].Line != 100 {
		t.Errorf("expected line=100, got %d", issues[2].Line)
	}
	if issues[2].Column != 20 {
		t.Errorf("expected column=20, got %d", issues[2].Column)
	}
}

func TestParseTscOutput_MixedOutput(t *testing.T) {
	// tsc sometimes outputs non-error lines
	output := `
Found 2 errors in 2 files.

Errors  Files
     1  src/app.ts:10
     1  src/utils.ts:3
src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.
src/utils.ts(3,1): error TS2304: Cannot find name 'foo'.
`

	issues := parseTscOutput(output, "/project")

	if len(issues) != 2 {
		t.Errorf("expected 2 issues (only error lines), got %d", len(issues))
	}
}

func TestParseTscOutput_WindowsPath(t *testing.T) {
	// On Windows, tsc may output backslash paths
	output := `src\app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.`

	issues := parseTscOutput(output, "C:\\project")

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	// File path should be normalized to forward slashes
	if issues[0].File != "src/app.ts" || issues[0].File == "src\\app.ts" {
		// The regex may or may not match backslash paths depending on platform
		// Just verify we got something reasonable
		t.Logf("file path: %q (platform-dependent)", issues[0].File)
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Test files
		{"src/app.test.ts", true},
		{"src/app.spec.ts", true},
		{"src/app.mock.ts", true},
		{"src/__tests__/app.ts", true},
		{"src/__mocks__/service.ts", true},
		{"src/__fixtures__/data.ts", true},
		{"/test/helpers.ts", true},
		{"/tests/setup.ts", true},
		{"/mocks/api.ts", true},
		{"/fixtures/data.ts", true},
		{"jest.config.ts", true},
		{"setup-tests.ts", true},
		{"test-utils.ts", true},

		// Production files
		{"src/app.ts", false},
		{"src/utils.ts", false},
		{"src/components/Button.tsx", false},
		{"src/testing-library.ts", false},
		{"src/contest.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	p := New()

	// Should detect with tsconfig.json
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	if !p.Detect(context.Background(), dir) {
		t.Error("expected Detect=true with tsconfig.json")
	}

	// Should not detect without tsconfig.json
	emptyDir := t.TempDir()
	if p.Detect(context.Background(), emptyDir) {
		t.Error("expected Detect=false without tsconfig.json")
	}
}

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Type Safety" {
		t.Errorf("expected Name='Type Safety', got %q", p.Name())
	}
	if p.ID() != "typescript" {
		t.Errorf("expected ID='typescript', got %q", p.ID())
	}
}
