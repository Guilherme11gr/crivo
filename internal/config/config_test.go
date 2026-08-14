package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Profile != "balanced" {
		t.Errorf("Profile = %q, want balanced", cfg.Profile)
	}
	if !cfg.Checks.Typescript {
		t.Error("Checks.Typescript should be true by default")
	}
	if cfg.Coverage.Lines != 60 {
		t.Errorf("Coverage.Lines = %f, want 60", cfg.Coverage.Lines)
	}
	// coverage.new-code defaults to "off": PR mode skips the expensive suite
	if cfg.Coverage.NewCode != string(CoverageNewCodeOff) {
		t.Errorf("Coverage.NewCode = %q, want %q", cfg.Coverage.NewCode, CoverageNewCodeOff)
	}
}

func TestLoad_NoConfig(t *testing.T) {
	dir := t.TempDir()
	cfg, source, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if source != "defaults" {
		t.Errorf("source = %q, want defaults", source)
	}
	if cfg.Profile != "balanced" {
		t.Errorf("Profile = %q, want balanced", cfg.Profile)
	}
}

func TestLoad_YAMLConfig(t *testing.T) {
	dir := t.TempDir()
	configContent := `
profile: strict
`
	err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, source, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if source != filepath.Join(dir, ".qualitygate.yaml") {
		t.Errorf("source = %q, want yaml path", source)
	}
	if cfg.Profile != "strict" {
		t.Errorf("Profile = %q, want strict", cfg.Profile)
	}
	// Strict profile sets coverage to 80
	if cfg.Coverage.Lines != 80 {
		t.Errorf("Coverage.Lines = %f, want 80 (strict profile)", cfg.Coverage.Lines)
	}
	// Strict enables all checks including semgrep
	if !cfg.Checks.Semgrep {
		t.Error("Semgrep should be true in strict profile")
	}
}

func TestLoad_YAMLConfigOverrides(t *testing.T) {
	dir := t.TempDir()
	configContent := `
profile: balanced
coverage:
  lines: 75
  new-code: related
checks:
  semgrep: true
`
	err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Coverage.Lines != 75 {
		t.Errorf("Coverage.Lines = %f, want 75", cfg.Coverage.Lines)
	}
	if cfg.Coverage.NewCode != "related" {
		t.Errorf("Coverage.NewCode = %q, want related", cfg.Coverage.NewCode)
	}
	if cfg.Checks.Semgrep != true {
		t.Error("Semgrep should be true after override")
	}
}

func TestLoad_CoverageNewCodeInvalidValueIsError(t *testing.T) {
	dir := t.TempDir()
	configContent := `
coverage:
  new-code: sometimes
`
	if err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load(dir)
	if err == nil {
		t.Fatal("expected a validation error for unknown coverage.new-code value, got nil")
	}
	if cfg == nil {
		t.Fatal("expected the parsed config alongside the validation error for diagnostics")
	}
	if cfg.Coverage.NewCode != "sometimes" {
		t.Fatalf("Coverage.NewCode = %q, want the invalid value surfaced", cfg.Coverage.NewCode)
	}
}

func TestLoad_CoverageNewCodeValidValues(t *testing.T) {
	for _, mode := range []string{"off", "related", "full"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			configContent := "coverage:\n  new-code: " + mode + "\n"
			if err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644); err != nil {
				t.Fatal(err)
			}

			cfg, _, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() validation error: %v, want nil", err)
			}
			if cfg.Coverage.NewCode != mode {
				t.Errorf("Coverage.NewCode = %q, want %q", cfg.Coverage.NewCode, mode)
			}
		})
	}
}

func TestGenerateDefault(t *testing.T) {
	data, err := GenerateDefault()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("GenerateDefault returned empty data")
	}
}

func TestLoad_QualityGateOverallCoverageOverridesThreshold(t *testing.T) {
	dir := t.TempDir()
	configContent := `
profile: balanced
quality-gate:
  overall:
    coverage: 70
    duplications: 8
`
	err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.QualityGate.Overall.Coverage != 70 {
		t.Errorf("Overall.Coverage = %f, want 70", cfg.QualityGate.Overall.Coverage)
	}
	if cfg.QualityGate.Overall.Duplications != 8 {
		t.Errorf("Overall.Duplications = %f, want 8", cfg.QualityGate.Overall.Duplications)
	}
}

func TestLoad_MalformedYAMLIsHardError(t *testing.T) {
	dir := t.TempDir()
	// Tab indentation is invalid YAML: silently falling back to defaults would
	// make every custom rule and threshold disappear while the gate stays green.
	configContent := "profile: strict\n\tchecks:\n\t  semgrep: true\n"
	err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, source, err := Load(dir)
	if err == nil {
		t.Fatalf("expected error for malformed YAML, got cfg=%+v source=%q", cfg, source)
	}
	if !strings.Contains(err.Error(), ".qualitygate.yaml") {
		t.Errorf("error should cite the config path, got %q", err)
	}
	if !strings.Contains(err.Error(), "config parse error") {
		t.Errorf("error should be a config parse error, got %q", err)
	}
}

func TestLoad_MalformedCandidateNotMaskedByValidSibling(t *testing.T) {
	dir := t.TempDir()
	// A broken .qualitygate.yaml must not be silently skipped just because a
	// valid .qualitygate.yml exists next to it.
	err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte("profile: strict\n\tbad: indent\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir, ".qualitygate.yml"), []byte("profile: strict\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = Load(dir)
	if err == nil {
		t.Fatal("expected error when a candidate file exists but is malformed")
	}
	if !strings.Contains(err.Error(), ".qualitygate.yaml") {
		t.Errorf("error should cite the malformed candidate, got %q", err)
	}
}

func TestLoad_CustomRuleTestsParsed(t *testing.T) {
	dir := t.TempDir()
	configContent := `
custom-rules:
  - id: no-eval
    type: semgrep
    pattern: "eval(...)"
    message: "No eval"
    tests:
      - code: "const x = eval(userInput)"
        match: true
      - code: "const x = evaluate(userInput)"
        match: false
`
	if err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(cfg.CustomRules))
	}
	rule := cfg.CustomRules[0]
	if len(rule.Tests) != 2 {
		t.Fatalf("expected 2 test specs, got %d", len(rule.Tests))
	}
	if rule.Tests[0].Code != "const x = eval(userInput)" || !rule.Tests[0].Match {
		t.Errorf("unexpected first spec: %+v", rule.Tests[0])
	}
	if rule.Tests[1].Code != "const x = evaluate(userInput)" || rule.Tests[1].Match {
		t.Errorf("unexpected second spec: %+v", rule.Tests[1])
	}
}

func TestLoad_CustomRuleWithoutTestsStillParses(t *testing.T) {
	// Retrocompat: YAML written before the tests field existed must load
	// unchanged — the field is additive.
	dir := t.TempDir()
	configContent := `
custom-rules:
  - id: no-moment
    type: ban-import
    packages: ["moment"]
    message: "Use date-fns"
`
	if err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(cfg.CustomRules))
	}
	if len(cfg.CustomRules[0].Tests) != 0 {
		t.Errorf("expected no test specs, got %d", len(cfg.CustomRules[0].Tests))
	}
}

// ─── Include: packs and rule files (plan 008 spike) ─────────────────────────

func TestLoad_IncludeRelativeRuleFile(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, ".qualitygate.yaml", `
include:
  - "./rules/security.yaml"
custom-rules:
  - id: local-rule
    type: ban-pattern
    pattern: "TODO"
    message: "No TODOs"
`)
	writeConfigFile(t, dir, "rules/security.yaml", `
custom-rules:
  - id: no-eval
    type: ban-pattern
    pattern: "eval\\("
    message: "No eval"
`)

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CustomRules) != 2 {
		t.Fatalf("expected 2 rules (1 local + 1 included), got %d", len(cfg.CustomRules))
	}
	ids := map[string]bool{}
	for _, r := range cfg.CustomRules {
		ids[r.ID] = true
	}
	if !ids["local-rule"] || !ids["no-eval"] {
		t.Errorf("expected both local and included rules, got %v", ids)
	}
}

func TestLoad_IncludeEmbeddedPack(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, ".qualitygate.yaml", `
include:
  - "pack:security-ts"
`)

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CustomRules) != 3 {
		t.Fatalf("expected 3 rules from the security-ts pack, got %d", len(cfg.CustomRules))
	}
	ids := map[string]bool{}
	for _, r := range cfg.CustomRules {
		ids[r.ID] = true
	}
	for _, want := range []string{"no-eval", "no-innerhtml", "no-dangerouslysetinnerhtml"} {
		if !ids[want] {
			t.Errorf("pack should include rule %q, got %v", want, ids)
		}
	}
	// The pack rules ship with fixtures.
	for _, r := range cfg.CustomRules {
		if len(r.Tests) == 0 {
			t.Errorf("pack rule %q should carry fixtures", r.ID)
		}
	}
}

func TestLoad_IncludeUnknownPackIsError(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, ".qualitygate.yaml", `
include:
  - "pack:does-not-exist"
`)

	_, _, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for an unknown pack")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should name the pack, got %q", err)
	}
}

func TestLoad_IncludeMissingFileIsError(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, ".qualitygate.yaml", `
include:
  - "./rules/missing.yaml"
`)

	_, _, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a missing include file")
	}
	if !strings.Contains(err.Error(), "missing.yaml") {
		t.Errorf("error should name the missing file, got %q", err)
	}
}

func TestLoad_IncludeDuplicateIDRejectedByCompile(t *testing.T) {
	// Duplicate IDs between a pack and local rules are not a load error: the
	// custom-rules provider's CompileRules already rejects duplicates, and the
	// pack rules flow through the same path.
	dir := t.TempDir()
	writeConfigFile(t, dir, ".qualitygate.yaml", `
include:
  - "pack:security-ts"
custom-rules:
  - id: no-eval
    type: ban-pattern
    pattern: "eval\\("
    message: "No eval"
`)

	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 3 pack rules + 1 local duplicate = 4; CompileRules must flag the dup.
	if len(cfg.CustomRules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(cfg.CustomRules))
	}
}

func writeConfigFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
