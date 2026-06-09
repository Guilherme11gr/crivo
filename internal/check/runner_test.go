package check

import (
	"testing"

	"github.com/guilherme11gr/crivo/internal/config"
)

func TestIsCheckEnabled_HonorsCLIDisabledChecks(t *testing.T) {
	cfg := config.DefaultConfig()

	if isCheckEnabled("complexity", cfg, map[string]bool{"complexity": true}) {
		t.Fatal("expected complexity to be disabled by CLI override")
	}

	if isCheckEnabled("eslint", cfg, map[string]bool{"complexity": true}) {
		t.Fatal("expected eslint to always be disabled (deprecated)")
	}

	if !isCheckEnabled("coverage", cfg, map[string]bool{"complexity": true}) {
		t.Fatal("expected coverage to remain enabled")
	}
}

func TestDefaultMaxWorkers_LocalIsConservative(t *testing.T) {
	t.Setenv("CI", "")
	workers := defaultMaxWorkers()
	if workers < 1 || workers > 2 {
		t.Fatalf("defaultMaxWorkers() = %d, want 1..2 for local runs", workers)
	}
}

func TestDefaultMaxWorkers_CIIsBounded(t *testing.T) {
	t.Setenv("CI", "true")
	workers := defaultMaxWorkers()
	if workers < 2 || workers > 4 {
		t.Fatalf("defaultMaxWorkers() = %d, want 2..4 for CI runs", workers)
	}
}

func TestIsHeavyProviderID(t *testing.T) {
	if !isHeavyProviderID("semgrep") {
		t.Fatal("expected semgrep to be heavy")
	}
	if !isHeavyProviderID("coverage") {
		t.Fatal("expected coverage to be heavy")
	}
	if isHeavyProviderID("unknown") {
		t.Fatal("expected unknown provider to be non-heavy")
	}
}
