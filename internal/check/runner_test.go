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
