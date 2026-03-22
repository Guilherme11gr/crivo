package output

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/quality-gate/internal/domain"
)

var noColor = os.Getenv("NO_COLOR") != "" || os.Getenv("CI") == "true"

// ANSI codes
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	white  = "\033[37m"
)

func color(text string, codes ...string) string {
	if noColor {
		return text
	}
	return strings.Join(codes, "") + text + reset
}

var statusIcons = map[domain.CheckStatus]string{
	domain.StatusPassed:  "✅",
	domain.StatusFailed:  "❌",
	domain.StatusWarning: "⚠️",
	domain.StatusSkipped: "⏭️",
	domain.StatusError:   "💥",
}

var statusColors = map[domain.CheckStatus]string{
	domain.StatusPassed:  green,
	domain.StatusFailed:  red,
	domain.StatusWarning: yellow,
	domain.StatusSkipped: dim,
	domain.StatusError:   red,
}

// PrintConsoleReport renders a box-drawing terminal report
func PrintConsoleReport(result *domain.AnalysisResult, verbose bool) {
	line := strings.Repeat("─", 58)

	fmt.Println()
	fmt.Println(color("  ┌"+line+"┐", dim))

	// Title
	gateIcon := "✅"
	gateText := "PASSED"
	gateColor := green
	if result.Status == domain.GateFailed {
		gateIcon = "❌"
		gateText = "FAILED"
		gateColor = red
	}

	title := fmt.Sprintf("QUALITY GATE: %s %s", gateIcon, gateText)
	padding := 58 - len(title) - 2
	left := padding / 2
	right := padding - left
	fmt.Printf("%s%s%s%s%s\n",
		color("  │", dim),
		strings.Repeat(" ", left+1),
		color(title, gateColor, bold),
		strings.Repeat(" ", right+1),
		color("│", dim),
	)

	fmt.Println(color("  ├"+line+"┤", dim))

	// Check results
	for _, check := range result.Checks {
		printCheckLine(check)
	}

	fmt.Println(color("  ├"+line+"┤", dim))

	// Footer
	footerText := fmt.Sprintf("Total: %s", formatDuration(result.TotalDuration))
	footerPad := 58 - len(footerText) - 2
	fmt.Printf("%s %s%s%s\n",
		color("  │", dim),
		color(footerText, dim),
		strings.Repeat(" ", max(0, footerPad)+1),
		color("│", dim),
	)
	fmt.Println(color("  └"+line+"┘", dim))
	fmt.Println()

	// Ratings
	if len(result.Ratings) > 0 {
		fmt.Println(color("  Ratings:", bold))
		for metric, rating := range result.Ratings {
			rColor := ratingColor(rating)
			fmt.Printf("    %s: %s\n", metric, color(string(rating), rColor, bold))
		}
		fmt.Println()
	}

	// Quality gate conditions
	failedConditions := 0
	for _, c := range result.Conditions {
		if !c.Passed {
			failedConditions++
		}
	}
	if failedConditions > 0 {
		fmt.Println(color("  Failed conditions:", red, bold))
		for _, c := range result.Conditions {
			if !c.Passed {
				fmt.Printf("    %s %s: %.1f (threshold: %.1f)\n",
					"❌", c.Metric, c.Actual, c.Threshold)
			}
		}
		fmt.Println()
	}

	// Details and issues
	for _, check := range result.Checks {
		showCheck := verbose || check.Status == domain.StatusFailed || check.Status == domain.StatusWarning
		if !showCheck {
			continue
		}

		hasContent := len(check.Details) > 0 || len(check.Issues) > 0
		if !hasContent {
			continue
		}

		icon := statusIcons[check.Status]
		sColor := statusColors[check.Status]
		fmt.Println(color(fmt.Sprintf("  %s %s:", icon, check.Name), sColor, bold))

		// Show summary details (thresholds, etc.)
		if len(check.Details) > 0 {
			limit := len(check.Details)
			if !verbose {
				limit = min(limit, 5)
			}
			for _, d := range check.Details[:limit] {
				fmt.Println(color("    "+d, dim))
			}
			if !verbose && len(check.Details) > 5 {
				fmt.Println(color(fmt.Sprintf("    ... and %d more", len(check.Details)-5), dim))
			}
		}

		// Show actionable issues with file:line
		if len(check.Issues) > 0 {
			fmt.Println()
			issueLimit := 10
			if verbose {
				issueLimit = 50
			}
			limit := min(len(check.Issues), issueLimit)
			for _, issue := range check.Issues[:limit] {
				sevIcon := severityIcon(issue.Severity)
				loc := fmt.Sprintf("%s:%d", issue.File, issue.Line)
				fmt.Printf("    %s %s %s\n",
					color(sevIcon, severityColor(issue.Severity)),
					color(loc, cyan),
					issue.Message,
				)
			}
			if len(check.Issues) > issueLimit {
				fmt.Println(color(fmt.Sprintf("    ... and %d more issues (use --verbose)", len(check.Issues)-issueLimit), dim))
			}
		}

		fmt.Println()
	}
}

func printCheckLine(check domain.CheckResult) {
	icon := statusIcons[check.Status]
	sColor := statusColors[check.Status]
	dur := formatDuration(check.Duration)

	name := padRight(check.Name, 16)
	durText := "(" + dur + ")"

	fmt.Printf("%s %s %s%s%s %s\n",
		color("  │", dim),
		icon,
		color(name, sColor),
		color(check.Summary, white),
		strings.Repeat(" ", max(0, 58-2-3-16-len(check.Summary)-len(durText)-2)),
		color(durText+" ", dim)+color("│", dim),
	)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func ratingColor(r domain.Rating) string {
	switch r {
	case domain.RatingA:
		return green
	case domain.RatingB:
		return green
	case domain.RatingC:
		return yellow
	case domain.RatingD:
		return red
	case domain.RatingE:
		return red
	default:
		return white
	}
}

func severityIcon(s domain.Severity) string {
	switch s {
	case domain.SeverityBlocker:
		return "!!!"
	case domain.SeverityCritical:
		return " !!"
	case domain.SeverityMajor:
		return "  !"
	case domain.SeverityMinor:
		return "  ~"
	default:
		return "  ·"
	}
}

func severityColor(s domain.Severity) string {
	switch s {
	case domain.SeverityBlocker, domain.SeverityCritical:
		return red
	case domain.SeverityMajor:
		return yellow
	default:
		return dim
	}
}

// PrintProgress prints a single-line progress update for a check
func PrintProgress(providerName, status string) {
	switch status {
	case "started":
		fmt.Printf("\r\033[K  ⏳ %s...", providerName)
	case "completed":
		fmt.Printf("\r\033[K")
	case "failed":
		fmt.Printf("\r\033[K")
	case "skipped":
		// don't print anything for skipped
	}
}
