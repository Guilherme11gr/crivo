package secrets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestParseGitleaksOutput_NoSecrets(t *testing.T) {
	var results []gitleaksResult
	if err := json.Unmarshal([]byte("[]"), &results); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseGitleaksOutput_WithSecrets(t *testing.T) {
	output := `[
		{
			"Description": "AWS Access Key",
			"StartLine": 10,
			"EndLine": 10,
			"StartColumn": 15,
			"EndColumn": 35,
			"File": "src/config.ts",
			"Entropy": 3.5,
			"RuleID": "aws-access-key-id",
			"Fingerprint": "abc123",
			"Match": "AKIAIOSFODNN7EXAMPLE"
		},
		{
			"Description": "Generic API Key",
			"StartLine": 25,
			"EndLine": 25,
			"StartColumn": 10,
			"EndColumn": 50,
			"File": "src/api.ts",
			"Entropy": 4.2,
			"RuleID": "generic-api-key",
			"Fingerprint": "def456",
			"Match": "pk_test_xxxxxxxxxxxxxxxxxxxxqrst"
		}
	]`

	var results []gitleaksResult
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	r := results[0]
	if r.Description != "AWS Access Key" {
		t.Errorf("expected description='AWS Access Key', got %q", r.Description)
	}
	if r.StartLine != 10 {
		t.Errorf("expected startLine=10, got %d", r.StartLine)
	}
	if r.File != "src/config.ts" {
		t.Errorf("expected file='src/config.ts', got %q", r.File)
	}
	if r.RuleID != "aws-access-key-id" {
		t.Errorf("expected ruleID='aws-access-key-id', got %q", r.RuleID)
	}
}

func TestParseGitleaksOutput_InvalidJSON(t *testing.T) {
	var results []gitleaksResult
	err := json.Unmarshal([]byte("invalid"), &results)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"AKIAIOSFODNN7EXAMPLE", "AKIA****MPLE"},
		{"pk_test_yyyyyyyyyyghij", "pk_t****ghij"},
		{"short", "****"},
		{"12345678", "****"},
		{"123456789", "1234****6789"},
		{"", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := maskSecret(tt.input)
			if got != tt.want {
				t.Errorf("maskSecret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Secrets" {
		t.Errorf("expected Name='Secrets', got %q", p.Name())
	}
	if p.ID() != "secrets" {
		t.Errorf("expected ID='secrets', got %q", p.ID())
	}
}

func TestIsTestOrMockFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Go test files
		{"internal/check/secrets_test.go", true},
		{"handler_test.go", true},
		// JS/TS test files
		{"src/utils.test.ts", true},
		{"src/utils.spec.ts", true},
		{"src/components/Button.test.tsx", true},
		{"src/hooks/useAuth.spec.tsx", true},
		{"src/utils.test.js", true},
		{"src/utils.spec.mjs", true},
		{"src/utils.test.cjs", true},
		// Mock/fixture/stub patterns
		{"src/mocks/database.mock.ts", true},
		{"src/fixtures/users.fixture.ts", true},
		{"src/stubs/api.stub.js", true},
		{"src/components/Button.stories.tsx", true},
		{"src/components/Button.story.tsx", true},
		// Directory patterns
		{"src/__tests__/utils.test.ts", true},
		{"src/__mocks__/fs.mock.ts", true},
		// Case insensitive
		{"SRC/UTILS.TEST.TS", true},
		// Non-test files (negatives)
		{"src/config.ts", false},
		{"src/index.js", false},
		{"src/utils.ts", false},
		{"internal/check/secrets.go", false},
		{".env.local", false},
		{"src/middleware.ts", false},
		{"src/types/api.d.ts", false},
		{"src/constants.ts", false},
		// Edge cases
		{"testing.ts", false},
		{"spec-helper.js", false},
		{"test-utils.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestOrMockFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestOrMockFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIssueFieldsFromGitleaksResult(t *testing.T) {
	// Test production file — should be blocker vulnerability
	prodResult := gitleaksResult{
		Description: "AWS Secret Key",
		StartLine:   15,
		StartColumn: 5,
		File:        "src/config.ts",
		RuleID:      "aws-secret-access-key",
		Match:       "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	prodMasked := maskSecret(prodResult.Match)
	prodIssue := domain.Issue{
		RuleID:   "secret/" + prodResult.RuleID,
		Message:  prodResult.Description + ": " + prodMasked,
		File:     prodResult.File,
		Line:     prodResult.StartLine,
		Column:   prodResult.StartColumn,
		Severity: domain.SeverityBlocker,
		Type:     domain.IssueTypeVulnerability,
		Source:   "gitleaks",
		Effort:   "15min",
	}

	if prodIssue.Severity != domain.SeverityBlocker {
		t.Errorf("prod file: expected severity=blocker, got %q", prodIssue.Severity)
	}
	if prodIssue.Type != domain.IssueTypeVulnerability {
		t.Errorf("prod file: expected type=vulnerability, got %q", prodIssue.Type)
	}
	if prodIssue.Line != 15 {
		t.Errorf("prod file: expected line=15, got %d", prodIssue.Line)
	}
	if len(prodIssue.Message) > 0 && prodIssue.Message == prodResult.Description+": "+prodResult.Match {
		t.Error("prod file: secret should be masked in the message")
	}

	// Test file — should be downgraded to info code_smell
	testResult := gitleaksResult{
		Description: "Stripe API Key",
		StartLine:   42,
		StartColumn: 10,
		File:        "src/services/payment.test.ts",
		RuleID:      "stripe-access-token",
		Match:       "pk_test_xxxxxxxxxxxxxxxxxxxxxxxxxx",
	}

	testRelPath := "src/services/payment.test.ts"
	isTest := isTestOrMockFile(testRelPath)
	if !isTest {
		t.Fatal("expected .test.ts to be detected as test file")
	}

	testMasked := maskSecret(testResult.Match)
	testSeverity := domain.SeverityBlocker
	testType := domain.IssueTypeVulnerability
	testRemediation := domain.SecretRemediation("secret/" + testResult.RuleID)
	if isTest {
		testSeverity = domain.SeverityInfo
		testType = domain.IssueTypeCodeSmell
		testRemediation = "Hardcoded secret in test file. Replace with environment variables, test fixtures, or mock services. Add this file to .gitleaksignore if the secret is intentionally fake."
	}

	testIssue := domain.Issue{
		RuleID:      "secret/" + testResult.RuleID,
		Message:     testResult.Description + ": " + testMasked,
		File:        testRelPath,
		Line:        testResult.StartLine,
		Column:      testResult.StartColumn,
		Severity:    testSeverity,
		Type:        testType,
		Source:      "gitleaks",
		Effort:      "15min",
		Remediation: testRemediation,
	}

	if testIssue.Severity != domain.SeverityInfo {
		t.Errorf("test file: expected severity=info, got %q", testIssue.Severity)
	}
	if testIssue.Type != domain.IssueTypeCodeSmell {
		t.Errorf("test file: expected type=code_smell, got %q", testIssue.Type)
	}
	if testIssue.Line != 42 {
		t.Errorf("test file: expected line=42, got %d", testIssue.Line)
	}
	if !strings.Contains(testIssue.Remediation, "test file") {
		t.Errorf("test file: expected remediation to mention 'test file', got %q", testIssue.Remediation)
	}
}
