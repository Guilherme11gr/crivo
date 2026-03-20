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

	// Calculate ratings
	result.Ratings = map[string]domain.Rating{
		"Reliability":     CalculateReliability(allIssues),
		"Security":        CalculateSecurity(allIssues),
		"Maintainability": CalculateMaintainability(allIssues, 1000), // TODO: count actual LOC
	}

	// Count totals
	result.TotalIssues = len(allIssues)

	// Evaluate conditions based on check statuses
	hasFailure := false
	for _, check := range result.Checks {
		if check.Status == domain.StatusFailed {
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
