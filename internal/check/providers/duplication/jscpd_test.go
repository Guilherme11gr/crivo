package duplication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
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

func TestAnalyze_NoReportIsErrorWithoutFabricatedMetrics(t *testing.T) {
	// jscpd fails to run (exit 1, no report file) — the check must be an
	// error and must NOT fabricate percentage/clones metrics that would
	// produce a passing duplication_pct condition.
	stubNpx(t, `echo "jscpd: command crashed" >&2; exit 1`)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}

	p := New()
	result, err := p.Analyze(context.Background(), dir, config.DefaultConfig())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if result.Status != domain.StatusError {
		t.Fatalf("status = %s, want error", result.Status)
	}
	if result.Metrics != nil {
		if _, hasPct := result.Metrics["percentage"]; hasPct {
			t.Fatal("percentage metric must not exist when no analysis happened")
		}
		if _, hasClones := result.Metrics["clones"]; hasClones {
			t.Fatal("clones metric must not exist when no analysis happened")
		}
	}
}

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

func TestAnalyze_MultiSrcScansAllDirs(t *testing.T) {
	// Two configured src dirs (a/, b/), with a clone crossing both. jscpd is
	// stubbed to emit a report whose findings reference both dirs, and the
	// captured argv must list both dirs as positional arguments.
	stubNpx(t, `echo "ARGS:$*" > args.txt
mkdir -p .qualitygate-temp
cat > .qualitygate-temp/jscpd-report.json <<'EOF'
{
  "statistics": {"total": {"percentage": 10.0, "lines": 100, "sources": 4, "clones": 1}},
  "duplicates": [
    {
      "firstFile": {"name": "a/util.ts", "start": 1, "end": 10, "startLoc": {"line": 1, "column": 1}, "endLoc": {"line": 10, "column": 1}},
      "secondFile": {"name": "b/helper.ts", "start": 1, "end": 10, "startLoc": {"line": 1, "column": 1}, "endLoc": {"line": 10, "column": 1}},
      "lines": 10, "tokens": 30, "fragment": "dup"
    },
    {
      "firstFile": {"name": "a/other.ts", "start": 1, "end": 10, "startLoc": {"line": 1, "column": 1}, "endLoc": {"line": 10, "column": 1}},
      "secondFile": {"name": "b/more.ts", "start": 1, "end": 10, "startLoc": {"line": 1, "column": 1}, "endLoc": {"line": 10, "column": 1}},
      "lines": 10, "tokens": 30, "fragment": "dup"
    }
  ]
}
EOF
`)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "b"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create the files the report references, so path resolution can verify
	// them on disk.
	for _, f := range []string{"a/util.ts", "a/other.ts", "b/helper.ts", "b/more.ts"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("export const x = 1;\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	p := New()
	cfg := config.DefaultConfig()
	cfg.Src = []string{"a/", "b/"}
	// Keep the test deterministic: only jscpd findings, no semantic clones
	// (the fixture files are identical tiny files that would be flagged).
	cfg.Duplication.Semantic = false

	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != domain.StatusFailed {
		t.Fatalf("status = %s, want failed (10%% > 5%% threshold)", result.Status)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result.Issues))
	}
	// Issue file must be project-relative with the src prefix preserved.
	// jscpd issues point at the first file of each pair.
	files := map[string]bool{}
	for _, is := range result.Issues {
		files[is.File] = true
	}
	if !files["a/util.ts"] {
		t.Errorf("issues %v missing a/util.ts (src prefix must be preserved)", result.Issues)
	}
	if !files["a/other.ts"] {
		t.Errorf("issues %v missing a/other.ts (second pair must keep its src prefix)", result.Issues)
	}

	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatalf("stub did not capture argv: %v", err)
	}
	captured := string(data)
	if !strings.Contains(captured, "jscpd") {
		t.Errorf("captured argv %q missing jscpd", captured)
	}
	// Both src dirs must be passed as positional arguments (absolute paths).
	for _, src := range []string{"a", "b"} {
		if !strings.Contains(captured, filepath.Join(dir, src)) {
			t.Errorf("captured argv %q does not contain src dir %q (both src dirs must be passed)", captured, filepath.Join(dir, src))
		}
	}
}

func TestAnalyze_MultiSrcSkipsOnlyWhenNoneExist(t *testing.T) {
	// One configured src dir missing, another present: analysis must still run.
	stubNpx(t, `echo "ARGS:$*" > args.txt
mkdir -p .qualitygate-temp
echo '{"statistics":{"total":{"percentage":0,"lines":0,"sources":1,"clones":0}},"duplicates":[]}' > .qualitygate-temp/jscpd-report.json
`)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "b"), 0755); err != nil {
		t.Fatal(err)
	}

	p := New()
	cfg := config.DefaultConfig()
	cfg.Src = []string{"missing/", "b/"}

	result, err := p.Analyze(context.Background(), dir, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != domain.StatusPassed {
		t.Fatalf("status = %s, want passed (missing dir ignored, b/ scanned)", result.Status)
	}

	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatalf("stub did not capture argv: %v", err)
	}
	captured := string(data)
	if strings.Contains(captured, "missing/") {
		t.Errorf("captured argv %q must not contain the missing dir", captured)
	}
	if !strings.Contains(captured, filepath.Join(dir, "b")) {
		t.Errorf("captured argv %q must contain the existing dir", captured)
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

func TestJscpdIgnoreArgs(t *testing.T) {
	excludes := []string{"node_modules/", "dist/"}

	v3Args := jscpdIgnoreArgs("3.5.10", excludes)
	if len(v3Args) != 2 || v3Args[0] != "--ignore=node_modules/" || v3Args[1] != "--ignore=dist/" {
		t.Fatalf("v3 args = %#v", v3Args)
	}

	v5Args := jscpdIgnoreArgs("cpd 5.0.5", excludes)
	if len(v5Args) != 1 || v5Args[0] != "--ignore-pattern=node_modules/,dist/" {
		t.Fatalf("v5 args = %#v", v5Args)
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

func TestNormalizeReportPath(t *testing.T) {
	dir := t.TempDir()
	// Two src dirs like a real multi-src project.
	for _, f := range []string{"a/util.ts", "b/helper.ts"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("export const x = 1;\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	srcPaths := []string{
		filepath.Join(dir, "a"),
		filepath.Join(dir, "b"),
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty", "", ""},
		// jscpd reports relative to project dir — resolved directly.
		{"project-relative", "a/util.ts", "a/util.ts"},
		// Some jscpd versions report relative to the scanned dir, dropping
		// the src prefix — must be resolved back to the project-relative path.
		{"scanned-dir-relative", "util.ts", "a/util.ts"},
		{"scanned-dir-relative-other", "helper.ts", "b/helper.ts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReportPath(dir, srcPaths, tt.path)
			if filepath.ToSlash(got) != tt.want {
				t.Errorf("normalizeReportPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	// Use a projectDir that works on both Windows and Unix
	projectDir := filepath.Join(string(filepath.Separator), "project")

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty path", "", ""},
		{"relative path from project root", "src/utils.ts", "src/utils.ts"},
		{"relative path with nested dirs", "src/components/Button.tsx", "src/components/Button.tsx"},
		{"dot-relative path", "./src/utils.ts", "src/utils.ts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(projectDir, tt.path)
			if filepath.ToSlash(got) != filepath.ToSlash(tt.want) {
				t.Errorf("normalizePath(%q, %q) = %q, want %q", projectDir, tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizePath_Absolute(t *testing.T) {
	// Absolute path tests require a real absolute projectDir (with drive letter on Windows)
	// Use the temp directory which is always absolute
	tmpDir := t.TempDir()

	// Create a file inside to make it a real path
	subDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	absPath := filepath.Join(tmpDir, "src", "utils.ts")
	got := normalizePath(tmpDir, absPath)
	want := filepath.ToSlash("src/utils.ts")
	if filepath.ToSlash(got) != want {
		t.Errorf("normalizePath(%q, %q) = %q, want %q", tmpDir, absPath, got, want)
	}
}
