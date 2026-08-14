package config

import (
	"os"
	"path/filepath"
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
	cfg, source, verr := Load(dir)

	if source != "defaults" {
		t.Errorf("source = %q, want defaults", source)
	}
	if cfg.Profile != "balanced" {
		t.Errorf("Profile = %q, want balanced", cfg.Profile)
	}
	if verr != "" {
		t.Errorf("Load() validation error = %q, want empty", verr)
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

	cfg, source, _ := Load(dir)

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

	cfg, _, _ := Load(dir)

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

	cfg, _, verr := Load(dir)

	if cfg.Coverage.NewCode != "sometimes" {
		t.Fatalf("Coverage.NewCode = %q, want the invalid value surfaced", cfg.Coverage.NewCode)
	}
	if verr == "" {
		t.Fatal("expected a validation error for unknown coverage.new-code value, got empty")
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

			cfg, _, verr := Load(dir)
			if verr != "" {
				t.Fatalf("Load() validation error = %q, want empty", verr)
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

	cfg, _, _ := Load(dir)

	if cfg.QualityGate.Overall.Coverage != 70 {
		t.Errorf("Overall.Coverage = %f, want 70", cfg.QualityGate.Overall.Coverage)
	}
	if cfg.QualityGate.Overall.Duplications != 8 {
		t.Errorf("Overall.Duplications = %f, want 8", cfg.QualityGate.Overall.Duplications)
	}
}
