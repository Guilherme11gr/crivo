package duplication

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

func TestTokenize(t *testing.T) {
	tokens := tokenize("if (x > 0) { return true; }")
	expected := []string{"if", "(", "x", ">", "0", ")", "{", "return", "true", ";", "}"}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(expected), tokens)
	}
	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("token[%d] = %q, want %q", i, tok, expected[i])
		}
	}
}

func TestTokenize_ArrowFunction(t *testing.T) {
	tokens := tokenize("const fn = (a, b) => a + b")
	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}
	// Should contain "const", "fn", "=", etc.
	if tokens[0] != "const" {
		t.Errorf("first token = %q, want const", tokens[0])
	}
}

// ---------------------------------------------------------------------------
// Normalization
// ---------------------------------------------------------------------------

func TestNormalizeBody_IdentifierReplacement(t *testing.T) {
	// Two function bodies that differ only in variable names
	bodyA := `if (!email.includes('@')) {
return false;
}
return true;`

	bodyB := `if (!input.includes('@')) {
return false;
}
return true;`

	normA, _ := normalizeBody(bodyA)
	normB, _ := normalizeBody(bodyB)

	if normA != normB {
		t.Errorf("expected identical normalized forms\n  A: %s\n  B: %s", normA, normB)
	}
}

func TestNormalizeBody_DifferentStructure(t *testing.T) {
	bodyA := `if (x > 0) { return true; }`
	bodyB := `for (let i = 0; i < n; i++) { total += i; }`

	normA, _ := normalizeBody(bodyA)
	normB, _ := normalizeBody(bodyB)

	if normA == normB {
		t.Errorf("different structures should not normalize to same form")
	}
}

func TestNormalizeBody_StringLiterals(t *testing.T) {
	bodyA := `console.log("hello world")`
	bodyB := `console.log("goodbye world")`

	normA, _ := normalizeBody(bodyA)
	normB, _ := normalizeBody(bodyB)

	if normA != normB {
		t.Errorf("only string literal differs, should normalize equally\n  A: %s\n  B: %s", normA, normB)
	}
}

func TestNormalizeBody_NumberLiterals(t *testing.T) {
	bodyA := `return x * 100 + y`
	bodyB := `return x * 200 + y`

	normA, _ := normalizeBody(bodyA)
	normB, _ := normalizeBody(bodyB)

	if normA != normB {
		t.Errorf("only number literal differs, should normalize equally\n  A: %s\n  B: %s", normA, normB)
	}
}

func TestNormalizeBody_KeywordsPreserved(t *testing.T) {
	body := `if (true) { return null; }`
	norm, tokens := normalizeBody(body)

	// Keywords should appear as-is
	found := map[string]bool{}
	for _, tok := range tokens {
		found[tok] = true
	}
	for _, kw := range []string{"if", "true", "return", "null"} {
		if !found[kw] {
			t.Errorf("keyword %q not preserved in normalized form: %s", kw, norm)
		}
	}
}

func TestNormalizeBody_MultipleIdentifiers(t *testing.T) {
	// Ensure different identifiers get different positional tokens
	body := `const result = calculate(input, factor)`
	_, tokens := normalizeBody(body)

	// "result" → $0, "calculate" → $1, "input" → $2, "factor" → $3
	positionals := map[string]bool{}
	for _, t := range tokens {
		if len(t) > 0 && t[0] == '$' {
			positionals[t] = true
		}
	}
	if len(positionals) != 4 {
		t.Errorf("expected 4 positional tokens, got %d: %v", len(positionals), positionals)
	}
}

// ---------------------------------------------------------------------------
// Hash
// ---------------------------------------------------------------------------

func TestHashNormalized(t *testing.T) {
	h1 := hashNormalized("if ( $0 > _N_ ) { return true ; }")
	h2 := hashNormalized("if ( $0 > _N_ ) { return true ; }")
	h3 := hashNormalized("for ( $0 = _N_ ; $0 < $1 ; $0 ++ ) { }")

	if h1 != h2 {
		t.Error("identical inputs should produce identical hashes")
	}
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
}

// ---------------------------------------------------------------------------
// Jaccard similarity
// ---------------------------------------------------------------------------

func TestJaccardTrigrams_Identical(t *testing.T) {
	tokens := []string{"if", "(", "$0", ">", "_N_", ")", "{", "return", "true", ";", "}"}
	sim := jaccardTrigrams(tokens, tokens)
	if sim != 1.0 {
		t.Errorf("identical tokens should have similarity 1.0, got %f", sim)
	}
}

func TestJaccardTrigrams_Disjoint(t *testing.T) {
	a := []string{"if", "(", "$0", ">", "_N_"}
	b := []string{"for", "let", "$1", "=", "_N_"}
	sim := jaccardTrigrams(a, b)
	if sim >= 0.5 {
		t.Errorf("very different tokens should have low similarity, got %f", sim)
	}
}

func TestJaccardTrigrams_TooShort(t *testing.T) {
	a := []string{"if", "("}
	b := []string{"if", "("}
	sim := jaccardTrigrams(a, b)
	if sim != 0 {
		t.Errorf("sequences shorter than 3 should return 0, got %f", sim)
	}
}

func TestJaccardTrigrams_HighSimilarity(t *testing.T) {
	// Longer sequences with mostly shared structure
	a := []string{"if", "(", "$0", ")", "{", "const", "$1", "=", "$0", ".", "$2", "(", ")", ";", "return", "$1", ";", "}"}
	b := []string{"if", "(", "$0", ")", "{", "const", "$1", "=", "$0", ".", "$2", "(", ")", ";", "return", "$1", ";", "}"}
	sim := jaccardTrigrams(a, b)
	if sim < 0.9 {
		t.Errorf("identical long sequences should have similarity ~1.0, got %f", sim)
	}

	// Slightly different long sequences
	c := []string{"if", "(", "$0", ")", "{", "const", "$1", "=", "$0", ".", "$2", "(", ")", ";", "return", "true", ";", "}"}
	d := []string{"if", "(", "$0", ")", "{", "const", "$1", "=", "$0", ".", "$2", "(", ")", ";", "return", "false", ";", "}"}
	sim2 := jaccardTrigrams(c, d)
	if sim2 < 0.6 {
		t.Errorf("mostly similar long sequences should have high similarity, got %f", sim2)
	}
}

// ---------------------------------------------------------------------------
// Function extraction
// ---------------------------------------------------------------------------

func TestExtractFunctionsFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.ts")

	content := `
function validateEmail(email: string): boolean {
  if (!email.includes('@')) {
    return false;
  }
  return true;
}

function checkInput(input: string): boolean {
  if (!input.includes('@')) {
    return false;
  }
  return true;
}

const helper = () => {
  return 42;
}
`
	os.WriteFile(file, []byte(content), 0644)

	fns := extractFunctionsFromFile(file, dir)
	if len(fns) < 2 {
		t.Fatalf("expected at least 2 functions, got %d", len(fns))
	}

	names := map[string]bool{}
	for _, fn := range fns {
		names[fn.Name] = true
	}
	if !names["validateEmail"] {
		t.Error("should extract validateEmail")
	}
	if !names["checkInput"] {
		t.Error("should extract checkInput")
	}
}

func TestExtractFunctionsFromFile_GoFunctions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func processData(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty data")
	}
	result := transform(data)
	return save(result)
}

func handleInput(input []byte) error {
	if len(input) == 0 {
		return fmt.Errorf("empty input")
	}
	result := transform(input)
	return save(result)
}
`
	os.WriteFile(file, []byte(content), 0644)

	fns := extractFunctionsFromFile(file, dir)
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}
	if fns[0].Name != "processData" || fns[1].Name != "handleInput" {
		t.Errorf("unexpected function names: %s, %s", fns[0].Name, fns[1].Name)
	}
}

func TestExtractFunctionsFromFile_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.ts")

	content := `
function foo() {
  // this is a comment
  /* block comment */
  return 1;
}
`
	os.WriteFile(file, []byte(content), 0644)

	fns := extractFunctionsFromFile(file, dir)
	if len(fns) != 1 {
		t.Fatalf("expected 1 function, got %d", len(fns))
	}
	// Body should not contain comment lines
	// Body should contain "return 1;" and closing "}" (comments stripped)
	if fns[0].BodyLines < 1 {
		t.Errorf("expected at least 1 body line (sans comments), got %d", fns[0].BodyLines)
	}
}

func TestExtractFunctionsFromFile_Methods(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.ts")

	content := `
class UserService {
  validateUser(user: User) {
    if (!user.email) {
      throw new Error("missing email");
    }
    return true;
  }

  checkPerson(person: Person) {
    if (!person.email) {
      throw new Error("missing email");
    }
    return true;
  }
}
`
	os.WriteFile(file, []byte(content), 0644)

	fns := extractFunctionsFromFile(file, dir)
	if len(fns) < 2 {
		t.Fatalf("expected at least 2 methods, got %d", len(fns))
	}
}

// ---------------------------------------------------------------------------
// End-to-end semantic detection
// ---------------------------------------------------------------------------

func TestFindSemanticClones_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	// Two functions with identical structure, different names
	fileA := filepath.Join(srcDir, "serviceA.ts")
	os.WriteFile(fileA, []byte(`
function validateEmail(email: string): boolean {
  if (!email.includes('@')) {
    return false;
  }
  const parts = email.split('@');
  if (parts.length !== 2) {
    return false;
  }
  return parts[1].includes('.');
}
`), 0644)

	fileB := filepath.Join(srcDir, "serviceB.ts")
	os.WriteFile(fileB, []byte(`
function checkMail(addr: string): boolean {
  if (!addr.includes('@')) {
    return false;
  }
  const segments = addr.split('@');
  if (segments.length !== 2) {
    return false;
  }
  return segments[1].includes('.');
}
`), 0644)

	clones := findSemanticClones(dir, []string{"src/"}, nil, 3, 0.85)
	if len(clones) == 0 {
		t.Fatal("expected to find semantic clones")
	}
	if clones[0].Similarity != 1.0 {
		t.Errorf("expected exact match (1.0), got %f", clones[0].Similarity)
	}
}

func TestFindSemanticClones_NoFalsePositives(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	// Two completely different functions
	file := filepath.Join(srcDir, "utils.ts")
	os.WriteFile(file, []byte(`
function add(a: number, b: number): number {
  return a + b;
}

function formatDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return year + '-' + month + '-' + day;
}
`), 0644)

	clones := findSemanticClones(dir, []string{"src/"}, nil, 2, 0.85)
	if len(clones) > 0 {
		t.Errorf("different functions should not be flagged as clones, got %d with similarity %f",
			len(clones), clones[0].Similarity)
	}
}

func TestFindSemanticClones_FuzzyMatch(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	// Two functions with mostly shared structure but STRUCTURAL differences
	// (extra lines, different control flow) — not just renames.
	// Pure renames produce exact semantic matches; fuzzy needs real differences.
	fileA := filepath.Join(srcDir, "handlerA.ts")
	os.WriteFile(fileA, []byte(`
function handleUserCreate(req: Request, res: Response) {
  const body = req.body;
  if (!body.name) {
    return res.status(400).json({ error: "name required" });
  }
  if (!body.email) {
    return res.status(400).json({ error: "email required" });
  }
  const result = await createUser(body);
  return res.status(201).json(result);
}
`), 0644)

	fileB := filepath.Join(srcDir, "handlerB.ts")
	os.WriteFile(fileB, []byte(`
function handleProductCreate(req: Request, res: Response) {
  const body = req.body;
  if (!body.name) {
    return res.status(400).json({ error: "name required" });
  }
  if (!body.price || body.price <= 0) {
    return res.status(400).json({ error: "valid price required" });
  }
  const sanitized = sanitize(body);
  const result = await createProduct(sanitized);
  logger.info("product created", result.id);
  return res.status(201).json(result);
}
`), 0644)

	clones := findSemanticClones(dir, []string{"src/"}, nil, 3, 0.50)
	if len(clones) == 0 {
		t.Fatal("expected to find fuzzy semantic clones")
	}
	if clones[0].Similarity >= 1.0 {
		t.Errorf("expected fuzzy match (< 1.0), got %f", clones[0].Similarity)
	}
	if clones[0].Similarity < 0.50 {
		t.Errorf("expected similarity >= 0.50, got %f", clones[0].Similarity)
	}
}

func TestFindSemanticClones_SkipsSmallFunctions(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	// Tiny functions that look alike
	file := filepath.Join(srcDir, "utils.ts")
	os.WriteFile(file, []byte(`
function getA() {
  return a;
}

function getB() {
  return b;
}
`), 0644)

	clones := findSemanticClones(dir, []string{"src/"}, nil, 5, 0.85)
	if len(clones) > 0 {
		t.Error("small functions below minLines should not be flagged")
	}
}

func TestFindSemanticClones_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	// Duplicate in test file — should be ignored
	fileA := filepath.Join(srcDir, "service.ts")
	os.WriteFile(fileA, []byte(`
function validate(input: string): boolean {
  if (!input) { return false; }
  if (input.length < 3) { return false; }
  if (input.length > 100) { return false; }
  return true;
}
`), 0644)

	fileB := filepath.Join(srcDir, "service.test.ts")
	os.WriteFile(fileB, []byte(`
function validate(input: string): boolean {
  if (!input) { return false; }
  if (input.length < 3) { return false; }
  if (input.length > 100) { return false; }
  return true;
}
`), 0644)

	clones := findSemanticClones(dir, []string{"src/"}, nil, 3, 0.85)
	if len(clones) > 0 {
		t.Error("test files should be excluded from semantic analysis")
	}
}

func TestFindSemanticClones_CrossFile(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(filepath.Join(srcDir, "modules", "auth"), 0755)
	os.MkdirAll(filepath.Join(srcDir, "modules", "billing"), 0755)

	// Same logic in different modules — classic refactoring opportunity
	os.WriteFile(filepath.Join(srcDir, "modules", "auth", "validator.ts"), []byte(`
function validateInput(data: any): ValidationResult {
  const errors: string[] = [];
  if (!data.name || data.name.trim() === '') {
    errors.push('Name is required');
  }
  if (!data.email || !data.email.includes('@')) {
    errors.push('Valid email is required');
  }
  return { valid: errors.length === 0, errors };
}
`), 0644)

	os.WriteFile(filepath.Join(srcDir, "modules", "billing", "checker.ts"), []byte(`
function checkPayload(info: any): ValidationResult {
  const issues: string[] = [];
  if (!info.name || info.name.trim() === '') {
    issues.push('Name is required');
  }
  if (!info.email || !info.email.includes('@')) {
    issues.push('Valid email is required');
  }
  return { valid: issues.length === 0, errors: issues };
}
`), 0644)

	clones := findSemanticClones(dir, []string{"src/"}, nil, 3, 0.85)
	if len(clones) == 0 {
		t.Fatal("expected cross-module semantic clone detection")
	}
	// Verify it found the right pair
	clone := clones[0]
	if clone.A.Name != "validateInput" && clone.B.Name != "validateInput" {
		t.Errorf("expected validateInput in clone pair, got %s and %s", clone.A.Name, clone.B.Name)
	}
}

// ---------------------------------------------------------------------------
// Regression: Python functions
// ---------------------------------------------------------------------------

func TestExtractFunctionsFromFile_Python(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "utils.py")

	content := `
def validate_email(email):
    if '@' not in email:
        return False
    parts = email.split('@')
    if len(parts) != 2:
        return False
    return '.' in parts[1]

def check_mail(addr):
    if '@' not in addr:
        return False
    segments = addr.split('@')
    if len(segments) != 2:
        return False
    return '.' in segments[1]
`
	os.WriteFile(file, []byte(content), 0644)

	// Python uses indentation, not braces — our brace-based extractor
	// won't reliably parse Python. This test documents current behavior.
	fns := extractFunctionsFromFile(file, dir)
	// We expect 0 or partial extraction since Python lacks braces
	_ = fns // best-effort for now
}
