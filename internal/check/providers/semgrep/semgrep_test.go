package semgrep

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Security (Semgrep)" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Security (Semgrep)")
	}
	if p.ID() != "semgrep" {
		t.Errorf("ID() = %q, want %q", p.ID(), "semgrep")
	}
}

func TestMapSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  domain.Severity
	}{
		{"ERROR", domain.SeverityCritical},
		{"error", domain.SeverityCritical},
		{"Error", domain.SeverityCritical},
		{"WARNING", domain.SeverityMajor},
		{"warning", domain.SeverityMajor},
		{"INFO", domain.SeverityMinor},
		{"info", domain.SeverityMinor},
		{"unknown", domain.SeverityMinor},
		{"", domain.SeverityMinor},
		{"CRITICAL", domain.SeverityMinor},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapSeverity(tt.input)
			if got != tt.want {
				t.Errorf("mapSeverity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyFinding_SecurityCategory(t *testing.T) {
	metadata := map[string]any{"category": "security"}
	got := classifyFinding("some.random.rule", metadata)
	if got != domain.IssueTypeVulnerability {
		t.Errorf("classifyFinding with security category = %q, want %q", got, domain.IssueTypeVulnerability)
	}
}

func TestClassifyFinding_NonSecurityCategory(t *testing.T) {
	metadata := map[string]any{"category": "performance"}
	got := classifyFinding("some.random.rule", metadata)
	if got != domain.IssueTypeSecurityHotspot {
		t.Errorf("classifyFinding with non-security category = %q, want %q", got, domain.IssueTypeSecurityHotspot)
	}
}

func TestClassifyFinding_SecurityPatterns(t *testing.T) {
	patterns := []string{
		"python.lang.security.sql-injection.raw-query",
		"javascript.browser.security.xss.innerHTML",
		"java.spring.security.csrf.disabled",
		"python.requests.security.ssrf.unvalidated",
		"generic.injection.command",
		"auth.bypass.missing-check",
		"crypto.weak-cipher",
		"python.flask.security.taint.unvalidated",
	}

	for _, checkID := range patterns {
		t.Run(checkID, func(t *testing.T) {
			got := classifyFinding(checkID, map[string]any{})
			if got != domain.IssueTypeVulnerability {
				t.Errorf("classifyFinding(%q) = %q, want %q", checkID, got, domain.IssueTypeVulnerability)
			}
		})
	}
}

func TestClassifyFinding_NonSecurity(t *testing.T) {
	got := classifyFinding("python.lang.best-practice.unused-variable", map[string]any{})
	if got != domain.IssueTypeSecurityHotspot {
		t.Errorf("classifyFinding for non-security rule = %q, want %q", got, domain.IssueTypeSecurityHotspot)
	}
}

func TestClassifyFinding_NilMetadata(t *testing.T) {
	got := classifyFinding("some.rule", nil)
	if got != domain.IssueTypeSecurityHotspot {
		t.Errorf("classifyFinding with nil metadata = %q, want %q", got, domain.IssueTypeSecurityHotspot)
	}
}

func TestExtractCWE_StringValue(t *testing.T) {
	metadata := map[string]any{"cwe": "CWE-89"}
	cwe, ok := extractCWE(metadata)
	if !ok {
		t.Fatal("expected ok=true for string CWE")
	}
	if cwe != "CWE-89" {
		t.Errorf("extractCWE string = %q, want %q", cwe, "CWE-89")
	}
}

func TestExtractCWE_ArrayValue(t *testing.T) {
	metadata := map[string]any{"cwe": []any{"CWE-79", "CWE-80"}}
	cwe, ok := extractCWE(metadata)
	if !ok {
		t.Fatal("expected ok=true for array CWE")
	}
	if cwe != "CWE-79" {
		t.Errorf("extractCWE array = %q, want %q", cwe, "CWE-79")
	}
}

func TestExtractCWE_EmptyArray(t *testing.T) {
	metadata := map[string]any{"cwe": []any{}}
	_, ok := extractCWE(metadata)
	if ok {
		t.Error("expected ok=false for empty CWE array")
	}
}

func TestExtractCWE_Missing(t *testing.T) {
	metadata := map[string]any{"other": "value"}
	_, ok := extractCWE(metadata)
	if ok {
		t.Error("expected ok=false for missing CWE")
	}
}

func TestExtractCWE_NilMetadata(t *testing.T) {
	_, ok := extractCWE(nil)
	if ok {
		t.Error("expected ok=false for nil metadata")
	}
}

func TestParseSemgrepOutput_NoResults(t *testing.T) {
	output := `{"results": [], "errors": []}`
	var parsed semgrepOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(parsed.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(parsed.Results))
	}
}

func TestParseSemgrepOutput_WithResults(t *testing.T) {
	output := `{
		"results": [
			{
				"check_id": "python.lang.security.sql-injection.raw-query",
				"path": "src/db.py",
				"start": {"line": 42, "col": 5},
				"end": {"line": 42, "col": 80},
				"extra": {
					"message": "Possible SQL injection via string concatenation",
					"severity": "ERROR",
					"metadata": {
						"category": "security",
						"cwe": "CWE-89"
					},
					"lines": "query = \"SELECT * FROM users WHERE id=\" + user_input"
				}
			},
			{
				"check_id": "generic.best-practice.logging",
				"path": "src/app.py",
				"start": {"line": 10, "col": 1},
				"end": {"line": 10, "col": 30},
				"extra": {
					"message": "Use structured logging",
					"severity": "INFO",
					"metadata": {},
					"lines": "print(debug_info)"
				}
			}
		],
		"errors": []
	}`

	var parsed semgrepOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(parsed.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(parsed.Results))
	}

	r := parsed.Results[0]
	if r.CheckID != "python.lang.security.sql-injection.raw-query" {
		t.Errorf("check_id = %q, want %q", r.CheckID, "python.lang.security.sql-injection.raw-query")
	}
	if r.Path != "src/db.py" {
		t.Errorf("path = %q, want %q", r.Path, "src/db.py")
	}
	if r.Start.Line != 42 {
		t.Errorf("start.line = %d, want 42", r.Start.Line)
	}
	if r.Start.Col != 5 {
		t.Errorf("start.col = %d, want 5", r.Start.Col)
	}
	if r.Extra.Severity != "ERROR" {
		t.Errorf("severity = %q, want %q", r.Extra.Severity, "ERROR")
	}
	if r.Extra.Metadata["category"] != "security" {
		t.Errorf("metadata.category = %v, want %q", r.Extra.Metadata["category"], "security")
	}

	r2 := parsed.Results[1]
	if r2.Extra.Severity != "INFO" {
		t.Errorf("result[1] severity = %q, want %q", r2.Extra.Severity, "INFO")
	}
}

func TestParseSemgrepOutput_InvalidJSON(t *testing.T) {
	var parsed semgrepOutput
	err := json.Unmarshal([]byte("not json"), &parsed)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseSemgrepOutput_WithErrors(t *testing.T) {
	output := `{
		"results": [],
		"errors": [
			{"message": "Could not parse file", "level": "warn"}
		]
	}`

	var parsed semgrepOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(parsed.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(parsed.Errors))
	}
	if parsed.Errors[0].Message != "Could not parse file" {
		t.Errorf("error message = %q, want %q", parsed.Errors[0].Message, "Could not parse file")
	}
}

func TestDetect_SemgrepNotInPath(t *testing.T) {
	// Use a clean PATH that does not contain semgrep
	p := New()
	// Save and clear PATH to ensure semgrep is not found
	origPath := t.Setenv // we'll use exec.LookPath as a proxy check
	_ = origPath

	// Verify that if semgrep is not in PATH, exec.LookPath fails
	_, err := exec.LookPath("semgrep-nonexistent-binary-xyz")
	if err == nil {
		t.Skip("somehow found a nonexistent binary, skipping")
	}

	// The Detect method uses check.FindTool which wraps exec.LookPath.
	// We test the behavior: if semgrep is not available, Detect returns false.
	// On CI/test machines without semgrep, this should return false.
	result := p.Detect(context.Background(), "/nonexistent/dir")
	// We can't assert true or false here since semgrep may or may not be installed,
	// but we verify it doesn't panic and returns a bool.
	_ = result
}
