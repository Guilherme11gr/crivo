package domain

import (
	"strings"
	"testing"
)

func TestTypescriptRemediation(t *testing.T) {
	tests := []struct {
		name    string
		ruleID  string
		want    string
		wantOK  bool // true = known code, false = fallback
	}{
		// Known codes that exist in the map
		{"TS2322 type mismatch", "TS2322", TSRemediation["TS2322"], true},
		{"TS6133 unused var", "TS6133", TSRemediation["TS6133"], true},
		{"TS6196 unused type", "TS6196", TSRemediation["TS6196"], true},
		{"TS2366 missing return", "TS2366", TSRemediation["TS2366"], true},
		{"TS2307 module not found", "TS2307", TSRemediation["TS2307"], true},
		{"TS7006 implicit any", "TS7006", TSRemediation["TS7006"], true},
		{"TS2571 object is possibly null", "TS2571", TSRemediation["TS2571"], true},
		{"TS18046 possibly undefined", "TS18046", TSRemediation["TS18046"], true},
		{"TS2532 object possibly null", "TS2532", TSRemediation["TS2532"], true},
		{"TS2554 wrong arguments", "TS2554", TSRemediation["TS2554"], true},
		{"TS2345 argument type mismatch", "TS2345", TSRemediation["TS2345"], true},
		{"TS2741 missing property", "TS2741", TSRemediation["TS2741"], true},
		{"TS1005 syntax error", "TS1005", TSRemediation["TS1005"], true},
		{"TS1109 extra closing paren", "TS1109", TSRemediation["TS1109"], true},
		{"TS2304 cannot find name", "TS2304", TSRemediation["TS2304"], true},
		{"TS1488 duplicate declaration", "TS1488", TSRemediation["TS1488"], true},
		{"TS2694 namespace readonly", "TS2694", TSRemediation["TS2694"], true},
		{"TS2667 predicate narrowing", "TS2667", TSRemediation["TS2667"], true},
		{"TS2416 class property mismatch", "TS2416", TSRemediation["TS2416"], true},
		{"TS2612 parameter overwrite", "TS2612", TSRemediation["TS2612"], true},

		// Edge cases
		{"Empty ruleID", "", "Fix the TypeScript error : check types, imports, and syntax at the reported location", false},
		{"Random string", "FOOBAR123", "Fix the TypeScript error FOOBAR123: check types, imports, and syntax at the reported location", false},
		{"Unknown TS code", "TS9999", "Fix the TypeScript error TS9999: check types, imports, and syntax at the reported location", false},
		{"TS code with extra info", "TS2322(some extra)", "Fix the TypeScript error TS2322(some extra): check types, imports, and syntax at the reported location", false},
		{"Lowercase ts", "ts2322", TSRemediation["TS2322"], true}, // the func adds TS prefix back
		{"Just number resolves", "2322", TSRemediation["TS2322"], true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TypescriptRemediation(tt.ruleID)
			if got != tt.want {
				t.Errorf("TypescriptRemediation(%q) = %q, want %q", tt.ruleID, got, tt.want)
			}
			// Verify no empty remediation for known codes
			if tt.wantOK && got == "" {
				t.Errorf("TypescriptRemediation(%q) returned empty for known code", tt.ruleID)
			}
		})
	}
}

func TestSecretRemediation(t *testing.T) {
	tests := []struct {
		name   string
		ruleID string
		wantOK bool // true = specific hint, false = generic fallback
	}{
		// Known secret types
		{"Stripe", "secret/stripe-access-token", true},
		{"AWS access", "secret/aws-access-token", true},
		{"AWS secret", "secret/aws-secret-key", true},
		{"GitHub token", "secret/github-token", true},
		{"Private key", "secret/private-key", true},
		{"Slack token", "secret/slack-token", true},
		{"Slack webhook", "secret/slack-webhook", true},
		{"Google API key", "secret/google-api-key", true},
		{"JWT", "secret/jwt", true},
		{"Mailgun", "secret/mailgun-api-key", true},
		{"SendGrid", "secret/sendgrid-api-key", true},
		{"Twilio", "secret/twilio-api-key", true},
		{"Heroku", "secret/heroku-api-key", true},
		{"Azure", "secret/azure-storage-key", true},
		{"Generic API key", "secret/generic-api-key", true},

		// Edge cases
		{"Empty ruleID", "secret/", false},
		{"No prefix", "stripe-access-token", true},       // partial match still works
		{"Unknown secret", "secret/unknown-secret-type", false},
		{"Completely random", "random-rule", false},
		{"Empty string", "", false},
		{"Partial match in unknown", "secret/my-stripe-access-token-custom", true}, // contains "stripe-access-token"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SecretRemediation(tt.ruleID)
			if got == "" {
				t.Errorf("SecretRemediation(%q) returned empty string", tt.ruleID)
			}
			if tt.wantOK {
				// Specific hint should NOT be the generic fallback
				if strings.HasPrefix(got, "Move this secret to an environment variable or secret manager, add the file to .gitignore") {
					t.Errorf("SecretRemediation(%q) returned generic fallback instead of specific hint: %s", tt.ruleID, got)
				}
			} else {
				// Should be the generic fallback
				if !strings.HasPrefix(got, "Move this secret to an environment variable or secret manager") {
					t.Errorf("SecretRemediation(%q) = %q, expected generic fallback", tt.ruleID, got)
				}
			}
		})
	}
}

func TestDeadcodeRemediation(t *testing.T) {
	tests := []struct {
		name    string
		ruleID  string
		message string
		wantOK  bool
	}{
		{"Unused file", "unused-file", "File is not imported anywhere", true},
		{"Unused export", "unused-export", "Export is not used", true},
		{"Unused type", "unused-type", "Type export is not used", true},
		{"Unused dependency with pkg", "unused-dependency", "Dependency is not imported: lodash", true},
		{"Unused dependency without prefix", "unused-dependency", "some other message", true},
		{"Unknown rule", "some-unknown-rule", "some message", true}, // default case
		{"Empty ruleID", "", "some message", true},                 // default case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeadcodeRemediation(tt.ruleID, tt.message)
			if got == "" {
				t.Errorf("DeadcodeRemediation(%q, %q) returned empty string", tt.ruleID, tt.message)
			}
			// Unused dependency with known format should include the package name
			if tt.ruleID == "unused-dependency" && strings.HasPrefix(tt.message, "Dependency is not imported:") {
				if !strings.Contains(got, "lodash") {
					t.Errorf("DeadcodeRemediation for unused-dependency should include package name, got: %s", got)
				}
				if !strings.Contains(got, "npm uninstall") {
					t.Errorf("DeadcodeRemediation for unused-dependency should include 'npm uninstall', got: %s", got)
				}
			}
		})
	}
}

func TestComplexityRemediation(t *testing.T) {
	got := ComplexityRemediation("")
	if got == "" {
		t.Error("ComplexityRemediation returned empty string")
	}
	if !strings.Contains(got, "extract helper functions") {
		t.Errorf("ComplexityRemediation missing key suggestion, got: %s", got)
	}
	if !strings.Contains(got, "guard clauses") {
		t.Errorf("ComplexityRemediation missing 'guard clauses', got: %s", got)
	}
}

func TestDuplicationRemediation(t *testing.T) {
	tests := []struct {
		ruleID string
		want   string
	}{
		{"duplication", "shared utility function"},
		{"semantic-duplication", "structurally similar"},
		{"unknown-dup-type", "shared function or utility module"},
		{"", "shared function or utility module"},
	}
	for _, tt := range tests {
		got := DuplicationRemediation(tt.ruleID)
		if !strings.Contains(got, tt.want) {
			t.Errorf("DuplicationRemediation(%q) = %q, want to contain %q", tt.ruleID, got, tt.want)
		}
	}
}

func TestCoverageRemediation(t *testing.T) {
	got := CoverageRemediation("")
	if got == "" {
		t.Error("CoverageRemediation returned empty string")
	}
	if !strings.Contains(got, "Add tests") {
		t.Errorf("CoverageRemediation missing 'Add tests', got: %s", got)
	}
}

func TestSemgrepRemediation(t *testing.T) {
	tests := []struct {
		name   string
		ruleID string
		wantOK bool // true = specific CWE hint
	}{
		// Known CWEs
		{"SQL Injection", "javascript.lang.security.audit.sqli [CWE-89]", true},
		{"XSS", "javascript.lang.security.audit.xss [CWE-79]", true},
		{"Path Traversal", "python.lang.security.audit.path-traversal [CWE-22]", true},
		{"Command Injection", "javascript.lang.security.audit.command-injection [CWE-78]", true},
		{"Hardcoded Secret", "javascript.lang.best-practice.hardcoded-secret [CWE-798]", true},
		{"SSRF", "javascript.lang.security.audit.ssrf [CWE-918]", true},
		{"Code Injection", "javascript.lang.security.audit.code-injection [CWE-94]", true},

		// Unknown CWE - should still have CWE-specific fallback
		{"Unknown CWE", "some.rule [CWE-999]", false},
		// No CWE at all - generic fallback
		{"No CWE", "some.rule.without.cwe", false},
		// Empty string
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SemgrepRemediation(tt.ruleID)
			if got == "" {
				t.Errorf("SemgrepRemediation(%q) returned empty string", tt.ruleID)
			}

			if tt.wantOK {
				// Should NOT be the generic "Review the flagged code" fallback
				if strings.HasPrefix(got, "Review the flagged code for security issues") {
					t.Errorf("SemgrepRemediation(%q) returned generic fallback instead of CWE hint: %s", tt.ruleID, got)
				}
			}

			// Unknown CWE should still mention the CWE number
			if tt.name == "Unknown CWE" {
				if !strings.Contains(got, "CWE-999") {
					t.Errorf("SemgrepRemediation for unknown CWE should mention CWE number, got: %s", got)
				}
			}
		})
	}
}

func TestCustomRuleRemediation(t *testing.T) {
	tests := []struct {
		ruleType string
		want     string
	}{
		{"ban-import", "Remove the banned import"},
		{"ban-pattern", "Replace the matched pattern"},
		{"require-import", "Add the required import"},
		{"enforce-pattern", "Add the required pattern"},
		{"max-lines", "Split this file"},
		{"ban-dependency", "npm uninstall"},
		{"semgrep", "semgrep rule requirement"},
		{"advisory", "Review the advisory"},
		{"unknown-type", "Review and fix"},  // default
		{"", "Review and fix"},              // default
	}
	for _, tt := range tests {
		got := CustomRuleRemediation(tt.ruleType, "test message")
		if !strings.Contains(got, tt.want) {
			t.Errorf("CustomRuleRemediation(%q) = %q, want to contain %q", tt.ruleType, got, tt.want)
		}
	}
}

// --- Integration-style: verify NO function ever returns empty string ---

func TestNoEmptyRemediations(t *testing.T) {
	// This is the critical test: ensure every remediation function
	// ALWAYS returns a non-empty string, regardless of input.

	type testCase struct {
		fn   func() string
		name string
	}
	cases := []testCase{
		// TypeScript
		{name: "TS known", fn: func() string { return TypescriptRemediation("TS2322") }},
		{name: "TS unknown", fn: func() string { return TypescriptRemediation("TS99999") }},
		{name: "TS empty", fn: func() string { return TypescriptRemediation("") }},
		{name: "TS garbage", fn: func() string { return TypescriptRemediation("NOT_EVEN_A_CODE!!!") }},

		// Secrets
		{name: "Secret known", fn: func() string { return SecretRemediation("secret/stripe-access-token") }},
		{name: "Secret unknown", fn: func() string { return SecretRemediation("secret/foobar") }},
		{name: "Secret empty", fn: func() string { return SecretRemediation("") }},
		{name: "Secret no prefix", fn: func() string { return SecretRemediation("random-string") }},

		// Dead code
		{name: "Deadcode file", fn: func() string { return DeadcodeRemediation("unused-file", "msg") }},
		{name: "Deadcode export", fn: func() string { return DeadcodeRemediation("unused-export", "msg") }},
		{name: "Deadcode dep", fn: func() string { return DeadcodeRemediation("unused-dependency", "msg") }},
		{name: "Deadcode unknown", fn: func() string { return DeadcodeRemediation("x", "y") }},
		{name: "Deadcode empty", fn: func() string { return DeadcodeRemediation("", "") }},

		// Complexity
		{name: "Complexity", fn: func() string { return ComplexityRemediation("") }},

		// Duplication
		{name: "Dup text", fn: func() string { return DuplicationRemediation("duplication") }},
		{name: "Dup semantic", fn: func() string { return DuplicationRemediation("semantic-duplication") }},
		{name: "Dup unknown", fn: func() string { return DuplicationRemediation("x") }},
		{name: "Dup empty", fn: func() string { return DuplicationRemediation("") }},

		// Coverage
		{name: "Coverage", fn: func() string { return CoverageRemediation("") }},

		// Semgrep
		{name: "Semgrep CWE-89", fn: func() string { return SemgrepRemediation("x [CWE-89]") }},
		{name: "Semgrep unknown CWE", fn: func() string { return SemgrepRemediation("x [CWE-999]") }},
		{name: "Semgrep no CWE", fn: func() string { return SemgrepRemediation("no-cwe-here") }},
		{name: "Semgrep empty", fn: func() string { return SemgrepRemediation("") }},

		// Custom rules
		{name: "Custom ban-import", fn: func() string { return CustomRuleRemediation("ban-import", "msg") }},
		{name: "Custom unknown", fn: func() string { return CustomRuleRemediation("x", "msg") }},
		{name: "Custom empty", fn: func() string { return CustomRuleRemediation("", "msg") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn()
			if got == "" {
				t.Errorf("remediation function %s returned empty string", tc.name)
			}
			if len(strings.TrimSpace(got)) == 0 {
				t.Errorf("remediation function %s returned whitespace-only string", tc.name)
			}
		})
	}
}
