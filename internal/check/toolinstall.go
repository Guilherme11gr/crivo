package check

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Auto-install for external tools (semgrep, gitleaks, etc.)
// Tools are installed into ~/.qualitygate/bin/ and cached across runs.
// ---------------------------------------------------------------------------

var (
	toolCacheMu sync.Mutex
	toolCache   = map[string]string{} // tool name → resolved binary path

	// qgToolDirOverride redirects QGToolDir in tests so installs never touch
	// the real ~/.qualitygate. Empty in production.
	qgToolDirOverride string
)

// QGToolDir returns the directory where quality-gate installs tools.
// Creates it if it doesn't exist.
func QGToolDir() string {
	if qgToolDirOverride != "" {
		return qgToolDirOverride
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".qualitygate", "bin")
	os.MkdirAll(dir, 0755)
	return dir
}

// FindTool looks for a tool binary:
//  1. In PATH (already installed by user)
//  2. In ~/.qualitygate/bin/ (previously auto-installed)
//  3. Auto-installs if possible (pip for semgrep, go install for others)
//
// Returns the full path to the binary, or "" if not available.
func FindTool(name string) string {
	toolCacheMu.Lock()
	defer toolCacheMu.Unlock()

	if cached, ok := toolCache[name]; ok {
		return cached
	}

	// 1. Check PATH
	if p, err := exec.LookPath(name); err == nil {
		toolCache[name] = p
		return p
	}

	// On Windows, also check with .exe
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath(name + ".exe"); err == nil {
			toolCache[name] = p
			return p
		}
	}

	// 2. Check QG tool dir
	toolDir := QGToolDir()
	binName := name
	if runtime.GOOS == "windows" {
		binName = name + ".exe"
	}
	localBin := filepath.Join(toolDir, binName)
	if _, err := os.Stat(localBin); err == nil {
		toolCache[name] = localBin
		return localBin
	}

	// 3. Check pip/pipx venv for Python tools
	venvBin := findInVenv(name)
	if venvBin != "" {
		toolCache[name] = venvBin
		return venvBin
	}

	// Not found
	return ""
}

// EnsureTool is like FindTool but also attempts auto-installation.
// Returns (path, nil) on success, ("", error) if install fails.
func EnsureTool(name string) (string, error) {
	// First try without installing
	if p := FindTool(name); p != "" {
		return p, nil
	}

	// CRIVO_NO_AUTO_INSTALL=1 opts out of downloading/executing third-party
	// code: report the tool as unavailable instead of installing it.
	if os.Getenv("CRIVO_NO_AUTO_INSTALL") == "1" {
		return "", fmt.Errorf("auto-install disabled (CRIVO_NO_AUTO_INSTALL=1): %s not found", name)
	}

	// Try to auto-install
	installer, ok := toolInstallers[name]
	if !ok {
		return "", fmt.Errorf("%s not found and no auto-installer available", name)
	}

	if err := installer(); err != nil {
		return "", fmt.Errorf("auto-install %s failed: %w", name, err)
	}

	// Invalidate cache and retry
	toolCacheMu.Lock()
	delete(toolCache, name)
	toolCacheMu.Unlock()

	if p := FindTool(name); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("%s installed but binary not found in PATH or tool dir", name)
}

// ---------------------------------------------------------------------------
// Installer registry
// ---------------------------------------------------------------------------

var toolInstallers = map[string]func() error{
	"semgrep":  installSemgrep,
	"gitleaks": installGitleaks,
}

// ---------------------------------------------------------------------------
// Semgrep installer
// ---------------------------------------------------------------------------

// semgrepVersion pins the semgrep release installed by the auto-installer.
// Pinned at v1.173.0 — the latest stable release at the time of writing
// (source: https://api.github.com/repos/semgrep/semgrep/releases/latest,
// checked 2026-08-14). Bumping this pin is a release note for crivo: a new
// semgrep version can change findings, which is intentional.
const semgrepVersion = "1.173.0"

func installSemgrep() error {
	venvDir := qgVenvDir()

	// If venv already exists, just try to install into it
	if _, err := os.Stat(venvDir); err != nil {
		// Create virtualenv
		python := findPython()
		if python == "" {
			return fmt.Errorf("python3 not found — install Python 3.8+ or install semgrep manually: pip install semgrep")
		}

		cmd := exec.Command(python, "-m", "venv", venvDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("creating venv: %s (%w)", string(out), err)
		}
	}

	// Install semgrep into the venv
	pip := venvPip(venvDir)
	if pip == "" {
		return fmt.Errorf("pip not found in venv %s", venvDir)
	}

	cmd := exec.Command(pip, "install", "--upgrade", "semgrep=="+semgrepVersion)
	cmd.Env = append(os.Environ(), "VIRTUAL_ENV="+venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pip install semgrep: %s (%w)", string(out), err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Gitleaks installer — downloads pre-built binary from GitHub Releases
// ---------------------------------------------------------------------------

const gitleaksVersion = "8.24.3"

// gitleaksChecksums pins the sha256 of every gitleaks 8.24.3 release asset we
// can install. Hashes are taken verbatim from the official release checksums:
// https://github.com/gitleaks/gitleaks/releases/download/v8.24.3/gitleaks_8.24.3_checksums.txt
// (fetched 2026-08-14). When bumping gitleaksVersion, update this map from the
// new release's checksums.txt — a mismatch is a hard install failure.
var gitleaksChecksums = map[string]string{
	"linux_amd64":   "9991e0b2903da4c8f6122b5c3186448b927a5da4deef1fe45271c3793f4ee29c",
	"linux_arm64":   "5f2edbe1f49f7b920f9e06e90759947d3c5dfc16f752fb93aaafc17e9d14cf07",
	"darwin_amd64":  "41c44ae8ad1d6eef57d4526ad0fd67d8129eee9a856f55c2b3b9395fd3d9ec0f",
	"darwin_arm64":  "b90f13bb8c90ab72083d9b0c842e39dafb82c0e5c3f872f407366b7a58909013",
	"windows_amd64": "3f1a35578631dbfe633cc5b49e6c906e55ff14a4bfd7336a10fb27fe33b6dcd2",
}

// gitleaksDownloadTimeout bounds the whole gitleaks download (headers + body).
const gitleaksDownloadTimeout = 60 * time.Second

// gitleaksHTTPClient is swappable in tests (e.g. to exercise the timeout).
var gitleaksHTTPClient = &http.Client{Timeout: gitleaksDownloadTimeout}

func installGitleaks() error {
	return installGitleaksFrom("https://github.com/gitleaks/gitleaks/releases/download/v" + gitleaksVersion)
}

// installGitleaksFrom downloads, verifies and extracts the gitleaks binary for
// the current platform. baseURL points at the release directory (the official
// GitHub release in production, an httptest server in tests).
func installGitleaksFrom(baseURL string) error {
	toolDir := QGToolDir()

	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go OS/arch names to gitleaks release naming
	var fileName string
	switch {
	case osName == "linux" && arch == "amd64":
		fileName = fmt.Sprintf("gitleaks_%s_linux_x64.tar.gz", gitleaksVersion)
	case osName == "linux" && arch == "arm64":
		fileName = fmt.Sprintf("gitleaks_%s_linux_arm64.tar.gz", gitleaksVersion)
	case osName == "darwin" && arch == "amd64":
		fileName = fmt.Sprintf("gitleaks_%s_darwin_x64.tar.gz", gitleaksVersion)
	case osName == "darwin" && arch == "arm64":
		fileName = fmt.Sprintf("gitleaks_%s_darwin_arm64.tar.gz", gitleaksVersion)
	case osName == "windows" && arch == "amd64":
		fileName = fmt.Sprintf("gitleaks_%s_windows_x64.zip", gitleaksVersion)
	default:
		return fmt.Errorf("unsupported platform: %s/%s — install gitleaks manually", osName, arch)
	}

	expectedHash, ok := gitleaksChecksums[osName+"_"+arch]
	if !ok {
		return fmt.Errorf("no pinned checksum for gitleaks %s on %s/%s — install gitleaks manually", gitleaksVersion, osName, arch)
	}

	url := fmt.Sprintf("%s/%s", baseURL, fileName)

	resp, err := gitleaksHTTPClient.Get(url)
	if err != nil {
		return fmt.Errorf("downloading gitleaks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("downloading gitleaks: HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading gitleaks download: %w", err)
	}

	// Verify integrity before writing anything to disk.
	actualHash := fmt.Sprintf("%x", sha256.Sum256(body))
	if actualHash != expectedHash {
		return fmt.Errorf("gitleaks checksum mismatch for %s: expected %s, got %s", fileName, expectedHash, actualHash)
	}

	binName := "gitleaks"
	if osName == "windows" {
		binName = "gitleaks.exe"
	}

	destPath := filepath.Join(toolDir, binName)

	if strings.HasSuffix(fileName, ".zip") {
		if err := extractZipBinary(body, binName, destPath); err != nil {
			return fmt.Errorf("extracting gitleaks zip: %w", err)
		}
	} else {
		if err := extractTarGzBinary(body, binName, destPath); err != nil {
			return fmt.Errorf("extracting gitleaks tar.gz: %w", err)
		}
	}

	os.Chmod(destPath, 0755)
	return nil
}

func extractTarGzBinary(data []byte, binName, destPath string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) == binName {
			f, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(f, tr)
			return err
		}
	}
	return fmt.Errorf("%s not found in archive", binName)
}

func extractZipBinary(data []byte, binName, destPath string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if filepath.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("%s not found in archive", binName)
}

// ---------------------------------------------------------------------------
// Python / venv helpers
// ---------------------------------------------------------------------------

func qgVenvDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".qualitygate", "venv")
}

func findPython() string {
	// Try common names
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			// Verify it's Python 3
			out, err := exec.Command(p, "--version").Output()
			if err == nil && strings.Contains(string(out), "Python 3") {
				return p
			}
		}
	}

	// Windows-specific paths
	if runtime.GOOS == "windows" {
		candidates := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python", "Python3*", "python.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Python3*", "python.exe"),
		}
		for _, pattern := range candidates {
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				return matches[len(matches)-1] // newest version
			}
		}
	}

	return ""
}

func findInVenv(name string) string {
	venvDir := qgVenvDir()

	var binDir string
	if runtime.GOOS == "windows" {
		binDir = filepath.Join(venvDir, "Scripts")
	} else {
		binDir = filepath.Join(venvDir, "bin")
	}

	binName := name
	if runtime.GOOS == "windows" {
		binName = name + ".exe"
	}

	bin := filepath.Join(binDir, binName)
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	return ""
}

func venvPip(venvDir string) string {
	var binDir string
	if runtime.GOOS == "windows" {
		binDir = filepath.Join(venvDir, "Scripts")
	} else {
		binDir = filepath.Join(venvDir, "bin")
	}

	for _, name := range []string{"pip3", "pip"} {
		bin := name
		if runtime.GOOS == "windows" {
			bin = name + ".exe"
		}
		p := filepath.Join(binDir, bin)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
