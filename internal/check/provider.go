package check

import (
	"context"

	"github.com/anthropics/quality-gate/internal/config"
	"github.com/anthropics/quality-gate/internal/domain"
)

// Provider is the core abstraction — each check implements this interface
type Provider interface {
	// Name returns the human-readable name (e.g. "Type Safety")
	Name() string

	// ID returns the unique identifier (e.g. "typescript")
	ID() string

	// Detect returns true if this check is applicable to the project
	Detect(ctx context.Context, projectDir string) bool

	// Analyze runs the check and returns results
	Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error)
}
