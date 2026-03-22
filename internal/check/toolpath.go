package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// NodeEnv returns a copy of os.Environ() with the node/npx directory
// prepended to PATH. This ensures child processes (like jscpd calling node)
// can find node even when it's not in the Go process PATH.
func NodeEnv() []string {
	npxBin := FindNpx()
	if npxBin == "" {
		return os.Environ()
	}

	nodeDir := filepath.Dir(npxBin)
	env := os.Environ()

	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env[i] = "PATH=" + nodeDir + string(os.PathListSeparator) + e[5:]
			return env
		}
	}

	return append(env, "PATH="+nodeDir)
}

// FindNpx locates the npx binary, checking PATH and common install locations.
// On Windows, Go's exec.LookPath often can't find npx because the Go process
// PATH differs from the shell PATH (common with nvm, fnm, volta).
func FindNpx() string {
	name := "npx"
	if runtime.GOOS == "windows" {
		name = "npx.cmd"
	}

	if p, err := exec.LookPath(name); err == nil {
		return p
	}

	if runtime.GOOS == "windows" {
		nodejsDir := filepath.Join(os.Getenv("ProgramFiles"), "nodejs")
		candidates := []string{
			filepath.Join(nodejsDir, "npx.cmd"),
		}

		if nvmHome := os.Getenv("NVM_HOME"); nvmHome != "" {
			entries, _ := os.ReadDir(nvmHome)
			for _, e := range entries {
				if e.IsDir() {
					candidates = append(candidates, filepath.Join(nvmHome, e.Name(), "npx.cmd"))
				}
			}
		}
		if nvmLink := os.Getenv("NVM_SYMLINK"); nvmLink != "" {
			candidates = append(candidates, filepath.Join(nvmLink, "npx.cmd"))
		}

		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	for _, p := range []string{"/usr/local/bin/npx", "/usr/bin/npx"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		candidates := []string{
			filepath.Join(home, ".nvm", "current", "bin", "npx"),
			filepath.Join(home, ".volta", "bin", "npx"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	return ""
}
