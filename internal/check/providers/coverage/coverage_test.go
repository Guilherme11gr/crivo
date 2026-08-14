package coverage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
	gitutil "github.com/guilherme11gr/crivo/internal/git"
)

// stubNpx installs a fake npx executable in PATH that runs the given script
// body (a shell snippet) and returns the cleanup function.
func stubNpx(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("npx stub uses a POSIX shell script")
	}
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "npx")
	content := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAnalyze_StaleSummaryIgnoredWhenSuiteFails(t *testing.T) {
	// The test suite fails (exit 1) but a stale coverage-summary.json from a
	// previous run exists on disk. The check must fail — never read the stale
	// numbers and pass.
	stubNpx(t, `echo "FAIL src/service.test.ts" >&2; exit 1`)

	dir := t.TempDir()
	pkg := `{"devDependencies":{"jest":"^29.0.0"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}
	// Stale summary from a previous (passing) run.
	stale := `{"total":{"lines":{"total":100,"covered":95,"skipped":0,"pct":95.0},"branches":{"total":50,"covered":45,"skipped":0,"pct":90.0},"functions":{"total":20,"covered":18,"skipped":0,"pct":90.0},"statements":{"total":120,"covered":110,"skipped":0,"pct":91.7}}}`
	if err := os.MkdirAll(filepath.Join(dir, "coverage"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "coverage", "coverage-summary.json"), []byte(stale), 0644); err != nil {
		t.Fatal(err)
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, config.DefaultConfig())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if result.Status != domain.StatusFailed {
		t.Fatalf("status = %s, want failed (stale summary must not pass)", result.Status)
	}
	if result.Metrics != nil {
		if _, hasLines := result.Metrics["lines"]; hasLines {
			t.Fatal("lines metric must not come from a stale summary")
		}
	}
}

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

// newCodeCtx returns a context carrying the given changed files as new-code scope.
func newCodeCtx(t *testing.T, files ...string) context.Context {
	t.Helper()
	changed := make([]gitutil.ChangedFile, 0, len(files))
	for _, f := range files {
		changed = append(changed, gitutil.ChangedFile{Path: f, Status: "M"})
	}
	return check.WithNewCodeScope(context.Background(), check.NewScope(changed, nil))
}

func TestAnalyze_NewCodeDefaultSkipsSuite(t *testing.T) {
	// In --new-code mode with the default coverage.new-code=off, the suite must
	// never run: no npx stub, no package.json — if the provider tried to run
	// anything it would fail loudly instead of skipping.
	stubNpx(t, `echo "npx must not run" >&2; exit 99`)

	dir := t.TempDir()

	p := New()
	cfg := config.DefaultConfig()
	result, err := p.Analyze(newCodeCtx(t, "src/app.ts"), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if result.Status != domain.StatusSkipped {
		t.Fatalf("status = %s, want skipped (default new-code mode must not run the suite)", result.Status)
	}
	if !strings.Contains(result.Summary, "coverage.new-code") {
		t.Errorf("summary = %q, want it to point at coverage.new-code", result.Summary)
	}
}

func TestAnalyze_NewCodeExplicitOffSkips(t *testing.T) {
	stubNpx(t, `echo "npx must not run" >&2; exit 99`)

	dir := t.TempDir()
	p := New()
	cfg := config.DefaultConfig()
	cfg.Coverage.NewCode = "off"

	result, err := p.Analyze(newCodeCtx(t, "src/app.ts"), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != domain.StatusSkipped {
		t.Fatalf("status = %s, want skipped", result.Status)
	}
}

func TestAnalyze_NewCodeFullRunsSuite(t *testing.T) {
	// full behaves like the pre-new-code path: suite runs, summary read.
	stubNpx(t, `mkdir -p coverage
cat > coverage/coverage-summary.json <<'EOF'
{"total":{"lines":{"total":100,"covered":90,"skipped":0,"pct":90.0},"branches":{"total":50,"covered":40,"skipped":0,"pct":80.0},"functions":{"total":20,"covered":18,"skipped":0,"pct":90.0},"statements":{"total":120,"covered":110,"skipped":0,"pct":91.7}}}
EOF
`)

	dir := t.TempDir()
	pkg := `{"devDependencies":{"jest":"^29.0.0"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}

	p := New()
	cfg := config.DefaultConfig()
	cfg.Coverage.NewCode = "full"

	result, err := p.Analyze(newCodeCtx(t, "src/app.ts"), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != domain.StatusPassed {
		t.Fatalf("status = %s, want passed (full mode runs the suite)", result.Status)
	}
}

func TestAnalyze_NewCodeRelatedNoSourceFilesSkips(t *testing.T) {
	// Scope contains only non-source files — nothing to run related tests
	// against, so it degrades to a skip with its own summary.
	stubNpx(t, `echo "npx must not run" >&2; exit 99`)

	dir := t.TempDir()
	p := New()
	cfg := config.DefaultConfig()
	cfg.Coverage.NewCode = "related"

	result, err := p.Analyze(newCodeCtx(t, "README.md", "package.json"), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != domain.StatusSkipped {
		t.Fatalf("status = %s, want skipped", result.Status)
	}
	if !strings.Contains(result.Summary, "no source files") {
		t.Errorf("summary = %q, want the no-source-files message", result.Summary)
	}
}

func TestRelatedTestArgs(t *testing.T) {
	files := []string{"src/a.ts", "src/b.tsx"}

	vitestArgs := relatedTestArgs("vitest", files)
	wantVitest := []string{"vitest", "related", "--coverage", "src/a.ts", "src/b.tsx"}
	if strings.Join(vitestArgs, " ") != strings.Join(wantVitest, " ") {
		t.Errorf("vitest args = %#v, want %#v", vitestArgs, wantVitest)
	}

	jestArgs := relatedTestArgs("jest", files)
	wantJest := []string{"jest", "--findRelatedTests", "src/a.ts", "src/b.tsx", "--coverage", "--coverageReporters=json-summary", "--passWithNoTests", "--silent"}
	if strings.Join(jestArgs, " ") != strings.Join(wantJest, " ") {
		t.Errorf("jest args = %#v, want %#v", jestArgs, wantJest)
	}
}

func TestAnalyze_NewCodeRelatedBuildsArgsViaStub(t *testing.T) {
	// Capture the argv the provider passes to npx in related mode and verify
	// the related flags plus the changed source files land on the command line.
	stubNpx(t, `echo "ARGS:$*" > args.txt
mkdir -p coverage
cat > coverage/coverage-summary.json <<'EOF'
{"total":{"lines":{"total":100,"covered":90,"skipped":0,"pct":90.0},"branches":{"total":50,"covered":40,"skipped":0,"pct":80.0},"functions":{"total":20,"covered":18,"skipped":0,"pct":90.0},"statements":{"total":120,"covered":110,"skipped":0,"pct":91.7}}}
EOF
`)

	dir := t.TempDir()
	pkg := `{"devDependencies":{"jest":"^29.0.0"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}

	p := New()
	cfg := config.DefaultConfig()
	cfg.Coverage.NewCode = "related"

	result, err := p.Analyze(newCodeCtx(t, "src/app.ts", "src/util.tsx"), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != domain.StatusPassed {
		t.Fatalf("status = %s, want passed", result.Status)
	}

	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatalf("stub did not capture argv: %v", err)
	}
	captured := strings.TrimSpace(string(data))
	for _, want := range []string{"jest", "--findRelatedTests", "src/app.ts", "src/util.tsx", "--coverage"} {
		if !strings.Contains(captured, want) {
			t.Errorf("captured argv %q does not contain %q", captured, want)
		}
	}
}

func TestNewCodeSourceFiles(t *testing.T) {
	scope := check.NewScope(
		[]gitutil.ChangedFile{
			{Path: "src/app.ts", Status: "M"},
			{Path: "src/util.tsx", Status: "M"},
			{Path: "src/legacy.js", Status: "M"},
			{Path: "src/styles.css", Status: "M"},
			{Path: "README.md", Status: "M"},
			{Path: "package-lock.json", Status: "M"},
		},
		nil,
	)

	got := newCodeSourceFiles(scope)
	want := []string{"src/app.ts", "src/legacy.js", "src/util.tsx"}
	if len(got) != len(want) {
		t.Fatalf("newCodeSourceFiles() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("newCodeSourceFiles()[%d] = %q, want %q (sorted, source-only)", i, got[i], want[i])
		}
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
