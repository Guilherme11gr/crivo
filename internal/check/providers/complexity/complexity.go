package complexity

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

//go:embed cognitive.js
var cognitiveJS []byte

// Provider calculates cognitive complexity.
// Uses AST analysis (TypeScript compiler API) for JS/TS projects,
// falls back to regex heuristics for Go/Python or when node is unavailable.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "Complexity" }
func (p *Provider) ID() string   { return "complexity" }

func (p *Provider) Detect(_ context.Context, projectDir string) bool {
	for _, src := range []string{"src", "lib", "app"} {
		if _, err := os.Stat(filepath.Join(projectDir, src)); err == nil {
			return true
		}
	}
	return false
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
	start := time.Now()
	threshold := cfg.Complexity.Threshold
	if threshold <= 0 {
		threshold = 15
	}

	// Try AST-based analysis first (for JS/TS projects with node + typescript)
	result, astErr := p.analyzeAST(ctx, projectDir, cfg, threshold)
	if astErr == nil {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Fallback to regex heuristics
	return p.analyzeRegex(ctx, projectDir, cfg, threshold, start)
}

// ---------------------------------------------------------------------------
// AST-based analysis (JS/TS) — uses embedded cognitive.js
// ---------------------------------------------------------------------------

type astOutput struct {
	Functions []astFunction `json:"functions"`
	Summary   astSummary    `json:"summary"`
	Error     string        `json:"error,omitempty"`
}

type astFunction struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	Line       int    `json:"line"`
	Complexity int    `json:"complexity"`
}

type astSummary struct {
	TotalFunctions int     `json:"totalFunctions"`
	TotalLines     int     `json:"totalLines"`
	Violations     int     `json:"violations"`
	MaxComplexity  int     `json:"maxComplexity"`
	AvgComplexity  float64 `json:"avgComplexity"`
}

func (p *Provider) analyzeAST(ctx context.Context, projectDir string, cfg *config.Config, threshold int) (*domain.CheckResult, error) {
	// Check prerequisites: node + typescript in node_modules
	nodeBin := findNode()
	if nodeBin == "" {
		return nil, fmt.Errorf("node not found")
	}
	tsPath := filepath.Join(projectDir, "node_modules", "typescript")
	if _, err := os.Stat(tsPath); err != nil {
		return nil, fmt.Errorf("typescript not in node_modules")
	}

	// Write embedded script to temp
	tmpDir, err := os.MkdirTemp("", "crivo-complexity-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "cognitive.js")
	if err := os.WriteFile(scriptPath, cognitiveJS, 0644); err != nil {
		return nil, err
	}

	// Determine source directory
	srcDir := "src/"
	if len(cfg.Src) > 0 {
		srcDir = cfg.Src[0]
	}
	dir := filepath.Join(projectDir, srcDir)

	args := []string{scriptPath, dir, fmt.Sprintf("--threshold=%d", threshold)}
	if len(cfg.Exclude) > 0 {
		args = append(args, "--exclude="+strings.Join(cfg.Exclude, ","))
	}

	cmd := exec.CommandContext(ctx, nodeBin, args...)
	cmd.Dir = projectDir
	// Ensure typescript is resolvable
	cmd.Env = append(os.Environ(), "NODE_PATH="+filepath.Join(projectDir, "node_modules"))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("node: %s (stderr: %s)", err, stderr.String())
	}

	var output astOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("parse ast output: %w", err)
	}
	if output.Error != "" {
		return nil, fmt.Errorf("cognitive.js: %s", output.Error)
	}

	// Fix paths: cognitive.js outputs paths relative to the scanned directory,
	// but we need them relative to projectDir (i.e., prefixed with srcDir).
	srcPrefix := filepath.ToSlash(srcDir)
	if !strings.HasSuffix(srcPrefix, "/") {
		srcPrefix += "/"
	}
	for i := range output.Functions {
		output.Functions[i].File = srcPrefix + output.Functions[i].File
	}

	return p.buildResult(output, threshold, "ast"), nil
}

func (p *Provider) buildResult(output astOutput, threshold int, method string) *domain.CheckResult {
	// Sort violations by complexity descending
	var violations []astFunction
	for _, fn := range output.Functions {
		if fn.Complexity > threshold {
			violations = append(violations, fn)
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		return violations[i].Complexity > violations[j].Complexity
	})

	var issues []domain.Issue
	var details []string
	for _, fn := range violations {
		issues = append(issues, domain.Issue{
			RuleID:      "cognitive-complexity",
			Message:     fmt.Sprintf("Function %q has cognitive complexity of %d (max: %d)", fn.Name, fn.Complexity, threshold),
			File:        fn.File,
			Line:        fn.Line,
			Column:      1,
			Severity:    severityForComplexity(fn.Complexity, threshold),
			Type:        domain.IssueTypeCodeSmell,
			Source:      "complexity-" + method,
			Effort:      fmt.Sprintf("%dmin", fn.Complexity*3),
			Remediation: domain.ComplexityRemediation(""),
		})
		details = append(details, fmt.Sprintf("%s:%d %s — complexity %d", fn.File, fn.Line, fn.Name, fn.Complexity))
	}

	status := domain.StatusPassed
	if len(issues) > 0 {
		status = domain.StatusWarning
		if len(issues) > 5 {
			status = domain.StatusFailed
		}
	}

	tag := ""
	if method == "ast" {
		tag = " (AST)"
	}
	summary := fmt.Sprintf("avg %.1f · max %d · %d violations%s",
		output.Summary.AvgComplexity, output.Summary.MaxComplexity, len(violations), tag)

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: summary,
		Issues:  issues,
		Details: details,
		Metrics: map[string]float64{
			"avg_complexity":  output.Summary.AvgComplexity,
			"max_complexity":  float64(output.Summary.MaxComplexity),
			"violations":      float64(len(violations)),
			"total_functions": float64(output.Summary.TotalFunctions),
			"total_lines":     float64(output.Summary.TotalLines),
		},
	}
}

func severityForComplexity(actual, threshold int) domain.Severity {
	ratio := float64(actual) / float64(threshold)
	if ratio > 4 {
		return domain.SeverityCritical
	}
	if ratio > 2 {
		return domain.SeverityMajor
	}
	return domain.SeverityMinor
}

// findNode returns the path to the node binary, checking PATH and common install locations.
func findNode() string {
	// Try PATH first
	if p, err := exec.LookPath("node"); err == nil {
		return p
	}

	// On Windows, node may not be in the Go process PATH.
	// Check common install locations.
	if runtime.GOOS == "windows" {
		candidates := []string{
			filepath.Join(os.Getenv("ProgramFiles"), "nodejs", "node.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "fnm_multishells", "node.exe"),
			filepath.Join(os.Getenv("APPDATA"), "nvm", "current", "node.exe"),
			filepath.Join(os.Getenv("USERPROFILE"), ".nvm", "current", "node.exe"),
		}

		// Also check NVM_HOME, NVM_SYMLINK
		if nvmHome := os.Getenv("NVM_HOME"); nvmHome != "" {
			// List version dirs inside NVM_HOME
			entries, _ := os.ReadDir(nvmHome)
			for _, e := range entries {
				if e.IsDir() {
					candidates = append(candidates, filepath.Join(nvmHome, e.Name(), "node.exe"))
				}
			}
		}
		if nvmLink := os.Getenv("NVM_SYMLINK"); nvmLink != "" {
			candidates = append(candidates, filepath.Join(nvmLink, "node.exe"))
		}

		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	// macOS / Linux: check common paths
	for _, p := range []string{"/usr/local/bin/node", "/usr/bin/node"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try .local share paths (fnm, nvm, volta)
	home, _ := os.UserHomeDir()
	if home != "" {
		candidates := []string{
			filepath.Join(home, ".nvm", "current", "bin", "node"),
			filepath.Join(home, ".local", "share", "fnm", "node-versions"),
			filepath.Join(home, ".volta", "bin", "node"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	return ""
}

// ---------------------------------------------------------------------------
// Regex-based fallback (Go, Python, or when node is unavailable)
// ---------------------------------------------------------------------------

func (p *Provider) analyzeRegex(ctx context.Context, projectDir string, cfg *config.Config, threshold int, start time.Time) (*domain.CheckResult, error) {
	srcDirs := cfg.Src
	if len(srcDirs) == 0 {
		srcDirs = []string{"src/"}
	}

	var allFunctions []astFunction
	totalLines := 0

	for _, srcDir := range srcDirs {
		dir := filepath.Join(projectDir, srcDir)
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if base == "node_modules" || base == ".next" || base == "dist" || base == "coverage" {
					return filepath.SkipDir
				}
				return nil
			}

			ext := filepath.Ext(path)
			if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" && ext != ".go" && ext != ".py" {
				return nil
			}

			functions, lines := analyzeFileRegex(path, projectDir)
			allFunctions = append(allFunctions, functions...)
			totalLines += lines
			return nil
		})
		if err != nil && err != context.Canceled {
			continue
		}
	}

	// Build summary
	maxComplexity := 0
	totalComplexity := 0
	violations := 0
	for _, fn := range allFunctions {
		totalComplexity += fn.Complexity
		if fn.Complexity > maxComplexity {
			maxComplexity = fn.Complexity
		}
		if fn.Complexity > threshold {
			violations++
		}
	}

	avgComplexity := 0.0
	if len(allFunctions) > 0 {
		avgComplexity = float64(totalComplexity) / float64(len(allFunctions))
	}

	output := astOutput{
		Functions: allFunctions,
		Summary: astSummary{
			TotalFunctions: len(allFunctions),
			TotalLines:     totalLines,
			Violations:     violations,
			MaxComplexity:  maxComplexity,
			AvgComplexity:  avgComplexity,
		},
	}

	result := p.buildResult(output, threshold, "regex")
	result.Duration = time.Since(start)
	return result, nil
}

// ---------------------------------------------------------------------------
// Regex analyzer (unchanged logic, adapted to new types)
// ---------------------------------------------------------------------------

var (
	funcDeclRe = regexp.MustCompile(`(?:function\s+(\w+)|(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[a-zA-Z_]\w*)\s*=>|(\w+)\s*\([^)]*\)\s*\{|func\s+(?:\([^)]*\)\s*)?(\w+))`)

	ifRe       = regexp.MustCompile(`\b(if|else\s+if)\b`)
	elseRe     = regexp.MustCompile(`\belse\b`)
	forRe      = regexp.MustCompile(`\b(for|while|do)\b`)
	switchRe   = regexp.MustCompile(`\bswitch\b`)
	catchRe    = regexp.MustCompile(`\bcatch\b`)
	ternaryRe  = regexp.MustCompile(`\?[^?]`)
	logicalRe  = regexp.MustCompile(`(&&|\|\|)`)
	nestedIfRe = regexp.MustCompile(`\b(if|for|while|switch)\b`)
)

func analyzeFileRegex(path string, projectDir string) ([]astFunction, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0
	}

	relPath, _ := filepath.Rel(projectDir, path)
	relPath = filepath.ToSlash(relPath)

	lines := strings.Split(string(data), "\n")
	lineCount := len(lines)

	var functions []astFunction
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0
	currentFunc := ""
	currentFuncLine := 0
	complexity := 0
	depth := 0
	inFunction := false
	braceCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if matches := funcDeclRe.FindStringSubmatch(line); matches != nil {
			name := ""
			for i := 1; i < len(matches); i++ {
				if matches[i] != "" {
					name = matches[i]
					break
				}
			}
			// Skip keywords that false-match as function declarations
			if name == "" || name == "if" || name == "for" || name == "while" || name == "switch" || name == "catch" || name == "return" {
				goto skipFuncDetection
			}

			if inFunction && currentFunc != "" {
				functions = append(functions, astFunction{
					Name:       currentFunc,
					File:       relPath,
					Line:       currentFuncLine,
					Complexity: complexity,
				})
			}

			currentFunc = name
			currentFuncLine = lineNum
			complexity = 0
			depth = 0
			inFunction = true
			braceCount = 0
		}
	skipFuncDetection:

		if !inFunction {
			continue
		}

		braceCount += strings.Count(line, "{") - strings.Count(line, "}")

		openBraces := strings.Count(line, "{")
		if openBraces > 0 && nestedIfRe.MatchString(line) {
			depth++
		}

		if ifRe.MatchString(trimmed) {
			complexity++
			complexity += depth
		}
		if elseRe.MatchString(trimmed) && !strings.Contains(trimmed, "else if") {
			complexity++
		}
		if forRe.MatchString(trimmed) {
			complexity++
			complexity += depth
		}
		if switchRe.MatchString(trimmed) {
			complexity++
			complexity += depth
		}
		if catchRe.MatchString(trimmed) {
			complexity++
		}
		if ternaryRe.MatchString(trimmed) {
			complexity++
			complexity += depth
		}

		logicals := logicalRe.FindAllString(trimmed, -1)
		if len(logicals) > 0 {
			complexity += len(logicals)
		}

		closeBraces := strings.Count(line, "}")
		for range closeBraces {
			if depth > 0 {
				depth--
			}
		}

		if braceCount <= 0 && lineNum > currentFuncLine {
			functions = append(functions, astFunction{
				Name:       currentFunc,
				File:       relPath,
				Line:       currentFuncLine,
				Complexity: complexity,
			})
			inFunction = false
			currentFunc = ""
		}
	}

	if inFunction && currentFunc != "" {
		functions = append(functions, astFunction{
			Name:       currentFunc,
			File:       relPath,
			Line:       currentFuncLine,
			Complexity: complexity,
		})
	}

	// Deduplicate
	seen := map[string]bool{}
	var deduped []astFunction
	for _, fn := range functions {
		key := fn.File + ":" + strconv.Itoa(fn.Line) + ":" + fn.Name
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, fn)
		}
	}

	return deduped, lineCount
}
