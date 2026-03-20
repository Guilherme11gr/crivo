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
	if !cfg.Checks.ESLint {
		t.Error("Checks.ESLint should be true by default")
	}
	if cfg.Coverage.Lines != 60 {
		t.Errorf("Coverage.Lines = %f, want 60", cfg.Coverage.Lines)
	}
}

func TestLoad_NoConfig(t *testing.T) {
	dir := t.TempDir()
	cfg, source := Load(dir)

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

	cfg, source := Load(dir)

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
  eslint: false
`
	err := os.WriteFile(filepath.Join(dir, ".qualitygate.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, _ := Load(dir)

	if cfg.Coverage.Lines != 75 {
		t.Errorf("Coverage.Lines = %f, want 75", cfg.Coverage.Lines)
	}
	if cfg.Checks.ESLint != false {
		t.Error("ESLint should be false after override")
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
