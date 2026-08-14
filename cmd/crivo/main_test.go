package main

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/guilherme11gr/crivo/internal/domain"
	gitutil "github.com/guilherme11gr/crivo/internal/git"
)

func TestParseArgs_DisableChecksSupportsRepeatAndCommaList(t *testing.T) {
	opts := parseArgs([]string{
		"run",
		"--disable", "complexity,coverage",
		"--disable", "semgrep",
	})

	expected := []string{"complexity", "coverage", "semgrep"}
	for _, checkID := range expected {
		if !opts.disabledChecks[checkID] {
			t.Fatalf("expected disabledChecks[%q] to be true", checkID)
		}
	}
}

func TestParseCheckList_NormalizesValues(t *testing.T) {
	values := parseCheckList(" Complexity, coverage , ,SEMGRP ")

	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(values))
	}

	if values[0] != "complexity" || values[1] != "coverage" || values[2] != "semgrp" {
		t.Fatalf("unexpected normalized values: %#v", values)
	}
}

func TestFilterCheckToNewCode_RecomputesTypescriptMetrics(t *testing.T) {
	check := domain.CheckResult{
		ID:      "typescript",
		Status:  domain.StatusFailed,
		Summary: "1 prod error, 1 in tests",
		Issues: []domain.Issue{
			{File: "src/a.ts", Line: 10, Severity: domain.SeverityMajor, Type: domain.IssueTypeBug},
			{File: "src/a.test.ts", Line: 20, Severity: domain.SeverityMinor, Type: domain.IssueTypeBug},
		},
		Metrics: map[string]float64{"errors": 2, "prod_errors": 1, "test_errors": 1},
	}

	filterCheckToNewCode(&check, map[string]bool{"src/a.test.ts": true}, []gitutil.ChangedLine{{File: "src/a.test.ts", StartLine: 1, EndLine: 40}})

	if check.Status != domain.StatusWarning {
		t.Fatalf("status = %s, want warning", check.Status)
	}
	if got := check.Metrics["prod_errors"]; got != 0 {
		t.Fatalf("prod_errors = %v, want 0", got)
	}
	if got := check.Metrics["test_errors"]; got != 1 {
		t.Fatalf("test_errors = %v, want 1", got)
	}
	if check.Summary != "0 prod errors, 1 in tests" {
		t.Fatalf("summary = %q", check.Summary)
	}
}

func TestFilterCheckToNewCode_RecomputesSecretsMetrics(t *testing.T) {
	check := domain.CheckResult{
		ID:      "secrets",
		Status:  domain.StatusFailed,
		Issues:  []domain.Issue{{File: ".env.local", Line: 2, Severity: domain.SeverityBlocker}},
		Metrics: map[string]float64{"secrets": 1},
	}

	filterCheckToNewCode(&check, map[string]bool{"src/app/page.tsx": true}, nil)

	if check.Status != domain.StatusPassed {
		t.Fatalf("status = %s, want passed", check.Status)
	}
	if got := check.Metrics["secrets"]; got != 0 {
		t.Fatalf("secrets = %v, want 0", got)
	}
	if check.Summary != "0 secrets detected" {
		t.Fatalf("summary = %q", check.Summary)
	}
}

func TestFilterCheckToNewCode_RecomputesCustomRulesBlocking(t *testing.T) {
	check := domain.CheckResult{
		ID:     "custom-rules",
		Status: domain.StatusFailed,
		Issues: []domain.Issue{
			{File: "src/allowed.ts", Line: 5, Severity: domain.SeverityBlocker, Advisory: false},
			{File: "src/changed.ts", Line: 9, Severity: domain.SeverityMajor, Advisory: true},
		},
		Metrics: map[string]float64{"blocking_violations": 1, "advisory_violations": 1},
	}

	filterCheckToNewCode(&check, map[string]bool{"src/changed.ts": true}, []gitutil.ChangedLine{{File: "src/changed.ts", StartLine: 1, EndLine: 20}})

	if check.Status != domain.StatusPassed {
		t.Fatalf("status = %s, want passed", check.Status)
	}
	if got := check.Metrics["blocking_violations"]; got != 0 {
		t.Fatalf("blocking_violations = %v, want 0", got)
	}
	if got := check.Metrics["advisory_violations"]; got != 1 {
		t.Fatalf("advisory_violations = %v, want 1", got)
	}
	if check.Summary != "1 advisory-only violations in new code" {
		t.Fatalf("summary = %q", check.Summary)
	}
}

func TestFilterCheckToNewCode_RecomputesDuplicationMetrics(t *testing.T) {
	check := domain.CheckResult{
		ID:     "duplication",
		Status: domain.StatusFailed,
		Issues: []domain.Issue{
			{File: "src/legacy.ts", Line: 10, Source: "jscpd"},
			{File: "src/changed.ts", Line: 20, Source: "semantic"},
		},
		Metrics: map[string]float64{"percentage": 12, "clones": 1, "semantic_clones": 1},
	}

	filterCheckToNewCode(&check, map[string]bool{"src/changed.ts": true}, []gitutil.ChangedLine{{File: "src/changed.ts", StartLine: 1, EndLine: 30}})

	if check.Status != domain.StatusFailed {
		t.Fatalf("status = %s, want failed", check.Status)
	}
	if got := check.Metrics["percentage"]; got != 100 {
		t.Fatalf("percentage = %v, want 100", got)
	}
	if got := check.Metrics["semantic_clones"]; got != 1 {
		t.Fatalf("semantic_clones = %v, want 1", got)
	}

	filterCheckToNewCode(&check, map[string]bool{"src/other.ts": true}, nil)
	if check.Status != domain.StatusPassed {
		t.Fatalf("status after empty filter = %s, want passed", check.Status)
	}
	if got := check.Metrics["percentage"]; got != 0 {
		t.Fatalf("percentage after empty filter = %v, want 0", got)
	}
}

func TestAcquireRunLock_PreventsConcurrentRuns(t *testing.T) {
	projectDir := t.TempDir()

	releaseLock, err := acquireRunLock(projectDir)
	if err != nil {
		t.Fatalf("acquireRunLock() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".qualitygate", "run.lock")); err != nil {
		t.Fatalf("expected run.lock to exist: %v", err)
	}

	if _, err := acquireRunLock(projectDir); err == nil {
		t.Fatal("expected second lock acquisition to fail")
	}

	releaseLock()

	releaseLock2, err := acquireRunLock(projectDir)
	if err != nil {
		t.Fatalf("expected lock acquisition after release to succeed: %v", err)
	}
	releaseLock2()
}

func TestApplyBaselineComparison_DoesNotCreateStoreWithoutHistory(t *testing.T) {
	projectDir := t.TempDir()
	analysis := &domain.AnalysisResult{}

	applyBaselineComparison(analysis, projectDir, options{jsonOutput: true})

	if _, err := os.Stat(filepath.Join(projectDir, ".qualitygate")); !os.IsNotExist(err) {
		t.Fatalf(".qualitygate existence error = %v, want not exist", err)
	}
}

// gitRun runs a git command in dir and returns trimmed stdout, failing the
// test on error so fixtures stay deterministic.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestComputeNewCodeScope_DetachedHeadWithOnlyOriginMain reproduces the CI
// checkout shape: detached HEAD on a feature commit, local "main" deleted,
// only refs/remotes/origin/main present. The scope must resolve via the
// origin fallback and contain exactly the feature commit's files.
func TestComputeNewCodeScope_DetachedHeadWithOnlyOriginMain(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	writeFile(t, dir, "base.txt", "base\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "base commit")
	baseSHA := gitRun(t, dir, "rev-parse", "HEAD")

	gitRun(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "new.txt", "new\n")
	writeFile(t, dir, "base.txt", "base\nchanged\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "feature commit")
	featureSHA := gitRun(t, dir, "rev-parse", "HEAD")

	// CI-like state: detached HEAD at the feature SHA, local main deleted,
	// only refs/remotes/origin/main (pointing at the base commit) remains.
	gitRun(t, dir, "checkout", "--detach", featureSHA)
	gitRun(t, dir, "update-ref", "refs/remotes/origin/main", baseSHA)
	gitRun(t, dir, "update-ref", "-d", "refs/heads/main")

	files, lines, err := gitutil.ComputeNewCodeScope(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("ComputeNewCodeScope() error = %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("got %d changed files, want 2: %#v", len(files), files)
	}
	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["new.txt"] || !paths["base.txt"] {
		t.Fatalf("changed files = %#v, want new.txt and base.txt", paths)
	}

	foundNew := false
	foundBase := false
	for _, l := range lines {
		switch l.File {
		case "new.txt":
			foundNew = true
		case "base.txt":
			foundBase = true
		}
	}
	if !foundNew || !foundBase {
		t.Fatalf("changed lines = %#v, want ranges for new.txt and base.txt", lines)
	}
}

// TestComputeNewCodeScope_UnresolvableBaseErrors ensures the function fails
// loudly when neither the base nor origin/<base> exists.
func TestComputeNewCodeScope_UnresolvableBaseErrors(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "trunk")
	writeFile(t, dir, "base.txt", "base\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "base commit")

	_, _, err := gitutil.ComputeNewCodeScope(context.Background(), dir, "main")
	if err == nil {
		t.Fatal("ComputeNewCodeScope() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--new-code: cannot resolve base branch 'main'") {
		t.Fatalf("error %q does not carry the expected prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "origin/main") {
		t.Fatalf("error %q does not cite origin/main", err.Error())
	}
}

// TestComputeNewCodeScope_WorkingTreeMode ensures the legitimate empty-diff
// case (current branch IS the base, working-tree mode) does not error.
func TestComputeNewCodeScope_WorkingTreeMode(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	writeFile(t, dir, "base.txt", "base\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "base commit")

	files, _, err := gitutil.ComputeNewCodeScope(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("ComputeNewCodeScope() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("got %d changed files, want 0", len(files))
	}
}

// TestRunAnalysis_CommitHashPopulatedWithSave verifies the commit hash is
// captured and persisted when --save is used on a git repo.
func TestRunAnalysis_CommitHashPopulatedWithSave(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	writeFile(t, dir, "base.txt", "base\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "base commit")
	sha := gitRun(t, dir, "rev-parse", "HEAD")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	exit := runAnalysis(options{command: "run", save: true, jsonOutput: true})
	if exit != 0 {
		t.Fatalf("runAnalysis() exit = %d, want 0", exit)
	}

	db := filepath.Join(dir, ".qualitygate", "history.db")
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("history.db not created: %v", err)
	}

	sqldb, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	defer sqldb.Close()
	rows, err := sqldb.Query("SELECT commit_hash FROM analyses")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	defer rows.Close()
	commits := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		commits[c] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !commits[sha] {
		t.Fatalf("commit %s not found in history rows %#v", sha, commits)
	}
}

// TestRunAnalysis_NonGitRepoDoesNotRequireCommitHash ensures running in a
// non-git dir succeeds and leaves the commit hash empty (no error path).
func TestRunAnalysis_NonGitRepoDoesNotRequireCommitHash(t *testing.T) {
	dir := t.TempDir()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if exit := runAnalysis(options{command: "run", save: true, jsonOutput: true}); exit != 0 {
		t.Fatalf("runAnalysis() exit = %d, want 0", exit)
	}
}
