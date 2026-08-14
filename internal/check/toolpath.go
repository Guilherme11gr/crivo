package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// defaultNodeHeapMB is the --max-old-space-size injected into NODE_OPTIONS for
// the node subprocesses crivo spawns (tsc, jest/vitest, jscpd, knip). The Node
// default heap (~2GB on 64-bit) is too small for typechecking or test-running
// large projects — on CI runners the subprocess OOMs and, since crivo 3.5,
// that crash correctly fails the gate instead of passing green. The value is a
// ceiling, not a preallocation: raising it costs nothing on small projects.
const defaultNodeHeapMB = "4096"

// withNodeHeap ensures NODE_OPTIONS carries a --max-old-space-size, appending
// the default when the user hasn't set one (an explicit value is never
// overridden). Set CRIVO_NODE_HEAP_MB to change the default or to "0" to
// disable injection entirely.
func withNodeHeap(env []string) []string {
	heap := os.Getenv("CRIVO_NODE_HEAP_MB")
	if heap == "" {
		heap = defaultNodeHeapMB
	}
	if heap == "0" || strings.EqualFold(heap, "off") {
		return env
	}

	for i, e := range env {
		if !strings.HasPrefix(strings.ToUpper(e), "NODE_OPTIONS=") {
			continue
		}
		if strings.Contains(e, "--max-old-space-size") {
			return env // user manages the heap themselves
		}
		existing := strings.TrimSpace(e[len("NODE_OPTIONS="):])
		if existing == "" {
			env[i] = "NODE_OPTIONS=--max-old-space-size=" + heap
		} else {
			env[i] = "NODE_OPTIONS=" + existing + " --max-old-space-size=" + heap
		}
		return env
	}

	return append(env, "NODE_OPTIONS=--max-old-space-size="+heap)
}

// NodeEnv returns a copy of os.Environ() with the node/npx directory
// prepended to PATH. This ensures child processes (like jscpd calling node)
// can find node even when it's not in the Go process PATH. It also ensures a
// roomy Node heap via NODE_OPTIONS — see withNodeHeap.
func NodeEnv() []string {
	npxBin := FindNpx()
	if npxBin == "" {
		return withNodeHeap(os.Environ())
	}

	nodeDir := filepath.Dir(npxBin)
	env := os.Environ()

	pathFound := false
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env[i] = "PATH=" + nodeDir + string(os.PathListSeparator) + e[5:]
			pathFound = true
			break
		}
	}
	if !pathFound {
		env = append(env, "PATH="+nodeDir)
	}

	return withNodeHeap(env)
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
