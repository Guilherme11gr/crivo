package secrets

import (
	"encoding/json"
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
			"Match": "sk_live_abcdefghijklmnopqrst"
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
		{"sk_live_abcdefghij", "sk_l****ghij"},
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

func TestIssueFieldsFromGitleaksResult(t *testing.T) {
	// Verify the issue construction logic
	r := gitleaksResult{
		Description: "AWS Secret Key",
		StartLine:   15,
		StartColumn: 5,
		File:        "src/config.ts",
		RuleID:      "aws-secret-access-key",
		Match:       "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	maskedMatch := maskSecret(r.Match)
	issue := domain.Issue{
		RuleID:   "secret/" + r.RuleID,
		Message:  r.Description + ": " + maskedMatch,
		File:     r.File,
		Line:     r.StartLine,
		Column:   r.StartColumn,
		Severity: domain.SeverityBlocker,
		Type:     domain.IssueTypeVulnerability,
		Source:   "gitleaks",
		Effort:   "15min",
	}

	if issue.RuleID != "secret/aws-secret-access-key" {
		t.Errorf("expected ruleID='secret/aws-secret-access-key', got %q", issue.RuleID)
	}
	if issue.Severity != domain.SeverityBlocker {
		t.Errorf("expected severity=blocker, got %q", issue.Severity)
	}
	if issue.Type != domain.IssueTypeVulnerability {
		t.Errorf("expected type=vulnerability, got %q", issue.Type)
	}
	if issue.Line != 15 {
		t.Errorf("expected line=15, got %d", issue.Line)
	}
	// Verify the match is masked in the message
	if len(issue.Message) > 0 && issue.Message == r.Description+": "+r.Match {
		t.Error("secret should be masked in the message")
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
