package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/guilherme11gr/crivo/internal/domain"
)

func TestOpenAndSave(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	result := &domain.AnalysisResult{
		ProjectDir:    dir,
		Status:        domain.GatePassed,
		TotalIssues:   5,
		TotalDuration: 10 * time.Second,
		Timestamp:     time.Now(),
		Ratings: map[string]domain.Rating{
			"Reliability": domain.RatingA,
		},
		Checks: []domain.CheckResult{
			{
				ID:     "typescript",
				Status: domain.StatusPassed,
				Metrics: map[string]float64{
					"errors": 0,
				},
			},
		},
	}

	id, err := s.SaveAnalysis(result, "main", "abc123", "overall")
	if err != nil {
		t.Fatalf("SaveAnalysis: %v", err)
	}
	if id <= 0 {
		t.Error("Expected positive ID")
	}
}

func TestOpen_MigratesLegacyDatabaseWithoutModeColumn(t *testing.T) {
	// Databases created before the mode column existed must be migrated on
	// open — otherwise SaveAnalysis fails with "no such column: mode".
	dir := t.TempDir()
	storeDir := filepath.Join(dir, ".qualitygate")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacyDB, err := sql.Open("sqlite", filepath.Join(storeDir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `
CREATE TABLE analyses (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_dir TEXT NOT NULL,
	branch TEXT DEFAULT '',
	commit_hash TEXT DEFAULT '',
	status TEXT NOT NULL,
	total_issues INTEGER DEFAULT 0,
	ratings_json TEXT DEFAULT '{}',
	metrics_json TEXT DEFAULT '{}',
	checks_json TEXT DEFAULT '[]',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`
	if _, err := legacyDB.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	legacyDB.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open on legacy db: %v", err)
	}
	defer s.Close()

	result := &domain.AnalysisResult{
		ProjectDir: dir,
		Status:     domain.GatePassed,
		Checks: []domain.CheckResult{
			{ID: "coverage", Metrics: map[string]float64{"lines": 60}},
		},
	}
	if _, err := s.SaveAnalysis(result, "main", "", "overall"); err != nil {
		t.Fatalf("SaveAnalysis after migration: %v", err)
	}

	metrics, err := s.GetLastMetrics(dir, "main", "overall")
	if err != nil {
		t.Fatalf("GetLastMetrics after migration: %v", err)
	}
	if got := metrics["coverage_lines"]; got != 60 {
		t.Fatalf("coverage_lines = %v, want 60", got)
	}
}

func TestGetLastMetrics_FiltersByBranchAndMode(t *testing.T) {	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	save := func(branch, mode string, lines float64) {
		result := &domain.AnalysisResult{
			ProjectDir: dir,
			Status:     domain.GatePassed,
			Checks: []domain.CheckResult{
				{ID: "coverage", Metrics: map[string]float64{"lines": lines}},
			},
		}
		if _, err := s.SaveAnalysis(result, branch, "", mode); err != nil {
			t.Fatalf("SaveAnalysis(%s,%s): %v", branch, mode, err)
		}
	}

	// Same branch+mode pair: latest wins.
	save("main", "overall", 60)
	save("main", "overall", 75)
	// Different branch and different mode must not leak into the baseline.
	save("feature", "overall", 90)
	save("main", "new-code", 95)

	metrics, err := s.GetLastMetrics(dir, "main", "overall")
	if err != nil {
		t.Fatalf("GetLastMetrics: %v", err)
	}
	if got := metrics["coverage_lines"]; got != 75 {
		t.Fatalf("coverage_lines = %v, want 75 (same branch+mode pair)", got)
	}

	// A pair with no saved run returns an error (no baseline).
	if _, err := s.GetLastMetrics(dir, "other", "overall"); err == nil {
		t.Fatal("expected error for branch with no saved run")
	}
}

func TestTrend(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Save 3 analyses
	for i := 0; i < 3; i++ {
		result := &domain.AnalysisResult{
			ProjectDir:  dir,
			Status:      domain.GatePassed,
			TotalIssues: i * 5,
			Checks: []domain.CheckResult{
				{
					ID:      "coverage",
					Metrics: map[string]float64{"lines": float64(60 + i*10)},
				},
			},
		}
		s.SaveAnalysis(result, "main", "", "overall")
	}

	points, err := s.GetTrend(dir, 10)
	if err != nil {
		t.Fatalf("GetTrend: %v", err)
	}
	if len(points) != 3 {
		t.Errorf("got %d points, want 3", len(points))
	}
}

func TestSparkline(t *testing.T) {
	points := []TrendPoint{
		{TotalIssues: 10},
		{TotalIssues: 20},
		{TotalIssues: 5},
		{TotalIssues: 15},
	}

	result := Sparkline(points, func(p TrendPoint) float64 {
		return float64(p.TotalIssues)
	})

	if len(result) == 0 {
		t.Error("Sparkline returned empty string")
	}
	// Should have 4 characters
	runes := []rune(result)
	if len(runes) != 4 {
		t.Errorf("Sparkline has %d chars, want 4", len(runes))
	}
}

func TestIssueLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	issues := []domain.Issue{
		{RuleID: "no-var", File: "a.ts", Line: 1, Source: "typescript", Message: "Use let"},
		{RuleID: "no-any", File: "b.ts", Line: 5, Source: "typescript", Message: "No any"},
	}

	if err := s.SyncIssues(issues); err != nil {
		t.Fatalf("SyncIssues: %v", err)
	}

	// Mark one as false positive
	fp := "typescript:no-var:a.ts:1"
	if err := s.MarkIssue(fp, "false_positive", "Not applicable"); err != nil {
		t.Fatalf("MarkIssue: %v", err)
	}

	suppressed, err := s.GetSuppressedFingerprints()
	if err != nil {
		t.Fatalf("GetSuppressed: %v", err)
	}
	if !suppressed[fp] {
		t.Error("Expected fingerprint to be suppressed")
	}
	if len(suppressed) != 1 {
		t.Errorf("Expected 1 suppressed, got %d", len(suppressed))
	}
}
