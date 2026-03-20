package complexity

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/quality-gate/internal/config"
	"github.com/anthropics/quality-gate/internal/domain"
)

// Provider calculates cognitive complexity using regex-based heuristics
// (simplified version — full accuracy would need tree-sitter)
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

	threshold := 15 // default cognitive complexity threshold

	srcDirs := cfg.Src
	if len(srcDirs) == 0 {
		srcDirs = []string{"src/"}
	}

	var allFunctions []functionComplexity
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

			functions, lines := analyzeFile(path, projectDir)
			allFunctions = append(allFunctions, functions...)
			totalLines += lines
			return nil
		})
		if err != nil && err != context.Canceled {
			continue
		}
	}

	duration := time.Since(start)

	// Find violations
	var issues []domain.Issue
	var details []string
	maxComplexity := 0
	totalComplexity := 0

	for _, fn := range allFunctions {
		totalComplexity += fn.complexity
		if fn.complexity > maxComplexity {
			maxComplexity = fn.complexity
		}

		if fn.complexity > threshold {
			issues = append(issues, domain.Issue{
				RuleID:   "cognitive-complexity",
				Message:  fmt.Sprintf("Function %q has cognitive complexity of %d (max: %d)", fn.name, fn.complexity, threshold),
				File:     fn.file,
				Line:     fn.line,
				Column:   1,
				Severity: domain.SeverityMajor,
				Type:     domain.IssueTypeCodeSmell,
				Source:   "complexity",
				Effort:   fmt.Sprintf("%dmin", fn.complexity*3),
			})
			details = append(details, fmt.Sprintf("%s:%d %s — complexity %d", fn.file, fn.line, fn.name, fn.complexity))
		}
	}

	status := domain.StatusPassed
	if len(issues) > 0 {
		status = domain.StatusWarning
		if len(issues) > 5 {
			status = domain.StatusFailed
		}
	}

	avgComplexity := 0.0
	if len(allFunctions) > 0 {
		avgComplexity = float64(totalComplexity) / float64(len(allFunctions))
	}

	summary := fmt.Sprintf("avg %.1f · max %d · %d violations", avgComplexity, maxComplexity, len(issues))

	return &domain.CheckResult{
		Name:    p.Name(),
		ID:      p.ID(),
		Status:  status,
		Summary: summary,
		Issues:  issues,
		Details: details,
		Duration: duration,
		Metrics: map[string]float64{
			"avg_complexity": avgComplexity,
			"max_complexity": float64(maxComplexity),
			"violations":     float64(len(issues)),
			"total_functions": float64(len(allFunctions)),
			"total_lines":    float64(totalLines),
		},
	}, nil
}

type functionComplexity struct {
	name       string
	file       string
	line       int
	complexity int
}

var (
	funcDeclRe = regexp.MustCompile(`(?:function\s+(\w+)|(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[a-zA-Z_]\w*)\s*=>|(\w+)\s*\([^)]*\)\s*\{|func\s+(?:\([^)]*\)\s*)?(\w+))`)

	// Complexity incrementors
	ifRe       = regexp.MustCompile(`\b(if|else\s+if)\b`)
	elseRe     = regexp.MustCompile(`\belse\b`)
	forRe      = regexp.MustCompile(`\b(for|while|do)\b`)
	switchRe   = regexp.MustCompile(`\bswitch\b`)
	catchRe    = regexp.MustCompile(`\bcatch\b`)
	ternaryRe  = regexp.MustCompile(`\?[^?]`)
	logicalRe  = regexp.MustCompile(`(&&|\|\|)`)
	nestedIfRe = regexp.MustCompile(`\b(if|for|while|switch)\b`)
)

func analyzeFile(path string, projectDir string) ([]functionComplexity, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0
	}

	relPath, _ := filepath.Rel(projectDir, path)
	relPath = filepath.ToSlash(relPath)

	lines := strings.Split(string(data), "\n")
	lineCount := len(lines)

	var functions []functionComplexity
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

		// Skip comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Detect function declarations
		if matches := funcDeclRe.FindStringSubmatch(line); matches != nil {
			// Save previous function
			if inFunction && currentFunc != "" {
				functions = append(functions, functionComplexity{
					name:       currentFunc,
					file:       relPath,
					line:       currentFuncLine,
					complexity: complexity,
				})
			}

			// Find the matched name
			name := ""
			for i := 1; i < len(matches); i++ {
				if matches[i] != "" {
					name = matches[i]
					break
				}
			}
			if name == "" || name == "if" || name == "for" || name == "while" || name == "switch" {
				continue
			}

			currentFunc = name
			currentFuncLine = lineNum
			complexity = 0
			depth = 0
			inFunction = true
			braceCount = 0
		}

		if !inFunction {
			continue
		}

		// Track braces for function boundary
		braceCount += strings.Count(line, "{") - strings.Count(line, "}")

		// Count nesting depth
		openBraces := strings.Count(line, "{")
		if openBraces > 0 && nestedIfRe.MatchString(line) {
			depth++
		}

		// Cognitive complexity increments
		if ifRe.MatchString(trimmed) {
			complexity++ // structural
			complexity += depth // nesting
		}
		if elseRe.MatchString(trimmed) && !strings.Contains(trimmed, "else if") {
			complexity++ // structural
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

		// Logical operators (each sequence of && or || adds 1)
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

		// Function ended
		if braceCount <= 0 && lineNum > currentFuncLine {
			functions = append(functions, functionComplexity{
				name:       currentFunc,
				file:       relPath,
				line:       currentFuncLine,
				complexity: complexity,
			})
			inFunction = false
			currentFunc = ""
		}
	}

	// Handle last function
	if inFunction && currentFunc != "" {
		functions = append(functions, functionComplexity{
			name:       currentFunc,
			file:       relPath,
			line:       currentFuncLine,
			complexity: complexity,
		})
	}

	// Deduplicate (regex might double-match)
	seen := map[string]bool{}
	var deduped []functionComplexity
	for _, fn := range functions {
		key := fn.file + ":" + strconv.Itoa(fn.line) + ":" + fn.name
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, fn)
		}
	}

	return deduped, lineCount
}
