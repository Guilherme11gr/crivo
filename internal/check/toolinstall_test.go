package check

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

// ---------------------------------------------------------------------------
// Gitleaks installer — checksum and timeout
// ---------------------------------------------------------------------------

// gitleaksAssetName returns the release asset name for the current platform,
// mirroring the mapping in installGitleaksFrom.
func gitleaksAssetName(t *testing.T) string {
	t.Helper()
	osName := runtime.GOOS
	arch := runtime.GOARCH
	switch {
	case osName == "linux" && arch == "amd64":
		return fmt.Sprintf("gitleaks_%s_linux_x64.tar.gz", gitleaksVersion)
	case osName == "linux" && arch == "arm64":
		return fmt.Sprintf("gitleaks_%s_linux_arm64.tar.gz", gitleaksVersion)
	case osName == "darwin" && arch == "amd64":
		return fmt.Sprintf("gitleaks_%s_darwin_x64.tar.gz", gitleaksVersion)
	case osName == "darwin" && arch == "arm64":
		return fmt.Sprintf("gitleaks_%s_darwin_arm64.tar.gz", gitleaksVersion)
	case osName == "windows" && arch == "amd64":
		return fmt.Sprintf("gitleaks_%s_windows_x64.zip", gitleaksVersion)
	default:
		t.Skipf("unsupported platform for gitleaks install test: %s/%s", osName, arch)
		return ""
	}
}

// makeGitleaksArchive builds a tar.gz (or zip on Windows) containing a fake
// gitleaks binary, and returns the archive bytes.
func makeGitleaksArchive(t *testing.T) []byte {
	t.Helper()
	binName := "gitleaks"
	if runtime.GOOS == "windows" {
		binName = "gitleaks.exe"
	}
	content := []byte("fake-gitleaks-binary")

	if runtime.GOOS == "windows" {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(binName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: binName, Mode: 0755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// serveGitleaksRelease serves a fake gitleaks release directory with the given
// archive bytes. Returns the server and the archive's sha256 hex digest.
func serveGitleaksRelease(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()
	assetName := gitleaksAssetName(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, assetName) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

// redirectQGToolDir points QGToolDir at a temp dir for the duration of the
// test, so installs never touch the real ~/.qualitygate.
func redirectQGToolDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := qgToolDirOverride
	qgToolDirOverride = dir
	t.Cleanup(func() { qgToolDirOverride = old })
	return dir
}

func TestInstallGitleaks_ChecksumMatches(t *testing.T) {
	dir := redirectQGToolDir(t)
	archive := makeGitleaksArchive(t)
	server := serveGitleaksRelease(t, archive)

	// The fake archive is not the real gitleaks binary, so point the pinned
	// checksum at the fake archive's actual hash — the point of the test is
	// that a download whose hash matches the pin installs successfully.
	key := runtime.GOOS + "_" + runtime.GOARCH
	oldHash, ok := gitleaksChecksums[key]
	if !ok {
		t.Fatalf("no pinned checksum for %s", key)
	}
	gitleaksChecksums[key] = fmt.Sprintf("%x", sha256.Sum256(archive))
	t.Cleanup(func() { gitleaksChecksums[key] = oldHash })

	if err := installGitleaksFrom(server.URL); err != nil {
		t.Fatalf("installGitleaksFrom with matching checksum failed: %v", err)
	}

	binName := "gitleaks"
	if runtime.GOOS == "windows" {
		binName = "gitleaks.exe"
	}
	dest := filepath.Join(dir, binName)
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected gitleaks binary at %s: %v", dest, err)
	}
}

func TestInstallGitleaks_ChecksumMismatch(t *testing.T) {
	dir := redirectQGToolDir(t)
	archive := makeGitleaksArchive(t)
	server := serveGitleaksRelease(t, archive)

	// Corrupt the pinned checksum for this platform so the download fails
	// verification.
	key := runtime.GOOS + "_" + runtime.GOARCH
	oldHash, ok := gitleaksChecksums[key]
	if !ok {
		t.Fatalf("no pinned checksum for %s", key)
	}
	gitleaksChecksums[key] = strings.Repeat("0", 64)
	t.Cleanup(func() { gitleaksChecksums[key] = oldHash })

	err := installGitleaksFrom(server.URL)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected 'checksum mismatch' in error, got: %v", err)
	}

	binName := "gitleaks"
	if runtime.GOOS == "windows" {
		binName = "gitleaks.exe"
	}
	if _, statErr := os.Stat(filepath.Join(dir, binName)); statErr == nil {
		t.Error("no binary should be written when the checksum mismatches")
	}
}

func TestInstallGitleaks_Timeout(t *testing.T) {
	redirectQGToolDir(t)
	assetName := gitleaksAssetName(t)

	// Server that responds slower than the client timeout — the client must
	// give up on its own. The handler returns after a while so the test server
	// can shut down cleanly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, assetName) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	oldClient := gitleaksHTTPClient
	gitleaksHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	t.Cleanup(func() { gitleaksHTTPClient = oldClient })

	start := time.Now()
	err := installGitleaksFrom(server.URL)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "downloading gitleaks") {
		t.Errorf("expected download error, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout not respected: install took %s", elapsed)
	}
}

// ---------------------------------------------------------------------------
// CRIVO_NO_AUTO_INSTALL
// ---------------------------------------------------------------------------

func TestEnsureTool_NoAutoInstallEnv(t *testing.T) {
	// A tool that is not on PATH and not cached — with the env set, EnsureTool
	// must refuse to install and never call the installer.
	toolName := "qg-no-auto-install-tool"
	toolCacheMu.Lock()
	delete(toolCache, toolName)
	toolCacheMu.Unlock()

	installerCalled := false
	oldInstallers := toolInstallers
	toolInstallers = map[string]func() error{
		toolName: func() error {
			installerCalled = true
			return nil
		},
	}
	t.Cleanup(func() { toolInstallers = oldInstallers })

	t.Setenv("CRIVO_NO_AUTO_INSTALL", "1")

	_, err := EnsureTool(toolName)
	if err == nil {
		t.Fatal("expected error when CRIVO_NO_AUTO_INSTALL=1")
	}
	if !strings.Contains(err.Error(), "auto-install disabled") {
		t.Errorf("expected 'auto-install disabled' in error, got: %v", err)
	}
	if installerCalled {
		t.Error("installer must not be called when CRIVO_NO_AUTO_INSTALL=1")
	}
}

func TestEnsureTool_AutoInstallAllowedByDefault(t *testing.T) {
	// Without the env var, EnsureTool falls through to the installer.
	toolName := "qg-auto-install-allowed-tool"
	toolCacheMu.Lock()
	delete(toolCache, toolName)
	toolCacheMu.Unlock()

	installerCalled := false
	oldInstallers := toolInstallers
	toolInstallers = map[string]func() error{
		toolName: func() error {
			installerCalled = true
			return fmt.Errorf("installer failed (expected in test)")
		},
	}
	t.Cleanup(func() { toolInstallers = oldInstallers })

	_, err := EnsureTool(toolName)
	if err == nil {
		t.Fatal("expected error from failing installer")
	}
	if !installerCalled {
		t.Error("installer should be called when CRIVO_NO_AUTO_INSTALL is unset")
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
