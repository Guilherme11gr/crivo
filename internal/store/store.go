package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/guilherme11gr/crivo/internal/domain"
	_ "modernc.org/sqlite"
)

// Store persists analysis results and issue lifecycle in SQLite
type Store struct {
	db *sql.DB
}

// AnalysisRecord is a stored analysis snapshot
type AnalysisRecord struct {
	ID          int64
	ProjectDir  string
	Branch      string
	Commit      string
	Status      string
	TotalIssues int
	Ratings     map[string]domain.Rating
	Metrics     map[string]float64
	CreatedAt   time.Time
}

// IssueRecord tracks issue lifecycle (false positive, won't fix, etc.)
type IssueRecord struct {
	Fingerprint string
	RuleID      string
	File        string
	Line        int
	Status      string // "open", "false_positive", "wont_fix", "fixed"
	Resolution  string
	UpdatedAt   time.Time
}

// TrendPoint is a single data point for sparkline rendering
type TrendPoint struct {
	Date        time.Time
	TotalIssues int
	Bugs        int
	Vulns       int
	CodeSmells  int
	Coverage    float64
	Duplication float64
}

const schema = `
CREATE TABLE IF NOT EXISTS analyses (
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
);

CREATE TABLE IF NOT EXISTS issue_lifecycle (
	fingerprint TEXT PRIMARY KEY,
	rule_id TEXT NOT NULL,
	file TEXT NOT NULL,
	line INTEGER DEFAULT 0,
	message TEXT DEFAULT '',
	status TEXT DEFAULT 'open',
	resolution TEXT DEFAULT '',
	first_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_analyses_project ON analyses(project_dir);
CREATE INDEX IF NOT EXISTS idx_analyses_created ON analyses(created_at);
CREATE INDEX IF NOT EXISTS idx_lifecycle_status ON issue_lifecycle(status);
`

// Open creates or opens the SQLite store
func Open(projectDir string) (*Store, error) {
	storeDir := filepath.Join(projectDir, ".qualitygate")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}

	dbPath := filepath.Join(storeDir, "history.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveAnalysis persists an analysis result
func (s *Store) SaveAnalysis(result *domain.AnalysisResult, branch, commit string) (int64, error) {
	ratingsJSON, _ := json.Marshal(result.Ratings)

	// Aggregate metrics from all checks
	metrics := map[string]float64{}
	for _, check := range result.Checks {
		for k, v := range check.Metrics {
			metrics[check.ID+"_"+k] = v
		}
	}
	metricsJSON, _ := json.Marshal(metrics)
	checksJSON, _ := json.Marshal(result.Checks)

	res, err := s.db.Exec(`
		INSERT INTO analyses (project_dir, branch, commit_hash, status, total_issues, ratings_json, metrics_json, checks_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		result.ProjectDir, branch, commit, string(result.Status),
		result.TotalIssues, string(ratingsJSON), string(metricsJSON), string(checksJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("insert analysis: %w", err)
	}

	return res.LastInsertId()
}

// GetTrend returns the last N analysis points for trend/sparkline display
func (s *Store) GetTrend(projectDir string, limit int) ([]TrendPoint, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
		SELECT created_at, total_issues, metrics_json
		FROM analyses
		WHERE project_dir = ?
		ORDER BY created_at DESC
		LIMIT ?`, projectDir, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TrendPoint
	for rows.Next() {
		var p TrendPoint
		var metricsStr string
		if err := rows.Scan(&p.Date, &p.TotalIssues, &metricsStr); err != nil {
			continue
		}

		var metrics map[string]float64
		if json.Unmarshal([]byte(metricsStr), &metrics) == nil {
			p.Coverage = metrics["coverage_lines"]
			p.Duplication = metrics["duplication_percentage"]
			if val, ok := metrics["typescript_type_errors"]; ok {
				p.Bugs = int(val)
			}
		}

		points = append(points, p)
	}

	// Reverse so oldest is first (for sparklines)
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}

	return points, nil
}

// GetLastMetrics returns the metrics from the most recent saved analysis for a project.
// Used for baseline comparison (e.g., coverage regression detection).
func (s *Store) GetLastMetrics(projectDir string) (map[string]float64, error) {
	var metricsStr string
	err := s.db.QueryRow(`
		SELECT metrics_json FROM analyses
		WHERE project_dir = ?
		ORDER BY created_at DESC
		LIMIT 1`, projectDir).Scan(&metricsStr)
	if err != nil {
		return nil, err
	}

	var metrics map[string]float64
	if err := json.Unmarshal([]byte(metricsStr), &metrics); err != nil {
		return nil, err
	}
	return metrics, nil
}

// MarkIssue updates the lifecycle status of an issue (false positive, won't fix)
func (s *Store) MarkIssue(fingerprint, status, resolution string) error {
	_, err := s.db.Exec(`
		UPDATE issue_lifecycle
		SET status = ?, resolution = ?, updated_at = CURRENT_TIMESTAMP
		WHERE fingerprint = ?`,
		status, resolution, fingerprint)
	return err
}

// SyncIssues updates the issue lifecycle table with current findings
func (s *Store) SyncIssues(issues []domain.Issue) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO issue_lifecycle (fingerprint, rule_id, file, line, message, status)
		VALUES (?, ?, ?, ?, ?, 'open')
		ON CONFLICT(fingerprint) DO UPDATE SET
			file = excluded.file,
			line = excluded.line,
			message = excluded.message,
			updated_at = CURRENT_TIMESTAMP
		WHERE status = 'open'`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, issue := range issues {
		fp := issueFingerprint(issue)
		if _, err := stmt.Exec(fp, issue.RuleID, issue.File, issue.Line, issue.Message); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetSuppressedFingerprints returns fingerprints marked as false_positive or wont_fix
func (s *Store) GetSuppressedFingerprints() (map[string]bool, error) {
	rows, err := s.db.Query(`
		SELECT fingerprint FROM issue_lifecycle
		WHERE status IN ('false_positive', 'wont_fix')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]bool{}
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			continue
		}
		result[fp] = true
	}

	return result, nil
}

// issueFingerprint creates a stable identifier for an issue
func issueFingerprint(issue domain.Issue) string {
	return fmt.Sprintf("%s:%s:%s:%d", issue.Source, issue.RuleID, issue.File, issue.Line)
}

// Sparkline generates a sparkline string from trend points
func Sparkline(points []TrendPoint, getValue func(TrendPoint) float64) string {
	if len(points) == 0 {
		return ""
	}

	blocks := []rune("▁▂▃▄▅▆▇█")

	values := make([]float64, len(points))
	minVal := getValue(points[0])
	maxVal := minVal

	for i, p := range points {
		v := getValue(p)
		values[i] = v
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	spread := maxVal - minVal
	if spread == 0 {
		spread = 1
	}

	var result []rune
	for _, v := range values {
		idx := int((v - minVal) / spread * float64(len(blocks)-1))
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		if idx < 0 {
			idx = 0
		}
		result = append(result, blocks[idx])
	}

	return string(result)
}
