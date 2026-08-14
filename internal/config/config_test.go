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
	if cfg.Checks.Semgrep != true {
		t.Error("Semgrep should be true after override")
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
