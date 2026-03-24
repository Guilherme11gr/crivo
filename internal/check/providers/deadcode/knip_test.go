package deadcode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestParseKnipOutput_Empty(t *testing.T) {
	issues, unusedFiles, unusedExports, unusedDeps := parseKnipOutput("", "/project")

	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
	if unusedFiles != 0 {
		t.Errorf("expected 0 unused files, got %d", unusedFiles)
	}
	if unusedExports != 0 {
		t.Errorf("expected 0 unused exports, got %d", unusedExports)
	}
	if unusedDeps != 0 {
		t.Errorf("expected 0 unused deps, got %d", unusedDeps)
	}
}

func TestParseKnipOutput_UnusedFiles(t *testing.T) {
	output := `Unused files
src/old-utils.ts
src/legacy/helper.ts
`
	issues, unusedFiles, unusedExports, unusedDeps := parseKnipOutput(output, "/project")

	if unusedFiles != 2 {
		t.Errorf("expected 2 unused files, got %d", unusedFiles)
	}
	if unusedExports != 0 {
		t.Errorf("expected 0 unused exports, got %d", unusedExports)
	}
	if unusedDeps != 0 {
		t.Errorf("expected 0 unused deps, got %d", unusedDeps)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	issue := issues[0]
	if issue.RuleID != "unused-file" {
		t.Errorf("expected ruleID='unused-file', got %q", issue.RuleID)
	}
	if issue.Severity != domain.SeverityMajor {
		t.Errorf("expected severity=major, got %q", issue.Severity)
	}
	if issue.Source != "knip" {
		t.Errorf("expected source='knip', got %q", issue.Source)
	}
}

func TestParseKnipOutput_UnusedExports(t *testing.T) {
	output := `Unused exports
src/utils.ts:15
src/helpers.ts:30
src/types.ts:5
`
	issues, _, unusedExports, _ := parseKnipOutput(output, "/project")

	if unusedExports != 3 {
		t.Errorf("expected 3 unused exports, got %d", unusedExports)
	}
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}

	issue := issues[0]
	if issue.RuleID != "unused-export" {
		t.Errorf("expected ruleID='unused-export', got %q", issue.RuleID)
	}
	if issue.Line != 15 {
		t.Errorf("expected line=15, got %d", issue.Line)
	}
}

func TestParseKnipOutput_UnusedDependencies(t *testing.T) {
	// Note: the parser requires lines to contain '/', '\', or '.' to be processed.
	// Simple package names like "lodash" are skipped. Scoped packages like
	// "@scope/pkg" or packages with dots like "lodash.merge" are parsed.
	output := `Unused dependencies
@angular/core
lodash.merge
`
	issues, _, _, unusedDeps := parseKnipOutput(output, "/project")

	if unusedDeps != 2 {
		t.Errorf("expected 2 unused deps, got %d", unusedDeps)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	issue := issues[0]
	if issue.RuleID != "unused-dependency" {
		t.Errorf("expected ruleID='unused-dependency', got %q", issue.RuleID)
	}
	if issue.File != "package.json" {
		t.Errorf("expected file='package.json', got %q", issue.File)
	}
}

func TestParseKnipOutput_UnusedTypes(t *testing.T) {
	output := `Unused types
src/types.ts:10
`
	issues, _, unusedExports, _ := parseKnipOutput(output, "/project")

	// Unused types count as unused exports
	if unusedExports != 1 {
		t.Errorf("expected 1 unused export (types counted as exports), got %d", unusedExports)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.RuleID != "unused-type" {
		t.Errorf("expected ruleID='unused-type', got %q", issue.RuleID)
	}
}

func TestParseKnipOutput_MixedSections(t *testing.T) {
	output := `Unused files
src/old.ts

Unused dependencies
@scope/unused-lib

Unused exports
src/utils.ts:5
src/helpers.ts:10

Unused types
src/types.ts:1
`
	issues, unusedFiles, unusedExports, unusedDeps := parseKnipOutput(output, "/project")

	if unusedFiles != 1 {
		t.Errorf("expected 1 unused file, got %d", unusedFiles)
	}
	if unusedDeps != 1 {
		t.Errorf("expected 1 unused dep, got %d", unusedDeps)
	}
	// 2 unused exports + 1 unused type = 3
	if unusedExports != 3 {
		t.Errorf("expected 3 unused exports (incl types), got %d", unusedExports)
	}
	// 1 file + 1 dep + 2 exports + 1 type = 5
	if len(issues) != 5 {
		t.Errorf("expected 5 issues total, got %d", len(issues))
	}
}

func TestParseKnipOutput_FileWithLineNumber(t *testing.T) {
	output := `Unused exports
src/utils.ts:42
`
	issues, _, _, _ := parseKnipOutput(output, "/project")

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Line != 42 {
		t.Errorf("expected line=42, got %d", issues[0].Line)
	}
}

func TestParseKnipOutput_FileWithoutLineNumber(t *testing.T) {
	output := `Unused files
src/old-utils.ts
`
	issues, _, _, _ := parseKnipOutput(output, "/project")

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Line != 1 {
		t.Errorf("expected line=1 (default), got %d", issues[0].Line)
	}
}

func TestDetect(t *testing.T) {
	p := New()

	// Should detect with package.json
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	if !p.Detect(context.Background(), dir) {
		t.Error("expected Detect=true with package.json")
	}

	// Should not detect without package.json
	emptyDir := t.TempDir()
	if p.Detect(context.Background(), emptyDir) {
		t.Error("expected Detect=false without package.json")
	}
}

func TestNameAndID(t *testing.T) {
	p := New()
	if p.Name() != "Dead Code" {
		t.Errorf("expected Name='Dead Code', got %q", p.Name())
	}
	if p.ID() != "dead-code" {
		t.Errorf("expected ID='dead-code', got %q", p.ID())
	}
}
