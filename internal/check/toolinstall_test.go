package check

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestQGToolDir(t *testing.T) {
	dir := QGToolDir()
	if dir == "" {
		t.Fatal("QGToolDir returned empty string")
	}
	// Should end with .qualitygate/bin
	if !contains(dir, ".qualitygate") {
		t.Errorf("expected .qualitygate in path, got %s", dir)
	}
	// Directory should exist
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("QGToolDir should create directory: %v", err)
	}
}

func TestFindTool_InPath(t *testing.T) {
	// "go" should be in PATH since we're running Go tests
	p := FindTool("go")
	if p == "" {
		t.Skip("go not in PATH")
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("FindTool returned non-existent path: %s", p)
	}
}

func TestFindTool_NotFound(t *testing.T) {
	p := FindTool("nonexistent-tool-xyz-12345")
	if p != "" {
		t.Errorf("expected empty string for nonexistent tool, got %s", p)
	}
}

func TestFindTool_CachesResult(t *testing.T) {
	// Clear cache
	toolCacheMu.Lock()
	delete(toolCache, "go")
	toolCacheMu.Unlock()

	p1 := FindTool("go")
	p2 := FindTool("go")
	if p1 != p2 {
		t.Errorf("cached result should be same: %s vs %s", p1, p2)
	}
}

func TestFindTool_LocalBin(t *testing.T) {
	// Create a fake binary in the tool dir
	dir := QGToolDir()
	fakeName := "qg-test-fake-tool"
	if runtime.GOOS == "windows" {
		fakeName += ".exe"
	}
	fakePath := filepath.Join(dir, fakeName)
	os.WriteFile(fakePath, []byte("fake"), 0755)
	defer os.Remove(fakePath)

	// Clear cache
	toolCacheMu.Lock()
	delete(toolCache, "qg-test-fake-tool")
	toolCacheMu.Unlock()

	p := FindTool("qg-test-fake-tool")
	if p == "" {
		t.Error("should find tool in QG tool dir")
	}
	if p != fakePath {
		t.Errorf("expected %s, got %s", fakePath, p)
	}
}

func TestEnsureTool_NoInstaller(t *testing.T) {
	_, err := EnsureTool("nonexistent-tool-xyz-12345")
	if err == nil {
		t.Error("expected error for tool with no installer")
	}
	if !contains(err.Error(), "no auto-installer") {
		t.Errorf("expected 'no auto-installer' in error, got: %s", err)
	}
}

func TestFindPython(t *testing.T) {
	p := findPython()
	// Python may or may not be available — just verify it doesn't crash
	if p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("findPython returned non-existent path: %s", p)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
