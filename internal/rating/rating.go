package rating

import (
	"github.com/anthropics/quality-gate/internal/domain"
)

// CalculateReliability returns A-E based on bug count by severity
func CalculateReliability(issues []domain.Issue) domain.Rating {
	var blocker, critical, major, minor int
	for _, issue := range issues {
		if issue.Type != domain.IssueTypeBug {
			continue
		}
		switch issue.Severity {
		case domain.SeverityBlocker:
			blocker++
		case domain.SeverityCritical:
			critical++
		case domain.SeverityMajor:
			major++
		case domain.SeverityMinor:
			minor++
		}
	}

	switch {
	case blocker > 0:
		return domain.RatingE
	case critical > 0:
		return domain.RatingD
	case major > 0:
		return domain.RatingC
	case minor > 0:
		return domain.RatingB
	default:
		return domain.RatingA
	}
}

// CalculateSecurity returns A-E based on vulnerability count by severity
func CalculateSecurity(issues []domain.Issue) domain.Rating {
	var blocker, high, medium, low int
	for _, issue := range issues {
		if issue.Type != domain.IssueTypeVulnerability && issue.Type != domain.IssueTypeSecurityHotspot {
			continue
		}
		switch issue.Severity {
		case domain.SeverityBlocker:
			blocker++
		case domain.SeverityCritical:
			high++
		case domain.SeverityMajor:
			medium++
		case domain.SeverityMinor:
			low++
		}
	}

	switch {
	case blocker > 0:
		return domain.RatingE
	case high > 0:
		return domain.RatingD
	case medium > 0:
		return domain.RatingC
	case low > 0:
		return domain.RatingB
	default:
		return domain.RatingA
	}
}

// CalculateMaintainability returns A-E based on debt ratio
// debtMinutes = sum of effort estimates for code smells
// totalLines = lines of code in the project
func CalculateMaintainability(issues []domain.Issue, totalLines int) domain.Rating {
	if totalLines <= 0 {
		return domain.RatingA
	}

	debtMinutes := 0
	for _, issue := range issues {
		if issue.Type != domain.IssueTypeCodeSmell {
			continue
		}
		debtMinutes += parseEffort(issue.Effort)
	}

	// Debt ratio = debt time / development time
	// Assume 30 min per line of code for development time
	devMinutes := totalLines * 30
	ratio := float64(debtMinutes) / float64(devMinutes) * 100

	switch {
	case ratio <= 5:
		return domain.RatingA
	case ratio <= 10:
		return domain.RatingB
	case ratio <= 20:
		return domain.RatingC
	case ratio <= 50:
		return domain.RatingD
	default:
		return domain.RatingE
	}
}

// parseEffort converts "5min", "10min", "1h" to minutes
func parseEffort(effort string) int {
	if effort == "" {
		return 5 // default
	}

	n := 0
	unit := ""
	for _, c := range effort {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			unit += string(c)
		}
	}

	switch unit {
	case "h":
		return n * 60
	case "d":
		return n * 480
	default: // "min", "m", ""
		return n
	}
}

// EvaluateQualityGate checks all conditions and returns pass/fail
func EvaluateQualityGate(result *domain.AnalysisResult) {
	allIssues := result.AllIssues()

	// Get LOC from complexity metrics if available
	totalLines := 1000
	for _, check := range result.Checks {
		if check.ID == "complexity" && check.Metrics != nil {
			if loc, ok := check.Metrics["total_lines"]; ok && loc > 0 {
				totalLines = int(loc)
			}
		}
	}

	// Calculate ratings
	result.Ratings = map[string]domain.Rating{
		"Reliability":     CalculateReliability(allIssues),
		"Security":        CalculateSecurity(allIssues),
		"Maintainability": CalculateMaintainability(allIssues, totalLines),
	}

	// Count totals
	result.TotalIssues = len(allIssues)

	// Build conditions from check results
	var conditions []domain.QualityGateCondition

	for _, check := range result.Checks {
		if check.Status == domain.StatusSkipped {
			continue
		}

		switch check.ID {
		case "typescript":
			errors := 0.0
			if check.Metrics != nil {
				errors = check.Metrics["errors"]
			}
			conditions = append(conditions, domain.QualityGateCondition{
				Metric:    "type_errors",
				Operator:  "lt",
				Threshold: 1,
				Actual:    errors,
				Passed:    errors == 0,
			})

		case "coverage":
			if check.Metrics != nil {
				if lines, ok := check.Metrics["lines"]; ok {
					conditions = append(conditions, domain.QualityGateCondition{
						Metric:    "coverage_lines",
						Operator:  "gt",
						Threshold: 60,
						Actual:    lines,
						Passed:    check.Status != domain.StatusFailed,
					})
				}
			}

		case "duplication":
			if check.Metrics != nil {
				if pct, ok := check.Metrics["percentage"]; ok {
					conditions = append(conditions, domain.QualityGateCondition{
						Metric:    "duplication_pct",
						Operator:  "lt",
						Threshold: 5,
						Actual:    pct,
						Passed:    check.Status != domain.StatusFailed,
					})
				}
			}

		case "eslint":
			errors := 0.0
			if check.Metrics != nil {
				errors = check.Metrics["errors"]
			}
			conditions = append(conditions, domain.QualityGateCondition{
				Metric:    "lint_errors",
				Operator:  "lt",
				Threshold: 1,
				Actual:    errors,
				Passed:    errors == 0,
			})

		case "secrets":
			secrets := 0.0
			if check.Metrics != nil {
				secrets = check.Metrics["secrets"]
			}
			conditions = append(conditions, domain.QualityGateCondition{
				Metric:    "secrets",
				Operator:  "lt",
				Threshold: 1,
				Actual:    secrets,
				Passed:    secrets == 0,
			})
		}
	}

	result.Conditions = conditions

	// Gate fails if any condition fails
	hasFailure := false
	for _, c := range conditions {
		if !c.Passed {
			hasFailure = true
			break
		}
	}
	// Also fail if any check is in error state
	for _, check := range result.Checks {
		if check.Status == domain.StatusFailed || check.Status == domain.StatusError {
			hasFailure = true
			break
		}
	}

	if hasFailure {
		result.Status = domain.GateFailed
	} else {
		result.Status = domain.GatePassed
	}
}
