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

	// Details
	if verbose {
		for _, check := range result.Checks {
			if len(check.Details) > 0 {
				icon := statusIcons[check.Status]
				sColor := statusColors[check.Status]
				fmt.Println(color(fmt.Sprintf("  %s %s Details:", icon, check.Name), sColor, bold))
				for _, d := range check.Details {
					fmt.Println(color("    "+d, dim))
				}
				fmt.Println()
			}
		}
	} else {
		for _, check := range result.Checks {
			if (check.Status == domain.StatusFailed || check.Status == domain.StatusWarning) && len(check.Details) > 0 {
				icon := statusIcons[check.Status]
				sColor := statusColors[check.Status]
				fmt.Println(color(fmt.Sprintf("  %s %s:", icon, check.Name), sColor, bold))
				limit := min(len(check.Details), 5)
				for _, d := range check.Details[:limit] {
					fmt.Println(color("    "+d, dim))
				}
				if len(check.Details) > 5 {
					fmt.Println(color(fmt.Sprintf("    ... and %d more (use --verbose)", len(check.Details)-5), dim))
				}
				fmt.Println()
			}
		}
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
