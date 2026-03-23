package check

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
)

// ProgressEvent is emitted as checks run (for TUI/console progress)
type ProgressEvent struct {
	ProviderID   string
	ProviderName string
	Status       string // "started", "completed", "failed", "skipped"
	Result       *domain.CheckResult
}

// Runner orchestrates parallel execution of checks
type Runner struct {
	registry   *Registry
	maxWorkers int
}

// NewRunner creates a runner with a concurrency limit
func NewRunner(registry *Registry, maxWorkers int) *Runner {
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU()
	}
	return &Runner{
		registry:   registry,
		maxWorkers: maxWorkers,
	}
}

// Run executes all applicable checks in parallel and returns results
func (r *Runner) Run(ctx context.Context, projectDir string, cfg *config.Config, progressCh chan<- ProgressEvent) ([]domain.CheckResult, error) {
	providers := r.registry.All()

	// Filter to enabled and detected providers
	var active []Provider
	for _, p := range providers {
		if !isCheckEnabled(p.ID(), cfg) {
			if progressCh != nil {
				progressCh <- ProgressEvent{
					ProviderID:   p.ID(),
					ProviderName: p.Name(),
					Status:       "skipped",
					Result: &domain.CheckResult{
						Name:    p.Name(),
						ID:      p.ID(),
						Status:  domain.StatusSkipped,
						Summary: "Disabled in config",
					},
				}
			}
			continue
		}

		if !p.Detect(ctx, projectDir) {
			if progressCh != nil {
				progressCh <- ProgressEvent{
					ProviderID:   p.ID(),
					ProviderName: p.Name(),
					Status:       "skipped",
					Result: &domain.CheckResult{
						Name:    p.Name(),
						ID:      p.ID(),
						Status:  domain.StatusSkipped,
						Summary: "Not detected in project",
					},
				}
			}
			continue
		}

		active = append(active, p)
	}

	// Run checks in parallel with semaphore
	sem := make(chan struct{}, r.maxWorkers)
	var mu sync.Mutex
	results := make([]domain.CheckResult, 0, len(active))
	var wg sync.WaitGroup

	for _, p := range active {
		wg.Add(1)

		go func(provider Provider) {
			defer wg.Done()

			sem <- struct{}{} // acquire
			defer func() { <-sem }() // release

			if progressCh != nil {
				progressCh <- ProgressEvent{
					ProviderID:   provider.ID(),
					ProviderName: provider.Name(),
					Status:       "started",
				}
			}

			// Per-check timeout (5 minutes default)
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			result, err := provider.Analyze(checkCtx, projectDir, cfg)
			if err != nil {
				result = &domain.CheckResult{
					Name:    provider.Name(),
					ID:      provider.ID(),
					Status:  domain.StatusError,
					Summary: fmt.Sprintf("Error: %s", err.Error()),
				}
			}

			mu.Lock()
			results = append(results, *result)
			mu.Unlock()

			if progressCh != nil {
				status := "completed"
				if result.Status == domain.StatusError {
					status = "failed"
				}
				progressCh <- ProgressEvent{
					ProviderID:   provider.ID(),
					ProviderName: provider.Name(),
					Status:       status,
					Result:       result,
				}
			}
		}(p)
	}

	wg.Wait()

	if progressCh != nil {
		close(progressCh)
	}

	return results, nil
}

func isCheckEnabled(id string, cfg *config.Config) bool {
	switch id {
	case "typescript":
		return cfg.Checks.Typescript
	case "eslint":
		return cfg.Checks.ESLint
	case "coverage":
		return cfg.Checks.Coverage
	case "duplication":
		return cfg.Checks.Duplication
	case "semgrep":
		return cfg.Checks.Semgrep
	case "secrets":
		return cfg.Checks.Secrets
	case "dead-code":
		return cfg.Checks.DeadCode
	default:
		return true
	}
}
