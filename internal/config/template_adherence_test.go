package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilherme11gr/crivo/internal/check/providers/customrules"
	"github.com/guilherme11gr/crivo/internal/config"
	"gopkg.in/yaml.v3"
)

// TestTemplateAdheresToSchema guarantees templates/qualitygate.json never
// diverges from the real Config schema (plan 008): every key must map to a
// field (strict decode) and the example custom rules must compile cleanly.
func TestTemplateAdheresToSchema(t *testing.T) {
	// Go tests run with cwd = package dir; the template lives two levels up.
	path := filepath.Join("..", "..", "templates", "qualitygate.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	// Strict decode: any key that does not exist in Config (e.g. the legacy
	// "lint", "structural", "testExclude", "srcPattern" keys) fails the test.
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var cfg config.Config
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("template has keys that do not exist in the Config schema: %v", err)
	}

	if len(cfg.CustomRules) == 0 {
		t.Fatal("template should include an example custom rule")
	}

	// The example rules must compile — including their fixtures. semgrep is not
	// installed in CI, so the semgrep fixture is collected as a warning, never
	// an error; the ban-pattern fixture is validated for real.
	t.Setenv("CRIVO_NO_AUTO_INSTALL", "1")
	_, errs, _ := customrules.CompileRules(cfg.CustomRules)
	if len(errs) > 0 {
		t.Fatalf("template custom rules must compile cleanly, got %d errors: %v", len(errs), errs)
	}
}
