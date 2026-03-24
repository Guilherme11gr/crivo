package duplication

// Semantic code duplication detection.
// Finds functions that are structurally identical despite different variable names
// (Type-2 clones) and structurally similar functions (Type-3 clones).
//
// Algorithm:
//  1. Walk source files, extract functions with their bodies
//  2. Normalize each body: identifiers → positional tokens, literals → placeholders
//  3. Hash normalized form → exact semantic matches (Type-2)
//  4. Jaccard trigram similarity → near-matches (Type-3)

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// FunctionInfo holds an extracted function with its normalized representation.
type FunctionInfo struct {
	Name       string
	File       string
	Line       int
	EndLine    int
	Body       string   // raw function body (without declaration line)
	BodyLines  int      // significant lines (non-empty, non-comment)
	Normalized string   // normalized token sequence
	Hash       string   // SHA-256 of normalized form
	Tokens     []string // normalized token list (for similarity)
}

// SemanticClone represents a detected semantic duplicate pair.
type SemanticClone struct {
	A          *FunctionInfo
	B          *FunctionInfo
	Similarity float64 // 1.0 = exact semantic match, <1.0 = fuzzy
}

// ---------------------------------------------------------------------------
// Keywords — preserved during normalization
// ---------------------------------------------------------------------------

var semanticKeywords = map[string]bool{
	// control flow
	"if": true, "else": true, "for": true, "while": true, "do": true,
	"switch": true, "case": true, "break": true, "continue": true,
	"return": true, "throw": true, "try": true, "catch": true, "finally": true,
	// declarations
	"function": true, "const": true, "let": true, "var": true,
	"class": true, "interface": true, "type": true, "enum": true,
	"new": true, "delete": true, "typeof": true, "instanceof": true, "void": true,
	// async
	"async": true, "await": true, "yield": true,
	// modules
	"import": true, "export": true, "default": true, "from": true,
	// values
	"true": true, "false": true, "null": true, "undefined": true,
	"this": true, "super": true, "in": true, "of": true,
	// Go
	"func": true, "range": true, "defer": true, "go": true, "select": true,
	"chan": true, "map": true, "struct": true, "package": true, "nil": true,
	// Python
	"def": true, "self": true, "None": true, "elif": true, "except": true,
	"pass": true, "raise": true, "with": true, "as": true, "lambda": true,
	// types (common across languages)
	"string": true, "number": true, "boolean": true, "int": true,
	"float": true, "float64": true, "float32": true, "int64": true, "int32": true,
	"double": true, "bool": true, "byte": true, "error": true, "any": true,
}

// ---------------------------------------------------------------------------
// Regex patterns
// ---------------------------------------------------------------------------

var (
	semFuncDeclRe = regexp.MustCompile(
		`(?:` +
			`function\s+(\w+)` + // function foo()
			`|(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[a-zA-Z_]\w*)\s*=>` + // const foo = () =>
			`|(\w+)\s*\([^)]*\)\s*\{` + // foo() { (method)
			`|func\s+(?:\([^)]*\)\s*)?(\w+)` + // func foo() / func (r T) foo()
			`|def\s+(\w+)` + // def foo (Python)
			`)`)
	semCommentLineRe = regexp.MustCompile(`^\s*(?://|#|\*|/\*)`)
	semStringLitRe   = regexp.MustCompile("(?:\"[^\"]*\"|'[^']*'|`[^`]*`)")
	semNumberLitRe   = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	semIdentRe       = regexp.MustCompile(`\b[a-zA-Z_]\w*\b`)
)

// falseMatchKeywords are identifiers that the function regex might capture
// but are actually control-flow keywords.
var falseMatchKeywords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true,
	"catch": true, "return": true, "else": true,
}

// ---------------------------------------------------------------------------
// Function extraction
// ---------------------------------------------------------------------------

// extractFunctionsFromFile parses a single file and extracts functions with bodies.
func extractFunctionsFromFile(path string, projectDir string) []FunctionInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(projectDir, path)
	relPath = filepath.ToSlash(relPath)

	var functions []FunctionInfo
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	lineNum := 0
	currentFunc := ""
	currentFuncLine := 0
	var bodyLines []string
	braceCount := 0
	inFunction := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Try to detect function start when not already inside one
		if !inFunction {
			matches := semFuncDeclRe.FindStringSubmatch(line)
			if matches != nil {
				name := ""
				for i := 1; i < len(matches); i++ {
					if matches[i] != "" {
						name = matches[i]
						break
					}
				}
				if name == "" || falseMatchKeywords[name] {
					continue
				}

				currentFunc = name
				currentFuncLine = lineNum
				bodyLines = nil
				braceCount = strings.Count(line, "{") - strings.Count(line, "}")
				inFunction = true

				// Arrow functions without opening brace on same line → skip
				if braceCount <= 0 && !strings.Contains(line, "{") {
					inFunction = false
					continue
				}
				continue
			}
			continue
		}

		// Inside a function — collect body lines
		if trimmed != "" && !semCommentLineRe.MatchString(trimmed) {
			bodyLines = append(bodyLines, trimmed)
		}

		braceCount += strings.Count(line, "{") - strings.Count(line, "}")

		if braceCount <= 0 && lineNum > currentFuncLine {
			// Function ended
			if len(bodyLines) > 0 {
				functions = append(functions, FunctionInfo{
					Name:      currentFunc,
					File:      relPath,
					Line:      currentFuncLine,
					EndLine:   lineNum,
					Body:      strings.Join(bodyLines, "\n"),
					BodyLines: len(bodyLines),
				})
			}
			inFunction = false
			currentFunc = ""
		}
	}

	// Handle unclosed function at EOF
	if inFunction && currentFunc != "" && len(bodyLines) > 0 {
		functions = append(functions, FunctionInfo{
			Name:      currentFunc,
			File:      relPath,
			Line:      currentFuncLine,
			EndLine:   lineNum,
			Body:      strings.Join(bodyLines, "\n"),
			BodyLines: len(bodyLines),
		})
	}

	return functions
}

// ---------------------------------------------------------------------------
// Normalization
// ---------------------------------------------------------------------------

// normalizeBody takes a function body and produces a canonical representation
// where identifiers are replaced with positional tokens ($0, $1, ...),
// string literals with _S_, and number literals with _N_.
func normalizeBody(body string) (string, []string) {
	// Replace string literals before tokenizing (they may contain keywords)
	normalized := semStringLitRe.ReplaceAllString(body, "_S_")
	// Replace number literals
	normalized = semNumberLitRe.ReplaceAllString(normalized, "_N_")

	words := tokenize(normalized)

	identMap := map[string]string{}
	nextID := 0
	tokens := make([]string, 0, len(words))

	for _, w := range words {
		switch {
		case w == "_S_" || w == "_N_":
			tokens = append(tokens, w)
		case semanticKeywords[w]:
			tokens = append(tokens, w)
		case semIdentRe.MatchString(w):
			if _, ok := identMap[w]; !ok {
				identMap[w] = fmt.Sprintf("$%d", nextID)
				nextID++
			}
			tokens = append(tokens, identMap[w])
		default:
			tokens = append(tokens, w)
		}
	}

	return strings.Join(tokens, " "), tokens
}

// tokenize splits code into meaningful tokens (identifiers, operators, punctuation).
func tokenize(code string) []string {
	runes := []rune(code)
	n := len(runes)
	tokens := make([]string, 0, n/2)
	i := 0

	for i < n {
		ch := runes[i]

		// Skip whitespace
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Identifier / keyword / placeholder
		if unicode.IsLetter(ch) || ch == '_' || ch == '$' {
			start := i
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '$') {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			continue
		}

		// Number
		if unicode.IsDigit(ch) {
			start := i
			for i < n && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			continue
		}

		// Single-character token
		tokens = append(tokens, string(ch))
		i++
	}

	return tokens
}

// hashNormalized creates a SHA-256 hash of the normalized form.
func hashNormalized(normalized string) string {
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Similarity
// ---------------------------------------------------------------------------

// jaccardTrigrams computes Jaccard similarity between two token sequences
// using trigrams (3-grams). Returns 0.0–1.0.
func jaccardTrigrams(a, b []string) float64 {
	if len(a) < 3 || len(b) < 3 {
		return 0
	}

	setA := make(map[string]struct{}, len(a))
	for i := 0; i <= len(a)-3; i++ {
		setA[a[i]+" "+a[i+1]+" "+a[i+2]] = struct{}{}
	}

	setB := make(map[string]struct{}, len(b))
	for i := 0; i <= len(b)-3; i++ {
		setB[b[i]+" "+b[i+1]+" "+b[i+2]] = struct{}{}
	}

	intersection := 0
	for k := range setA {
		if _, ok := setB[k]; ok {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// ---------------------------------------------------------------------------
// Main detection logic
// ---------------------------------------------------------------------------

var excludedDirs = map[string]bool{
	"node_modules": true, ".next": true, "dist": true, "build": true,
	"coverage": true, "__pycache__": true, ".git": true, "vendor": true,
}

var sourceExtensions = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".go": true, ".py": true,
}

// findSemanticClones walks source files, extracts functions, normalizes them,
// and finds semantic duplicates using exact hash matching and fuzzy trigram similarity.
func findSemanticClones(projectDir string, srcDirs []string, exclude []string, minLines int, similarityThreshold float64) []SemanticClone {
	var allFunctions []FunctionInfo

	for _, srcDir := range srcDirs {
		dir := filepath.Join(projectDir, srcDir)
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if excludedDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !sourceExtensions[filepath.Ext(path)] {
				return nil
			}
			// Skip test files
			base := filepath.Base(path)
			if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
				strings.Contains(base, "_test.") || strings.HasSuffix(base, "_test.go") {
				return nil
			}

			fns := extractFunctionsFromFile(path, projectDir)
			allFunctions = append(allFunctions, fns...)
			return nil
		})
	}

	// Normalize and hash each function
	for i := range allFunctions {
		fn := &allFunctions[i]
		fn.Normalized, fn.Tokens = normalizeBody(fn.Body)
		fn.Hash = hashNormalized(fn.Normalized)
	}

	// Filter by minimum body size
	candidates := make([]*FunctionInfo, 0, len(allFunctions))
	for i := range allFunctions {
		if allFunctions[i].BodyLines >= minLines {
			candidates = append(candidates, &allFunctions[i])
		}
	}

	var clones []SemanticClone
	seen := map[string]bool{}

	pairKey := func(a, b *FunctionInfo) string {
		if a.File+fmt.Sprint(a.Line) > b.File+fmt.Sprint(b.Line) {
			a, b = b, a
		}
		return a.File + ":" + fmt.Sprint(a.Line) + "|" + b.File + ":" + fmt.Sprint(b.Line)
	}

	// Phase 1: Exact semantic matches (same normalized hash)
	hashGroups := map[string][]int{}
	for i, fn := range candidates {
		hashGroups[fn.Hash] = append(hashGroups[fn.Hash], i)
	}

	for _, indices := range hashGroups {
		if len(indices) < 2 {
			continue
		}
		for i := 0; i < len(indices); i++ {
			for j := i + 1; j < len(indices); j++ {
				a := candidates[indices[i]]
				b := candidates[indices[j]]
				key := pairKey(a, b)
				if seen[key] {
					continue
				}
				seen[key] = true
				clones = append(clones, SemanticClone{A: a, B: b, Similarity: 1.0})
			}
		}
	}

	// Phase 2: Fuzzy matches via trigram Jaccard similarity
	if similarityThreshold < 1.0 && len(candidates) <= 2000 {
		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				a := candidates[i]
				b := candidates[j]

				if a.Hash == b.Hash {
					continue
				}
				key := pairKey(a, b)
				if seen[key] {
					continue
				}

				// Quick size filter — bodies too different in size won't match
				minSize := a.BodyLines
				maxSize := b.BodyLines
				if minSize > maxSize {
					minSize, maxSize = maxSize, minSize
				}
				if float64(minSize)/float64(maxSize) < 0.5 {
					continue
				}

				sim := jaccardTrigrams(a.Tokens, b.Tokens)
				if sim >= similarityThreshold {
					seen[key] = true
					clones = append(clones, SemanticClone{A: a, B: b, Similarity: sim})
				}
			}
		}
	}

	// Sort: exact matches first, then by similarity descending
	sort.Slice(clones, func(i, j int) bool {
		return clones[i].Similarity > clones[j].Similarity
	})

	return clones
}
