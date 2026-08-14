package secrets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/domain"
	gitutil "github.com/guilherme11gr/crivo/internal/git"
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

func TestGitleaksTargets_UsesChangedFilesScope(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "src"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "src", "secret.ts"), []byte("const x = 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx := check.WithNewCodeScope(context.Background(), check.NewScope(
		[]gitutil.ChangedFile{{Path: "src/secret.ts"}, {Path: "missing.ts"}},
		nil,
	))

	targets := gitleaksTargets(ctx, projectDir)
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d: %#v", len(targets), targets)
	}
	if !strings.HasSuffix(filepath.ToSlash(targets[0]), "src/secret.ts") {
		t.Fatalf("unexpected target %q", targets[0])
	}
}

// ─── Fail-closed: empty scope never passes ───────────────────────────────────

// gitleaksQuietStub installs a shared stub gitleaks binary (emits an empty
// results array) and puts its directory at the front of PATH. The stub is
// created once per package because check.FindTool caches positive lookups for
// the process lifetime: per-test stubs would go stale after the test's temp
// dir is cleaned.
func gitleaksQuietStub(t *testing.T) string {
	t.Helper()
	gitleaksStubOnce.Do(func() {
		dir, err := os.MkdirTemp("", "crivo-gitleaks-stub-*")
		if err != nil {
			t.Fatalf("create stub dir: %v", err)
		}
		gitleaksStubDir = dir
		bin := filepath.Join(dir, "gitleaks")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		script := "#!/bin/sh\necho \"$@\" >> \"" + filepath.ToSlash(filepath.Join(dir, "invocations.log")) + "\"\necho '[]'\n"
		if runtime.GOOS == "windows" {
			script = "@echo off\necho []\n"
		}
		if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
			t.Fatalf("write stub: %v", err)
		}
	})
	t.Setenv("PATH", gitleaksStubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(gitleaksStubDir, "gitleaks")
}

var (
	gitleaksStubOnce sync.Once
	gitleaksStubDir  string
)

func TestAnalyze_ActiveScopeZeroTargets_StatusError(t *testing.T) {
	projectDir := t.TempDir()
	gitleaksQuietStub(t)

	// Scope references only files that do not exist on disk → 0 scannable targets.
	ctx := check.WithNewCodeScope(context.Background(), check.NewScope(
		[]gitutil.ChangedFile{{Path: "src/secret.ts"}},
		nil,
	))

	p := New()
	result, err := p.Analyze(ctx, projectDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusError {
		t.Fatalf("expected StatusError for active scope with 0 targets, got %s (summary %q)", result.Status, result.Summary)
	}
	if !strings.Contains(result.Summary, "0 scannable targets") {
		t.Errorf("expected summary to mention 0 scannable targets, got %q", result.Summary)
	}
	if len(result.Details) == 0 {
		t.Error("expected a detail instructing to check the diff")
	}
}

func TestAnalyze_ActiveScopeWithTargets_RunsScan(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "src"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "src", "app.ts"), []byte("const x = 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	gitleaksQuietStub(t)

	ctx := check.WithNewCodeScope(context.Background(), check.NewScope(
		[]gitutil.ChangedFile{{Path: "src/app.ts"}},
		nil,
	))

	p := New()
	result, err := p.Analyze(ctx, projectDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The stub emitted [] — a real scan ran and found nothing, which is a
	// legitimate pass (unlike the vacuous 0-target pass).
	if result.Status != domain.StatusPassed {
		t.Fatalf("expected passed after a real (stubbed) scan with no findings, got %s (summary %q)", result.Status, result.Summary)
	}
	invocations, err := os.ReadFile(filepath.Join(gitleaksStubDir, "invocations.log"))
	if err != nil {
		t.Fatalf("expected the stub to be invoked: %v", err)
	}
	if len(invocations) == 0 {
		t.Error("expected at least one gitleaks invocation for a scope with targets")
	}
}

func TestGitleaksTargets_NoScope_ReturnsProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	targets := gitleaksTargets(context.Background(), projectDir)
	if len(targets) != 1 || targets[0] != projectDir {
		t.Fatalf("expected [projectDir] without scope, got %#v", targets)
	}
}

// ─── Stub binary tests ───────────────────────────────────────────────────────

// gitleaksStubBin writes a fake gitleaks binary that records every invocation
// (args) to a log file and emits a fixed JSON report on stdout. The report
// references a file inside projectDir so normalization can be asserted.
// Returns the binary path and the log file path.
func gitleaksStubBin(t *testing.T, projectDir string) (binPath, logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "invocations.log")
	binPath = filepath.Join(dir, "gitleaks")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	reportedFile := filepath.ToSlash(filepath.Join(projectDir, "src", "config.ts"))
	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
cat <<'EOF'
[{"Description":"AWS Access Key","StartLine":10,"EndLine":10,"StartColumn":15,"EndColumn":35,"File":"` + reportedFile + `","Entropy":3.5,"RuleID":"aws-access-key-id","Fingerprint":"abc123","Match":"AKIAIOSFODNN7EXAMPLE"}]
EOF
`
	if runtime.GOOS == "windows" {
		script = `@echo off
echo %*>> "` + logPath + `"
echo [{"Description":"AWS Access Key","StartLine":10,"EndLine":10,"StartColumn":15,"EndColumn":35,"File":"` + reportedFile + `","Entropy":3.5,"RuleID":"aws-access-key-id","Fingerprint":"abc123","Match":"AKIAIOSFODNN7EXAMPLE"}]
`
	}

	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return binPath, logPath
}

func readInvocations(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read invocations log: %v", err)
	}
	var invocations []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			invocations = append(invocations, line)
		}
	}
	return invocations
}

func TestRunGitleaksTargets_SingleInvocationPerChunk(t *testing.T) {
	projectDir := t.TempDir()
	binPath, logPath := gitleaksStubBin(t, projectDir)

	targets := []string{
		filepath.Join(projectDir, "src", "a.ts"),
		filepath.Join(projectDir, "src", "b.ts"),
		filepath.Join(projectDir, "src", "c.ts"),
	}

	results, err := runGitleaksTargets(context.Background(), binPath, projectDir, targets)
	if err != nil {
		t.Fatalf("runGitleaksTargets() error = %v", err)
	}

	// gitleaks 8.24.3 accepts a single --source per invocation (chunk size 1),
	// so 3 targets must produce 3 invocations, each with exactly one --source.
	invocations := readInvocations(t, logPath)
	if len(invocations) != 3 {
		t.Fatalf("expected 3 invocations, got %d: %#v", len(invocations), invocations)
	}
	for i, inv := range invocations {
		if got := strings.Count(inv, "--source="); got != 1 {
			t.Errorf("invocation %d: expected 1 --source arg, got %d: %q", i, got, inv)
		}
	}

	// Results must be identical to the old per-target loop (normalized to
	// project-relative paths).
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.File != "src/config.ts" {
			t.Errorf("expected normalized relative path src/config.ts, got %q", r.File)
		}
	}
}

func TestRunGitleaksTargets_Chunking(t *testing.T) {
	projectDir := t.TempDir()
	binPath, logPath := gitleaksStubBin(t, projectDir)

	targets := make([]string, 200)
	for i := range targets {
		targets[i] = filepath.Join(projectDir, "src", "f"+string(rune('a'+i%26))+".ts")
	}

	results, err := runGitleaksTargets(context.Background(), binPath, projectDir, targets)
	if err != nil {
		t.Fatalf("runGitleaksTargets() error = %v", err)
	}

	// With chunk size 1, 200 targets => 200 invocations, one --source each.
	invocations := readInvocations(t, logPath)
	if len(invocations) != 200 {
		t.Fatalf("expected 200 invocations, got %d", len(invocations))
	}
	for i, inv := range invocations {
		if got := strings.Count(inv, "--source="); got != 1 {
			t.Errorf("invocation %d: expected 1 --source arg, got %d", i, got)
		}
	}
	if len(results) != 200 {
		t.Fatalf("expected 200 results, got %d", len(results))
	}
}
