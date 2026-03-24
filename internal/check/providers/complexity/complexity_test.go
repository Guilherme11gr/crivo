package complexity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestSeverityForComplexity(t *testing.T) {
	tests := []struct {
		name      string
		actual    int
		threshold int
		want      domain.Severity
	}{
		{"minor - just above threshold", 16, 15, domain.SeverityMinor},
		{"minor - 2x threshold", 30, 15, domain.SeverityMinor},
		{"major - above 2x threshold", 31, 15, domain.SeverityMajor},
		{"major - 4x threshold", 60, 15, domain.SeverityMajor},
		{"critical - above 4x threshold", 61, 15, domain.SeverityCritical},
		{"critical - very high", 100, 15, domain.SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := severityForComplexity(tt.actual, tt.threshold)
			if got != tt.want {
				t.Errorf("severityForComplexity(%d, %d) = %q, want %q", tt.actual, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestBuildResult_NoViolations(t *testing.T) {
	p := New()
	output := astOutput{
		Functions: []astFunction{
			{File: "src/app.ts", Name: "main", Line: 1, Complexity: 5},
			{File: "src/util.ts", Name: "helper", Line: 10, Complexity: 3},
		},
		Summary: astSummary{
			TotalFunctions: 2,
			TotalLines:     50,
			Violations:     0,
			MaxComplexity:  5,
			AvgComplexity:  4.0,
		},
	}

	result := p.buildResult(output, 15, "ast")

	if result.Status != domain.StatusPassed {
		t.Errorf("expected StatusPassed, got %s", result.Status)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
	if result.Metrics["avg_complexity"] != 4.0 {
		t.Errorf("expected avg_complexity=4.0, got %f", result.Metrics["avg_complexity"])
	}
	if result.Metrics["max_complexity"] != 5.0 {
		t.Errorf("expected max_complexity=5.0, got %f", result.Metrics["max_complexity"])
	}
}

func TestBuildResult_WithViolations(t *testing.T) {
	p := New()
	output := astOutput{
		Functions: []astFunction{
			{File: "src/complex.ts", Name: "badFunc", Line: 10, Complexity: 25},
			{File: "src/complex.ts", Name: "okFunc", Line: 50, Complexity: 5},
			{File: "src/worse.ts", Name: "terribleFunc", Line: 1, Complexity: 80},
		},
		Summary: astSummary{
			TotalFunctions: 3,
			TotalLines:     100,
			Violations:     2,
			MaxComplexity:  80,
			AvgComplexity:  36.7,
		},
	}

	result := p.buildResult(output, 15, "ast")

	if result.Status != domain.StatusWarning {
		t.Errorf("expected StatusWarning (<=5 violations), got %s", result.Status)
	}
	if len(result.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(result.Issues))
	}

	// Issues should be sorted by complexity descending
	if result.Issues[0].File != "src/worse.ts" {
		t.Errorf("expected first issue to be worst violation (worse.ts), got %s", result.Issues[0].File)
	}

	// Check issue fields
	issue := result.Issues[0]
	if issue.RuleID != "cognitive-complexity" {
		t.Errorf("expected ruleID 'cognitive-complexity', got %q", issue.RuleID)
	}
	if issue.Source != "complexity-ast" {
		t.Errorf("expected source 'complexity-ast', got %q", issue.Source)
	}
	if issue.Type != domain.IssueTypeCodeSmell {
		t.Errorf("expected IssueTypeCodeSmell, got %q", issue.Type)
	}
}

func TestBuildResult_MoreThan5Violations_StatusFailed(t *testing.T) {
	p := New()
	functions := make([]astFunction, 7)
	for i := range functions {
		functions[i] = astFunction{
			File: "src/bad.ts", Name: "fn" + string(rune('A'+i)), Line: i * 10, Complexity: 20,
		}
	}

	output := astOutput{
		Functions: functions,
		Summary: astSummary{
			TotalFunctions: 7,
			TotalLines:     200,
			MaxComplexity:  20,
			AvgComplexity:  20.0,
		},
	}

	result := p.buildResult(output, 15, "regex")

	if result.Status != domain.StatusFailed {
		t.Errorf("expected StatusFailed for >5 violations, got %s", result.Status)
	}
	if len(result.Issues) != 7 {
		t.Errorf("expected 7 issues, got %d", len(result.Issues))
	}
}

func TestBuildResult_ASTTag(t *testing.T) {
	p := New()
	output := astOutput{
		Summary: astSummary{AvgComplexity: 1.0},
	}

	astResult := p.buildResult(output, 15, "ast")
	regexResult := p.buildResult(output, 15, "regex")

	if got := astResult.Summary; !contains(got, "(AST)") {
		t.Errorf("AST result summary should contain '(AST)', got %q", got)
	}
	if got := regexResult.Summary; contains(got, "(AST)") {
		t.Errorf("regex result summary should not contain '(AST)', got %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestAnalyzeFileRegex_SimpleFunction(t *testing.T) {
	// Create a temp file with a simple JS function
	dir := t.TempDir()
	code := `function simple() {
    const x = 1;
    return x;
}
`
	filePath := filepath.Join(dir, "simple.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, lines := analyzeFileRegex(filePath, dir)

	if lines != 5 {
		t.Errorf("expected 5 lines, got %d", lines)
	}
	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}
	if functions[0].Name != "simple" {
		t.Errorf("expected function name 'simple', got %q", functions[0].Name)
	}
	if functions[0].Complexity != 0 {
		t.Errorf("expected complexity 0 for simple function, got %d", functions[0].Complexity)
	}
}

func TestAnalyzeFileRegex_TernaryAndLogical(t *testing.T) {
	dir := t.TempDir()
	// Ternary operators and logical operators on standalone lines are counted
	code := `function check(x) {
    const result = x > 0 ? "yes" : "no";
    const ok = x && true;
    return result;
}
`
	filePath := filepath.Join(dir, "check.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}
	// ternary = +1, && = +1 => complexity >= 2
	if functions[0].Complexity < 2 {
		t.Errorf("expected complexity >= 2 for ternary + logical, got %d", functions[0].Complexity)
	}
}

func TestAnalyzeFileRegex_CatchBlock(t *testing.T) {
	dir := t.TempDir()
	code := `function risky(x) {
    try {
        doSomething();
    } catch (e) {
        handleError(e);
    }
}
`
	filePath := filepath.Join(dir, "risky.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}
	// catch = +1
	if functions[0].Complexity < 1 {
		t.Errorf("expected complexity >= 1 for catch block, got %d", functions[0].Complexity)
	}
}

func TestAnalyzeFileRegex_LogicalOperatorsOnSeparateLines(t *testing.T) {
	dir := t.TempDir()
	code := `function validate(a, b, c) {
    const x = a && b;
    const y = x || c;
    return x && y;
}
`
	filePath := filepath.Join(dir, "logical.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}
	// 3 lines with logical operators => complexity >= 3
	if functions[0].Complexity < 3 {
		t.Errorf("expected complexity >= 3 for logical operators, got %d", functions[0].Complexity)
	}
}

func TestAnalyzeFileRegex_MultipleFunctions(t *testing.T) {
	dir := t.TempDir()
	code := `function first() {
    return 1;
}

function second(x) {
    if (x) {
        return true;
    }
    return false;
}
`
	filePath := filepath.Join(dir, "multi.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(functions))
	}
	if functions[0].Name != "first" {
		t.Errorf("expected first function 'first', got %q", functions[0].Name)
	}
	if functions[1].Name != "second" {
		t.Errorf("expected second function 'second', got %q", functions[1].Name)
	}
}

func TestAnalyzeFileRegex_GoFunction(t *testing.T) {
	dir := t.TempDir()
	code := `func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		return
	}
	for _, item := range items {
		if item.Valid {
			process(item)
		}
	}
}
`
	filePath := filepath.Join(dir, "handler.go")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) < 1 {
		t.Fatal("expected at least 1 function from Go file")
	}
	if functions[0].Name != "handleRequest" {
		t.Errorf("expected function name 'handleRequest', got %q", functions[0].Name)
	}
}

func TestAnalyzeFileRegex_ArrowFunction(t *testing.T) {
	dir := t.TempDir()
	code := `const greet = (name) => {
    if (name) {
        return "Hello " + name;
    }
    return "Hello";
}
`
	filePath := filepath.Join(dir, "arrow.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) < 1 {
		t.Fatal("expected at least 1 arrow function")
	}
	if functions[0].Name != "greet" {
		t.Errorf("expected function name 'greet', got %q", functions[0].Name)
	}
}

func TestAnalyzeFileRegex_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	code := `function foo() {
    // if (x) { this should not count }
    /* if (y) { neither should this } */
    return 1;
}
`
	filePath := filepath.Join(dir, "commented.js")
	os.WriteFile(filePath, []byte(code), 0644)

	functions, _ := analyzeFileRegex(filePath, dir)

	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}
	if functions[0].Complexity != 0 {
		t.Errorf("expected complexity 0 (comments should be skipped), got %d", functions[0].Complexity)
	}
}

func TestAnalyzeFileRegex_NonexistentFile(t *testing.T) {
	functions, lines := analyzeFileRegex("/nonexistent/file.js", "/nonexistent")

	if len(functions) != 0 {
		t.Errorf("expected 0 functions for nonexistent file, got %d", len(functions))
	}
	if lines != 0 {
		t.Errorf("expected 0 lines for nonexistent file, got %d", lines)
	}
}

func TestDetect(t *testing.T) {
	p := New()

	// Should detect when src/ exists
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "src"), 0755)
	if !p.Detect(nil, dir) {
		t.Error("expected Detect=true when src/ exists")
	}

	// Should not detect when no source dirs exist
	emptyDir := t.TempDir()
	if p.Detect(nil, emptyDir) {
		t.Error("expected Detect=false when no source dirs exist")
	}
}
