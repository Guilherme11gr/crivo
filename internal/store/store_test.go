package store

import (
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
				ID:     "eslint",
				Status: domain.StatusPassed,
				Metrics: map[string]float64{
					"errors": 0,
				},
			},
		},
	}

	id, err := s.SaveAnalysis(result, "main", "abc123")
	if err != nil {
		t.Fatalf("SaveAnalysis: %v", err)
	}
	if id <= 0 {
		t.Error("Expected positive ID")
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
		s.SaveAnalysis(result, "main", "")
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
		{RuleID: "no-var", File: "a.ts", Line: 1, Source: "eslint", Message: "Use let"},
		{RuleID: "no-any", File: "b.ts", Line: 5, Source: "eslint", Message: "No any"},
	}

	if err := s.SyncIssues(issues); err != nil {
		t.Fatalf("SyncIssues: %v", err)
	}

	// Mark one as false positive
	fp := "eslint:no-var:a.ts:1"
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
