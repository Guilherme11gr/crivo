package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primary   = lipgloss.Color("#7C3AED") // violet
	secondary = lipgloss.Color("#06B6D4") // cyan
	success   = lipgloss.Color("#22C55E") // green
	warning   = lipgloss.Color("#EAB308") // yellow
	danger    = lipgloss.Color("#EF4444") // red
	muted     = lipgloss.Color("#6B7280") // gray
	surface   = lipgloss.Color("#1E1E2E") // dark bg
	text      = lipgloss.Color("#CDD6F4") // light text
	subtle    = lipgloss.Color("#45475A") // subtle border

	// Rating colors
	ratingColors = map[string]lipgloss.Color{
		"A": success,
		"B": lipgloss.Color("#86EFAC"),
		"C": warning,
		"D": lipgloss.Color("#FB923C"),
		"E": danger,
	}

	// Layout
	appStyle = lipgloss.NewStyle().
			Padding(1, 2)

	// Tab bar
	activeTab = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primary).
			Padding(0, 2).
			MarginRight(1)

	inactiveTab = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 2).
			MarginRight(1)

	// Cards / Boxes
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(1, 2).
			MarginBottom(1)

	// Rating badge
	ratingBadge = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			MarginRight(1)

	// Status indicators
	statusPassed = lipgloss.NewStyle().
			Foreground(success).
			Bold(true)

	statusFailed = lipgloss.NewStyle().
			Foreground(danger).
			Bold(true)

	statusWarning = lipgloss.NewStyle().
			Foreground(warning).
			Bold(true)

	// Issue list
	issueSelected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primary).
			Padding(0, 1).
			Width(80)

	issueNormal = lipgloss.NewStyle().
			Foreground(text).
			Padding(0, 1).
			Width(80)

	// Header
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondary).
			MarginBottom(1)

	// Help bar
	helpStyle = lipgloss.NewStyle().
			Foreground(muted).
			MarginTop(1)

	// Metric value
	metricLabel = lipgloss.NewStyle().
			Foreground(muted).
			Width(16)

	metricValue = lipgloss.NewStyle().
			Bold(true).
			Foreground(text)

	// Severity badges
	severityBlocker  = lipgloss.NewStyle().Foreground(danger).Bold(true)
	severityCritical = lipgloss.NewStyle().Foreground(danger)
	severityMajor    = lipgloss.NewStyle().Foreground(warning)
	severityMinor    = lipgloss.NewStyle().Foreground(muted)

	// Gate banner
	gatePassed = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(success).
			Padding(0, 3).
			Align(lipgloss.Center)

	gateFailed = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(danger).
			Padding(0, 3).
			Align(lipgloss.Center)
)

func severityStyle(severity string) lipgloss.Style {
	switch severity {
	case "blocker":
		return severityBlocker
	case "critical":
		return severityCritical
	case "major":
		return severityMajor
	default:
		return severityMinor
	}
}

func ratingStyle(rating string) lipgloss.Style {
	c, ok := ratingColors[rating]
	if !ok {
		c = muted
	}
	return ratingBadge.Foreground(c)
}
