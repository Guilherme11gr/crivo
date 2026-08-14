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

func TestIsCheckEnabled_ComplexityHonorsConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if !isCheckEnabled("complexity", cfg, nil) {
		t.Fatal("expected complexity to be enabled by default")
	}

	cfg.Checks.Complexity = false
	if isCheckEnabled("complexity", cfg, nil) {
		t.Fatal("expected complexity to be disabled when checks.complexity is false")
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
	if !isHeavyProviderID("typescript") {
		t.Fatal("expected typescript to be heavy")
	}
	if !isHeavyProviderID("complexity") {
		t.Fatal("expected complexity to be heavy")
	}
	if isHeavyProviderID("unknown") {
		t.Fatal("expected unknown provider to be non-heavy")
	}
}

func TestDefaultMaxWorkers_EnvOverride(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("CRIVO_MAX_WORKERS", "8")
	if got := defaultMaxWorkers(); got != 8 {
		t.Fatalf("defaultMaxWorkers() with CRIVO_MAX_WORKERS=8 = %d, want 8", got)
	}
}

func TestDefaultMaxWorkers_EnvInvalidKeepsDefault(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("CRIVO_MAX_WORKERS", "banana")
	got := defaultMaxWorkers()
	if got < 1 || got > 2 {
		t.Fatalf("defaultMaxWorkers() with invalid env = %d, want local default 1..2", got)
	}
}

func TestDefaultMaxWorkers_EnvClamped(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("CRIVO_MAX_WORKERS", "999")
	if got := defaultMaxWorkers(); got != 16 {
		t.Fatalf("defaultMaxWorkers() with CRIVO_MAX_WORKERS=999 = %d, want clamped 16", got)
	}
}

func TestDefaultHeavyWorkers_EnvOverride(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("CRIVO_MAX_HEAVY", "4")
	if got := defaultHeavyWorkers(); got != 4 {
		t.Fatalf("defaultHeavyWorkers() with CRIVO_MAX_HEAVY=4 = %d, want 4", got)
	}
}

func TestDefaultHeavyWorkers_EnvInvalidKeepsDefault(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("CRIVO_MAX_HEAVY", "banana")
	if got := defaultHeavyWorkers(); got != 1 {
		t.Fatalf("defaultHeavyWorkers() with invalid env = %d, want local default 1", got)
	}
}
