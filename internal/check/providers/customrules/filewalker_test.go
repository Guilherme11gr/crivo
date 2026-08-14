package customrules

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// writeFileAt writes a file, creating parent directories as needed.
func writeFileAt(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// ─── WalkFiles ───────────────────────────────────────────────────────────────

func TestWalkFiles_MatchesGlob(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, dir, "src/app.ts", "x")
	writeFileAt(t, dir, "src/lib/util.ts", "x")
	writeFileAt(t, dir, "src/lib/util.tsx", "x")
	writeFileAt(t, dir, "src/readme.md", "x")
	writeFileAt(t, dir, "other.ts", "x")

	files, err := WalkFiles(context.Background(), dir, "src/**/*.{ts,tsx}", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(files)
	want := []string{"src/app.ts", "src/lib/util.ts", "src/lib/util.tsx"}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
}

func TestWalkFiles_ExcludesHardcodedDirs(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, dir, "src/app.ts", "x")
	writeFileAt(t, dir, "node_modules/pkg/index.ts", "x")
	writeFileAt(t, dir, "dist/bundle.ts", "x")
	writeFileAt(t, dir, ".next/out.ts", "x")
	writeFileAt(t, dir, "coverage/lcov.ts", "x")

	files, err := WalkFiles(context.Background(), dir, "**/*.ts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(files, []string{"src/app.ts"}) {
		t.Errorf("files = %v, want only src/app.ts", files)
	}
}

func TestWalkFiles_ExcludeDirAndFileGlob(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, dir, "a.min.js", "x")
	writeFileAt(t, dir, "src/generated/x.ts", "x")
	writeFileAt(t, dir, "pkg/generated/y.ts", "x")
	writeFileAt(t, dir, "src/keep.ts", "x")

	exclude := []string{"*.min.js", "src/generated/"}
	files, err := WalkFiles(context.Background(), dir, "**/*.{ts,js}", exclude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(files)
	// a.min.js excluded by the file glob; src/generated/ excluded by the
	// relative dir path; pkg/generated is NOT excluded (the old basename-only
	// matching would have wrongly dropped it).
	want := []string{"pkg/generated/y.ts", "src/keep.ts"}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
}

func TestWalkFiles_SkipsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, maxFileSize+1)
	if err := os.WriteFile(filepath.Join(dir, "big.ts"), big, 0644); err != nil {
		t.Fatal(err)
	}
	writeFileAt(t, dir, "small.ts", "x")

	files, err := WalkFiles(context.Background(), dir, "*.ts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(files, []string{"small.ts"}) {
		t.Errorf("files = %v, want only small.ts", files)
	}
}

func TestWalkFiles_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, dir, "real.ts", "x")
	link := filepath.Join(dir, "link.ts")
	if err := os.Symlink(filepath.Join(dir, "real.ts"), link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	files, err := WalkFiles(context.Background(), dir, "*.ts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(files, []string{"real.ts"}) {
		t.Errorf("files = %v, want only real.ts (symlink must be skipped)", files)
	}
}

func TestWalkFiles_DepthGuard(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a")
	for i := 0; i < maxDepth+2; i++ {
		deep = filepath.Join(deep, "d")
	}
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	writeFileAt(t, deep, "leaf.ts", "x")
	writeFileAt(t, dir, "top.ts", "x")

	files, err := WalkFiles(context.Background(), dir, "**/*.ts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(files, []string{"top.ts"}) {
		t.Errorf("files = %v, want only top.ts (depth guard must prune)", files)
	}
}

func TestWalkFiles_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, dir, "src/app.ts", "x")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := WalkFiles(ctx, dir, "**/*.ts", nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// ─── matchGlob ───────────────────────────────────────────────────────────────

func TestMatchGlob_AlternationAndNoCharClass(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// {a,b} alternation
		{"src/**/*.{ts,tsx}", "src/app/page.ts", true},
		{"src/**/*.{ts,tsx}", "src/app/page.tsx", true},
		{"src/**/*.{ts,tsx}", "src/app/page.js", false},
		// ? matches a single non-separator character
		{"file?.ts", "file1.ts", true},
		{"file?.ts", "file12.ts", false},
		// [...] character classes are NOT supported (documented limitation)
		{"**/*.[jt]s", "src/app.ts", false},
		{"**/*.[jt]s", "src/app.js", false},
		// * does not cross separators
		{"*.ts", "src/file.ts", false},
		{"*.ts", "file.ts", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// ─── IsTextFile ──────────────────────────────────────────────────────────────

func TestIsTextFile(t *testing.T) {
	if !IsTextFile([]byte("const x = 1\n")) {
		t.Error("plain text should be accepted")
	}
	if IsTextFile([]byte("const x = 1\x00\x00\x00")) {
		t.Error("null bytes should be rejected as binary")
	}
	if IsTextFile([]byte{0xff, 0xfe, 0x00, 0x01}) {
		t.Error("invalid UTF-8 should be rejected as binary")
	}
	if !IsTextFile([]byte("áéíóú — utf8 ok")) {
		t.Error("valid UTF-8 with accents should be accepted")
	}
}
