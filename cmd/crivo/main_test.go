package main

import (
	"os"
	"path/filepath"
	"testing"

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
