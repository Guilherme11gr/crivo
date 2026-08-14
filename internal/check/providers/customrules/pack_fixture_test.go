package customrules

import (
	"testing"

	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/packs"
	"gopkg.in/yaml.v3"
)

// TestEmbeddedPackFixturesCompileWithRealSemgrep validates the security-ts
// pack's fixtures against the REAL semgrep binary when present. Stub-based
// tests cannot catch semantic mismatches (namespaced check_ids, language
// detection on extensionless files) — only the real binary can.
func TestEmbeddedPackFixturesCompileWithRealSemgrep(t *testing.T) {
	if check.FindTool("semgrep") == "" {
		t.Skip("semgrep not available: fixture semantics need the real binary")
	}

	data, err := packs.Load("security-ts")
	if err != nil {
		t.Fatalf("load embedded pack: %v", err)
	}
	var pack struct {
		CustomRules []config.CustomRule `yaml:"custom-rules"`
	}
	if err := yaml.Unmarshal(data, &pack); err != nil {
		t.Fatalf("parse embedded pack: %v", err)
	}
	if len(pack.CustomRules) == 0 {
		t.Fatal("embedded pack has no rules")
	}

	_, errs, warns := CompileRules(pack.CustomRules)
	for _, e := range errs {
		t.Errorf("compile: %v", e)
	}
	for _, w := range warns {
		t.Errorf("unexpected warning with semgrep present: %s", w)
	}
}

// TestCheckIDMatchesRule pins the namespaced-id resolution against real
// semgrep behavior (config in /tmp → "tmp.<rule-id>").
func TestCheckIDMatchesRule(t *testing.T) {
	cases := []struct {
		checkID, ruleID string
		want            bool
	}{
		{"no-eval", "no-eval", true},
		{"tmp.no-eval", "no-eval", true},
		{"crivo.no-eval", "no-eval", true},
		{"no-eval.other", "no-eval", false},
		{"no-eval-2", "no-eval", false},
		{"", "no-eval", false},
	}
	for _, c := range cases {
		if got := checkIDMatchesRule(c.checkID, c.ruleID); got != c.want {
			t.Errorf("checkIDMatchesRule(%q, %q) = %v, want %v", c.checkID, c.ruleID, got, c.want)
		}
	}
}

// TestSemgrepFixtureExt pins the language→extension mapping that lets semgrep
// classify fixture temp files.
func TestSemgrepFixtureExt(t *testing.T) {
	cases := map[string]string{
		"ts": ".ts", "tsx": ".tsx", "TS": ".ts", "python": ".py", "unknown-lang": ".txt", "": ".txt",
	}
	for lang, want := range cases {
		if got := semgrepFixtureExt(lang); got != want {
			t.Errorf("semgrepFixtureExt(%q) = %q, want %q", lang, got, want)
		}
	}
	if exts := semgrepFixtureExts("ts"); len(exts) != 2 || exts[0] != ".ts" || exts[1] != ".tsx" {
		t.Errorf("semgrepFixtureExts(ts) = %v, want [.ts .tsx]", exts)
	}
	if exts := semgrepFixtureExts("go"); len(exts) != 1 || exts[0] != ".go" {
		t.Errorf("semgrepFixtureExts(go) = %v, want [.go]", exts)
	}
}
