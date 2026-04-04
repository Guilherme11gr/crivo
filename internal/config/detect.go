package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DetectProject analyzes a project directory and returns an optimized Config
func DetectProject(projectDir string) *Config {
	cfg := DefaultConfig()

	detected := &detection{
		projectDir: projectDir,
		cfg:        cfg,
	}

	detectLanguages(detected)
	detectFrameworks(detected)
	detectSrcDirs(detected)
	detectExcludes(detected)
	detectTestRunner(detected)
	detectAvailableTools(detected)
	applySmartDefaults(detected)

	return cfg
}

type detection struct {
	projectDir string
	cfg        *Config
	isGo       bool
	isPython   bool
	isRust     bool
	isNode     bool
	isNext     bool
	isReact    bool
	isVue      bool
	isSvelte   bool
	isRemix    bool
	hasTS      bool
	hasJest    bool
	hasVitest  bool
	hasESLint  bool
	hasTestRunner bool
}

func detectLanguages(d *detection) {
	if fileExists(d.projectDir, "go.mod") {
		d.isGo = true
		d.cfg.Languages = []string{"go"}
	}
	if fileExists(d.projectDir, "pyproject.toml") || fileExists(d.projectDir, "requirements.txt") {
		d.isPython = true
		d.cfg.Languages = []string{"python"}
	}
	if fileExists(d.projectDir, "Cargo.toml") {
		d.isRust = true
		d.cfg.Languages = []string{"rust"}
	}
	if fileExists(d.projectDir, "package.json") {
		d.isNode = true
	}
	if fileExists(d.projectDir, "tsconfig.json") {
		d.hasTS = true
	}
}

func detectFrameworks(d *detection) {
	if !d.isNode {
		return
	}

	deps := readPackageJSONDeps(d.projectDir)

	if contains(deps, "next") {
		d.isNext = true
	}
	if contains(deps, "react") && !d.isNext {
		d.isReact = true
	}
	if contains(deps, "vue") {
		d.isVue = true
	}
	if contains(deps, "svelte") {
		d.isSvelte = true
	}
	if contains(deps, "@remix-run/react") {
		d.isRemix = true
	}
}

func detectSrcDirs(d *detection) {
	srcDirs := []string{}
	candidates := []string{"src/", "app/", "lib/", "packages/", "internal/", "pkg/"}

	for _, dir := range candidates {
		if dirExists(d.projectDir, dir) {
			srcDirs = append(srcDirs, dir)
		}
	}

	if len(srcDirs) > 0 {
		d.cfg.Src = srcDirs
	}
}

func detectExcludes(d *detection) {
	excludes := []string{}

	alwaysExclude := []string{"node_modules/", "*.generated.*", "*.min.js"}
	for _, pat := range alwaysExclude {
		excludes = append(excludes, pat)
	}

	conditionalExclude := []string{".next/", "dist/", "build/", "coverage/", "out/", ".nuxt/", ".svelte-kit/"}
	for _, dir := range conditionalExclude {
		if dirExists(d.projectDir, dir) {
			excludes = append(excludes, dir)
		}
	}

	d.cfg.Exclude = excludes
}

func detectTestRunner(d *detection) {
	hasJest := hasJestConfig(d.projectDir)
	hasVitest := hasVitestConfig(d.projectDir)

	if hasJest {
		d.hasJest = true
		d.hasTestRunner = true
	}
	if hasVitest {
		d.hasVitest = true
		d.hasTestRunner = true
	}
}

func detectAvailableTools(d *detection) {
	if _, err := exec.LookPath("semgrep"); err == nil {
		d.cfg.Checks.Semgrep = true
	}
	if _, err := exec.LookPath("gitleaks"); err == nil {
		d.cfg.Checks.Secrets = true
	}
	if _, err := exec.LookPath("npx"); err == nil {
		d.cfg.Checks.DeadCode = true
	}
}

func applySmartDefaults(d *detection) {
	if d.isGo || d.isPython || d.isRust {
		d.cfg.Checks.Typescript = false
		d.cfg.Checks.Coverage = false
		d.cfg.Checks.Duplication = false
		d.cfg.Checks.DeadCode = false
		d.cfg.Checks.Secrets = true
		d.cfg.Checks.CustomRules = true
		d.cfg.Complexity.Threshold = 15
		return
	}

	if d.isNext {
		d.cfg.Checks.Duplication = true
		d.cfg.Duplication.Semantic = true
		d.cfg.Complexity.Threshold = 15
		d.cfg.Coverage.Lines = 60
		d.cfg.Coverage.Branches = 50
		d.cfg.Coverage.Functions = 60
		d.cfg.Coverage.Statements = 60
		if !d.hasTestRunner {
			d.cfg.Checks.Coverage = false
		}
		return
	}

	if d.isReact || d.isVue || d.isSvelte || d.isRemix {
		d.cfg.Checks.Duplication = true
		d.cfg.Duplication.Semantic = true
		d.cfg.Complexity.Threshold = 15
		d.cfg.Coverage.Lines = 60
		d.cfg.Coverage.Branches = 50
		d.cfg.Coverage.Functions = 60
		d.cfg.Coverage.Statements = 60
		if !d.hasTestRunner {
			d.cfg.Checks.Coverage = false
		}
		return
	}

	if d.isNode {
		if !d.hasTestRunner {
			d.cfg.Checks.Coverage = false
		}
		return
	}
}

func hasJestConfig(projectDir string) bool {
	patterns := []string{
		"jest.config.js",
		"jest.config.ts",
		"jest.config.mjs",
		"jest.config.cjs",
	}
	for _, p := range patterns {
		if fileExists(projectDir, p) {
			return true
		}
	}

	deps := readPackageJSONDeps(projectDir)
	if contains(deps, "jest") {
		return true
	}

	pkgJSONPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return false
	}

	var pkg struct {
		Jest *json.RawMessage `json:"jest"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	return pkg.Jest != nil
}

func hasVitestConfig(projectDir string) bool {
	patterns := []string{
		"vitest.config.js",
		"vitest.config.ts",
		"vitest.config.mjs",
		"vitest.config.cjs",
	}
	for _, p := range patterns {
		if fileExists(projectDir, p) {
			return true
		}
	}

	deps := readPackageJSONDeps(projectDir)
	return contains(deps, "vitest")
}

func readPackageJSONDeps(projectDir string) map[string]bool {
	deps := map[string]bool{}

	pkgJSONPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return deps
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return deps
	}

	for name := range pkg.Dependencies {
		deps[name] = true
	}
	for name := range pkg.DevDependencies {
		deps[name] = true
	}

	return deps
}

func fileExists(projectDir, name string) bool {
	_, err := os.Stat(filepath.Join(projectDir, name))
	return err == nil
}

func dirExists(projectDir, name string) bool {
	info, err := os.Stat(filepath.Join(projectDir, name))
	return err == nil && info.IsDir()
}

func contains(m map[string]bool, key string) bool {
	_, ok := m[key]
	return ok
}

// BuildDetectionSummary returns a human-readable summary of what was detected
func BuildDetectionSummary(projectDir string) string {
	parts := []string{}

	if fileExists(projectDir, "tsconfig.json") {
		parts = append(parts, "TypeScript")
	}

	if fileExists(projectDir, "package.json") {
		deps := readPackageJSONDeps(projectDir)
		if contains(deps, "next") {
			parts = append(parts, "Next.js")
		} else if contains(deps, "react") {
			parts = append(parts, "React")
		} else if contains(deps, "vue") {
			parts = append(parts, "Vue")
		} else if contains(deps, "svelte") {
			parts = append(parts, "Svelte")
		} else if contains(deps, "@remix-run/react") {
			parts = append(parts, "Remix")
		} else {
			parts = append(parts, "Node.js")
		}
	}

	if fileExists(projectDir, "go.mod") {
		parts = append(parts, "Go")
	}
	if fileExists(projectDir, "pyproject.toml") || fileExists(projectDir, "requirements.txt") {
		parts = append(parts, "Python")
	}
	if fileExists(projectDir, "Cargo.toml") {
		parts = append(parts, "Rust")
	}

	if hasJestConfig(projectDir) {
		parts = append(parts, "Jest")
	} else if hasVitestConfig(projectDir) {
		parts = append(parts, "Vitest")
	}

	if len(parts) == 0 {
		return "generic project"
	}

	return strings.Join(parts, ", ")
}

// GenerateDetected returns an optimized YAML config based on project analysis
func GenerateDetected(projectDir string) ([]byte, error) {
	cfg := DetectProject(projectDir)
	return yaml.Marshal(cfg)
}
