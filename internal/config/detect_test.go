package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writePackageJSON(dir string, deps, devDeps map[string]string) error {
	pkg := struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}{
		Dependencies:    deps,
		DevDependencies: devDeps,
	}
	data, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
}

func TestDetectProject_TypeScriptNextJest(t *testing.T) {
	dir := t.TempDir()

	deps := map[string]string{
		"next":  "^14.0.0",
		"react": "^18.0.0",
	}
	devDeps := map[string]string{
		"jest": "^29.0.0",
	}
	if err := writePackageJSON(dir, deps, devDeps); err != nil {
		t.Fatal(err)
	}

	tsconfig := `{ "compilerOptions": {} }`
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0644); err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	nextDir := filepath.Join(dir, ".next")
	if err := os.MkdirAll(nextDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := DetectProject(dir)

	if cfg.Languages[0] != "typescript" {
		t.Errorf("Languages = %v, want typescript", cfg.Languages)
	}

	found := false
	for _, s := range cfg.Src {
		if s == "src/" {
			found = true
			break
		}
	}
	if !found {
		t.Error("src/ should be in Src")
	}

	foundNext := false
	for _, e := range cfg.Exclude {
		if e == ".next/" {
			foundNext = true
			break
		}
	}
	if !foundNext {
		t.Error(".next/ should be in Exclude")
	}

	if !cfg.Checks.Typescript {
		t.Error("Typescript check should be enabled")
	}
}

func TestDetectProject_Go(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21"), 0644); err != nil {
		t.Fatal(err)
	}

	internalDir := filepath.Join(dir, "internal")
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := DetectProject(dir)

	if cfg.Languages[0] != "go" {
		t.Errorf("Languages = %v, want go", cfg.Languages)
	}

	if cfg.Checks.Typescript {
		t.Error("Typescript should be disabled for Go projects")
	}
	if cfg.Checks.Coverage {
		t.Error("Coverage should be disabled for Go projects")
	}
	if cfg.Checks.Duplication {
		t.Error("Duplication should be disabled for Go projects")
	}
	if !cfg.Checks.Secrets {
		t.Error("Secrets should be enabled for Go projects")
	}
	if !cfg.Checks.CustomRules {
		t.Error("CustomRules should be enabled for Go projects")
	}

	found := false
	for _, s := range cfg.Src {
		if s == "internal/" {
			found = true
			break
		}
	}
	if !found {
		t.Error("internal/ should be in Src")
	}
}

func TestDetectProject_Fallback(t *testing.T) {
	dir := t.TempDir()

	cfg := DetectProject(dir)

	if cfg.Profile != "balanced" {
		t.Errorf("Profile = %q, want balanced", cfg.Profile)
	}
}

func TestDetectProject_SrcDirs(t *testing.T) {
	dir := t.TempDir()

	dirs := []string{"src", "lib", "packages"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DetectProject(dir)

	expected := map[string]bool{
		"src/":      true,
		"lib/":      true,
		"packages/": true,
	}

	for _, s := range cfg.Src {
		delete(expected, s)
	}

	if len(expected) > 0 {
		t.Errorf("Missing src dirs: %v", expected)
	}
}

func TestDetectProject_ExcludePatterns(t *testing.T) {
	dir := t.TempDir()

	dirs := []string{".next", "dist", "build", "coverage"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DetectProject(dir)

	excludeSet := map[string]bool{}
	for _, e := range cfg.Exclude {
		excludeSet[e] = true
	}

	required := []string{"node_modules/", "*.generated.*", "*.min.js", ".next/", "dist/", "build/", "coverage/"}
	for _, r := range required {
		if !excludeSet[r] {
			t.Errorf("Exclude should contain %q", r)
		}
	}
}

func TestDetectProject_AvailableTools(t *testing.T) {
	// Tool detection uses exec.LookPath, so this test just verifies
	// that a Go project with tools available has the right checks enabled.
	// The actual tool detection is integration-tested by `crivo init`.
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DetectProject(dir)

	// For Go projects, secrets and custom-rules should always be enabled
	if !cfg.Checks.Secrets {
		t.Error("Secrets should be enabled for Go projects")
	}
	if !cfg.Checks.CustomRules {
		t.Error("CustomRules should be enabled for Go projects")
	}
	// Semgrep/gitleaks/npx are checked via exec.LookPath —
	// they may or may not be present in the test environment
}

func TestDetectProject_Python(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DetectProject(dir)

	if cfg.Languages[0] != "python" {
		t.Errorf("Languages = %v, want python", cfg.Languages)
	}

	if cfg.Checks.Typescript {
		t.Error("Typescript should be disabled for Python projects")
	}
	if !cfg.Checks.Secrets {
		t.Error("Secrets should be enabled for Python projects")
	}
}

func TestDetectProject_Rust(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DetectProject(dir)

	if cfg.Languages[0] != "rust" {
		t.Errorf("Languages = %v, want rust", cfg.Languages)
	}

	if cfg.Checks.Typescript {
		t.Error("Typescript should be disabled for Rust projects")
	}
}

func TestBuildDetectionSummary_NextJS(t *testing.T) {
	dir := t.TempDir()

	deps := map[string]string{
		"next":  "^14.0.0",
		"react": "^18.0.0",
	}
	if err := writePackageJSON(dir, deps, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	summary := BuildDetectionSummary(dir)

	if summary == "" {
		t.Error("Summary should not be empty")
	}
}

func TestBuildDetectionSummary_Generic(t *testing.T) {
	dir := t.TempDir()

	summary := BuildDetectionSummary(dir)

	if summary != "generic project" {
		t.Errorf("Summary = %q, want 'generic project'", summary)
	}
}

func TestGenerateDetected(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21"), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := GenerateDetected(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("GenerateDetected returned empty data")
	}
}
