package check

import (
	"strings"
	"testing"
)

func nodeOption(t *testing.T, env []string) string {
	t.Helper()
	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "NODE_OPTIONS=") {
			return e[len("NODE_OPTIONS="):]
		}
	}
	return ""
}

func TestWithNodeHeap_InjectsDefaultWhenAbsent(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "")
	t.Setenv("CRIVO_NODE_HEAP_MB", "")
	env := withNodeHeap([]string{"PATH=/usr/bin", "FOO=bar"})
	if got := nodeOption(t, env); got != "--max-old-space-size=4096" {
		t.Errorf("NODE_OPTIONS = %q, want default heap injection", got)
	}
}

func TestWithNodeHeap_AppendsToExistingOptions(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--enable-source-maps")
	env := withNodeHeap([]string{"NODE_OPTIONS=--enable-source-maps"})
	if got := nodeOption(t, env); got != "--enable-source-maps --max-old-space-size=4096" {
		t.Errorf("NODE_OPTIONS = %q, want heap appended preserving existing flags", got)
	}
}

func TestWithNodeHeap_NeverOverridesExplicitHeap(t *testing.T) {
	env := withNodeHeap([]string{"NODE_OPTIONS=--max-old-space-size=8192"})
	if got := nodeOption(t, env); got != "--max-old-space-size=8192" {
		t.Errorf("NODE_OPTIONS = %q, want user heap untouched", got)
	}
}

func TestWithNodeHeap_OverrideViaEnv(t *testing.T) {
	t.Setenv("CRIVO_NODE_HEAP_MB", "8192")
	env := withNodeHeap([]string{"PATH=/usr/bin"})
	if got := nodeOption(t, env); got != "--max-old-space-size=8192" {
		t.Errorf("NODE_OPTIONS = %q, want CRIVO_NODE_HEAP_MB honored", got)
	}
}

func TestWithNodeHeap_Disabled(t *testing.T) {
	t.Setenv("CRIVO_NODE_HEAP_MB", "0")
	env := withNodeHeap([]string{"PATH=/usr/bin"})
	if got := nodeOption(t, env); got != "" {
		t.Errorf("NODE_OPTIONS = %q, want no injection when disabled", got)
	}
}

func TestNodeEnv_IncludesHeap(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "")
	t.Setenv("CRIVO_NODE_HEAP_MB", "")
	env := NodeEnv()
	if got := nodeOption(t, env); got != "--max-old-space-size=4096" {
		t.Errorf("NodeEnv NODE_OPTIONS = %q, want heap present", got)
	}
}
