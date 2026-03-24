package eslint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestParseEslintOutput_NoIssues(t *testing.T) {
	output := `[{"filePath":"/project/src/app.ts","messages":[]}]`

	var results []eslintResult
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(results[0].Messages))
	}
}

func TestParseEslintOutput_WithErrors(t *testing.T) {
	ruleID := "no-unused-vars"
	output := `[{
		"filePath": "/project/src/app.ts",
		"messages": [
			{
				"ruleId": "no-unused-vars",
				"severity": 2,
				"message": "'x' is defined but never used",
				"line": 5,
				"column": 7
			},
			{
				"ruleId": "semi",
				"severity": 1,
				"message": "Missing semicolon",
				"line": 10,
				"column": 20
			}
		]
	}]`

	var results []eslintResult
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(results[0].Messages))
	}

	msg := results[0].Messages[0]
	if *msg.RuleID != ruleID {
		t.Errorf("expected ruleId=%q, got %q", ruleID, *msg.RuleID)
	}
	if msg.Severity != 2 {
		t.Errorf("expected severity=2, got %d", msg.Severity)
	}
	if msg.Line != 5 {
		t.Errorf("expected line=5, got %d", msg.Line)
	}
	if msg.Column != 7 {
		t.Errorf("expected column=7, got %d", msg.Column)
	}
}

func TestParseEslintOutput_NilRuleID(t *testing.T) {
	// Messages without ruleId (e.g., parser errors) should be handled
	output := `[{
		"filePath": "/project/src/broken.ts",
		"messages": [
			{
				"ruleId": null,
				"severity": 2,
				"message": "Parsing error: unexpected token",
				"line": 1,
				"column": 1
			}
		]
	}]`

	var results []eslintResult
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	msg := results[0].Messages[0]
	if msg.RuleID != nil {
		t.Errorf("expected ruleId=nil, got %q", *msg.RuleID)
	}
}

func TestParseEslintOutput_InvalidJSON(t *testing.T) {
	var results []eslintResult
	err := json.Unmarshal([]byte("not json at all"), &results)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseEslintOutput_EmptyArray(t *testing.T) {
	var results []eslintResult
	if err := json.Unmarshal([]byte("[]"), &results); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestIsBugRule(t *testing.T) {
	tests := []struct {
		ruleID string
		want   bool
	}{
		{"sonarjs/no-all-duplicated-branches", true},
		{"sonarjs/no-identical-conditions", true},
		{"sonarjs/no-element-overwrite", true},
		{"no-unused-vars", false},
		{"semi", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			got := isBugRule(tt.ruleID)
			if got != tt.want {
				t.Errorf("isBugRule(%q) = %v, want %v", tt.ruleID, got, tt.want)
			}
		})
	}
}

func TestIsSecurityRule(t *testing.T) {
	tests := []struct {
		ruleID string
		want   bool
	}{
		{"security/detect-eval-with-expression", true},
		{"security/detect-child-process", true},
		{"security/detect-possible-timing-attacks", true},
		{"no-eval", false},
		{"semi", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			got := isSecurityRule(tt.ruleID)
			if got != tt.want {
				t.Errorf("isSecurityRule(%q) = %v, want %v", tt.ruleID, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is to..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		parts []string
		sep   string
		want  string
	}{
		{[]string{"a", "b", "c"}, " · ", "a · b · c"},
		{[]string{"single"}, ", ", "single"},
		{[]string{}, ", ", ""},
	}

	for _, tt := range tests {
		got := join(tt.parts, tt.sep)
		if got != tt.want {
			t.Errorf("join(%v, %q) = %q, want %q", tt.parts, tt.sep, got, tt.want)
		}
	}
}

func TestSeverityMapping(t *testing.T) {
	// Verify the severity/type mapping logic used in Analyze
	// Severity 1 (warning) => SeverityMinor, IssueTypeCodeSmell
	// Severity 2 (error) => SeverityMajor, IssueTypeCodeSmell (default)
	// Severity 2 + bug rule => SeverityCritical, IssueTypeBug
	// Severity 2 + security rule => SeverityCritical, IssueTypeVulnerability

	type testCase struct {
		name         string
		eslintSev    int
		ruleID       string
		wantSeverity domain.Severity
		wantType     domain.IssueType
	}

	cases := []testCase{
		{"warning", 1, "semi", domain.SeverityMinor, domain.IssueTypeCodeSmell},
		{"error - normal", 2, "no-unused-vars", domain.SeverityMajor, domain.IssueTypeCodeSmell},
		{"error - bug rule", 2, "sonarjs/no-identical-conditions", domain.SeverityCritical, domain.IssueTypeBug},
		{"error - security rule", 2, "security/detect-eval-with-expression", domain.SeverityCritical, domain.IssueTypeVulnerability},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			severity := domain.SeverityMinor
			issueType := domain.IssueTypeCodeSmell

			if tc.eslintSev == 2 {
				severity = domain.SeverityMajor
				if isBugRule(tc.ruleID) {
					issueType = domain.IssueTypeBug
					severity = domain.SeverityCritical
				} else if isSecurityRule(tc.ruleID) {
					issueType = domain.IssueTypeVulnerability
					severity = domain.SeverityCritical
				}
			}

			if severity != tc.wantSeverity {
				t.Errorf("severity = %q, want %q", severity, tc.wantSeverity)
			}
			if issueType != tc.wantType {
				t.Errorf("type = %q, want %q", issueType, tc.wantType)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	p := New()

	// Should detect eslint config file
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "eslint.config.js"), []byte("module.exports = {};"), 0644)
	if !p.Detect(context.Background(), dir) {
		t.Error("expected Detect=true with eslint.config.js")
	}

	// Should detect .eslintrc.json
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, ".eslintrc.json"), []byte("{}"), 0644)
	if !p.Detect(context.Background(), dir2) {
		t.Error("expected Detect=true with .eslintrc.json")
	}

	// Should detect eslintConfig in package.json
	dir3 := t.TempDir()
	os.WriteFile(filepath.Join(dir3, "package.json"), []byte(`{"eslintConfig":{}}`), 0644)
	if !p.Detect(context.Background(), dir3) {
		t.Error("expected Detect=true with eslintConfig in package.json")
	}

	// Should not detect when nothing exists
	emptyDir := t.TempDir()
	if p.Detect(context.Background(), emptyDir) {
		t.Error("expected Detect=false with no eslint config")
	}
}

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Code Quality" {
		t.Errorf("expected Name='Code Quality', got %q", p.Name())
	}
	if p.ID() != "eslint" {
		t.Errorf("expected ID='eslint', got %q", p.ID())
	}
}
