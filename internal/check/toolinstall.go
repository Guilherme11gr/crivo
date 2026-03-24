package check

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Auto-install for external tools (semgrep, gitleaks, etc.)
// Tools are installed into ~/.qualitygate/bin/ and cached across runs.
// ---------------------------------------------------------------------------

var (
	toolCacheMu sync.Mutex
	toolCache   = map[string]string{} // tool name → resolved binary path
)

// QGToolDir returns the directory where quality-gate installs tools.
// Creates it if it doesn't exist.
func QGToolDir() string {
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
	"semgrep": installSemgrep,
}

// ---------------------------------------------------------------------------
// Semgrep installer
// ---------------------------------------------------------------------------

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

	cmd := exec.Command(pip, "install", "--upgrade", "semgrep")
	cmd.Env = append(os.Environ(), "VIRTUAL_ENV="+venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pip install semgrep: %s (%w)", string(out), err)
	}

	return nil
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
