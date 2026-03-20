package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/anthropics/quality-gate/internal/domain"
	"github.com/anthropics/quality-gate/internal/store"
)

// Tab represents a dashboard tab
type Tab int

const (
	TabDashboard Tab = iota
	TabIssues
	TabTrends
)

var tabNames = []string{"Dashboard", "Issues", "Trends"}

// Model is the top-level bubbletea model
type Model struct {
	analysis    *domain.AnalysisResult
	trendPoints []store.TrendPoint

	activeTab   Tab
	width       int
	height      int

	// Issues tab
	issueList     []domain.Issue
	issueCursor   int
	issueFilter   string // "" = all, or severity/type filter
	issueViewport viewport.Model

	// Ready state
	ready bool
}

// New creates a new TUI model
func New(analysis *domain.AnalysisResult, trends []store.TrendPoint) Model {
	allIssues := analysis.AllIssues()

	return Model{
		analysis:    analysis,
		trendPoints: trends,
		activeTab:   TabDashboard,
		issueList:   allIssues,
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.SetWindowTitle("Quality Gate")
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			m.activeTab = (m.activeTab + 1) % 3
			m.issueCursor = 0
		case "shift+tab", "left", "h":
			m.activeTab = (m.activeTab + 2) % 3
			m.issueCursor = 0
		case "j", "down":
			if m.activeTab == TabIssues && m.issueCursor < len(m.issueList)-1 {
				m.issueCursor++
			}
		case "k", "up":
			if m.activeTab == TabIssues && m.issueCursor > 0 {
				m.issueCursor--
			}
		case "1":
			m.activeTab = TabDashboard
		case "2":
			m.activeTab = TabIssues
		case "3":
			m.activeTab = TabTrends
		case "a":
			m.issueFilter = ""
			m.issueList = m.analysis.AllIssues()
			m.issueCursor = 0
		case "b":
			m.issueFilter = "bug"
			m.issueList = filterByType(m.analysis.AllIssues(), domain.IssueTypeBug)
			m.issueCursor = 0
		case "v":
			m.issueFilter = "vulnerability"
			m.issueList = filterByType(m.analysis.AllIssues(), domain.IssueTypeVulnerability)
			m.issueCursor = 0
		case "s":
			m.issueFilter = "code_smell"
			m.issueList = filterByType(m.analysis.AllIssues(), domain.IssueTypeCodeSmell)
			m.issueCursor = 0
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder

	// Tab bar
	b.WriteString(m.renderTabBar())
	b.WriteString("\n\n")

	// Content
	switch m.activeTab {
	case TabDashboard:
		b.WriteString(m.renderDashboard())
	case TabIssues:
		b.WriteString(m.renderIssues())
	case TabTrends:
		b.WriteString(m.renderTrends())
	}

	// Help bar
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return appStyle.Render(b.String())
}

func (m Model) renderTabBar() string {
	var tabs []string
	for i, name := range tabNames {
		if Tab(i) == m.activeTab {
			tabs = append(tabs, activeTab.Render(name))
		} else {
			tabs = append(tabs, inactiveTab.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) renderDashboard() string {
	var sections []string

	// Quality Gate banner
	if m.analysis.Status == domain.GatePassed {
		sections = append(sections, gatePassed.Render("  QUALITY GATE: PASSED  "))
	} else {
		sections = append(sections, gateFailed.Render("  QUALITY GATE: FAILED  "))
	}
	sections = append(sections, "")

	// Ratings row
	if len(m.analysis.Ratings) > 0 {
		var ratingParts []string
		for metric, rating := range m.analysis.Ratings {
			badge := ratingStyle(string(rating)).Render(string(rating))
			ratingParts = append(ratingParts, fmt.Sprintf("%s %s", badge, metric))
		}
		sections = append(sections, headerStyle.Render("Ratings"))
		sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Top, ratingParts...))
		sections = append(sections, "")
	}

	// Check results
	sections = append(sections, headerStyle.Render("Checks"))

	for _, check := range m.analysis.Checks {
		icon := statusIcon(check.Status)
		style := statusStyleForCheck(check.Status)
		dur := formatDuration(check.Duration)

		name := padRight(check.Name, 20)
		line := fmt.Sprintf("  %s %s %s  %s",
			icon,
			style.Render(name),
			check.Summary,
			lipgloss.NewStyle().Foreground(muted).Render("("+dur+")"),
		)
		sections = append(sections, line)
	}
	sections = append(sections, "")

	// Issue summary
	sections = append(sections, headerStyle.Render("Issue Summary"))

	bugs := m.analysis.CountByType(domain.IssueTypeBug)
	vulns := m.analysis.CountByType(domain.IssueTypeVulnerability)
	smells := m.analysis.CountByType(domain.IssueTypeCodeSmell)

	sections = append(sections, fmt.Sprintf("  %s Bugs: %d    %s Vulnerabilities: %d    %s Code Smells: %d",
		severityStyle("critical").Render("●"), bugs,
		severityStyle("blocker").Render("●"), vulns,
		severityStyle("minor").Render("●"), smells,
	))
	sections = append(sections, "")

	// Metrics from checks
	sections = append(sections, headerStyle.Render("Metrics"))
	for _, check := range m.analysis.Checks {
		if check.Metrics == nil {
			continue
		}
		for k, v := range check.Metrics {
			label := metricLabel.Render(check.Name + "." + k)
			value := metricValue.Render(fmt.Sprintf("%.1f", v))
			sections = append(sections, fmt.Sprintf("  %s %s", label, value))
		}
	}

	return strings.Join(sections, "\n")
}

func (m Model) renderIssues() string {
	var sections []string

	// Filter info
	filterText := "All issues"
	if m.issueFilter != "" {
		filterText = "Filter: " + m.issueFilter
	}
	sections = append(sections, headerStyle.Render(fmt.Sprintf("Issues (%d) — %s", len(m.issueList), filterText)))
	sections = append(sections, "")

	if len(m.issueList) == 0 {
		sections = append(sections, lipgloss.NewStyle().Foreground(success).Render("  No issues found!"))
		return strings.Join(sections, "\n")
	}

	// Issue list (scrollable window)
	maxVisible := m.height - 12
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if m.issueCursor >= maxVisible {
		startIdx = m.issueCursor - maxVisible + 1
	}

	endIdx := startIdx + maxVisible
	if endIdx > len(m.issueList) {
		endIdx = len(m.issueList)
	}

	for i := startIdx; i < endIdx; i++ {
		issue := m.issueList[i]

		sev := severityStyle(string(issue.Severity)).Render(fmt.Sprintf("%-8s", issue.Severity))
		loc := fmt.Sprintf("%s:%d", issue.File, issue.Line)
		if len(loc) > 40 {
			loc = "..." + loc[len(loc)-37:]
		}

		msg := issue.Message
		if len(msg) > 50 {
			msg = msg[:47] + "..."
		}

		line := fmt.Sprintf(" %s %-40s %s", sev, loc, msg)

		if i == m.issueCursor {
			sections = append(sections, issueSelected.Render(line))
		} else {
			sections = append(sections, issueNormal.Render(line))
		}
	}

	// Scroll indicator
	if len(m.issueList) > maxVisible {
		pct := float64(m.issueCursor) / float64(len(m.issueList)-1) * 100
		sections = append(sections, "")
		sections = append(sections, lipgloss.NewStyle().Foreground(muted).Render(
			fmt.Sprintf("  %d/%d (%.0f%%)", m.issueCursor+1, len(m.issueList), pct)))
	}

	// Selected issue detail
	if m.issueCursor < len(m.issueList) {
		issue := m.issueList[m.issueCursor]
		sections = append(sections, "")
		detail := cardStyle.Render(
			fmt.Sprintf("Rule: %s\nFile: %s:%d:%d\nType: %s\nSeverity: %s\nSource: %s\nEffort: %s\n\n%s",
				issue.RuleID, issue.File, issue.Line, issue.Column,
				issue.Type, issue.Severity, issue.Source, issue.Effort,
				issue.Message,
			),
		)
		sections = append(sections, detail)
	}

	return strings.Join(sections, "\n")
}

func (m Model) renderTrends() string {
	var sections []string

	sections = append(sections, headerStyle.Render("Trends"))
	sections = append(sections, "")

	if len(m.trendPoints) < 2 {
		sections = append(sections, lipgloss.NewStyle().Foreground(muted).Render(
			"  Not enough data yet. Run `qg run --save` a few more times."))
		sections = append(sections, "")
		sections = append(sections, lipgloss.NewStyle().Foreground(muted).Render(
			fmt.Sprintf("  Currently: %d data point(s)", len(m.trendPoints))))
		return strings.Join(sections, "\n")
	}

	first := m.trendPoints[0]
	last := m.trendPoints[len(m.trendPoints)-1]

	// Issues trend
	issuesSpark := store.Sparkline(m.trendPoints, func(p store.TrendPoint) float64 {
		return float64(p.TotalIssues)
	})
	trend := trendArrow(float64(first.TotalIssues), float64(last.TotalIssues), true)
	sections = append(sections, fmt.Sprintf("  Issues:      %s  %d → %d %s",
		issuesSpark, first.TotalIssues, last.TotalIssues, trend))

	// Coverage trend
	covSpark := store.Sparkline(m.trendPoints, func(p store.TrendPoint) float64 {
		return p.Coverage
	})
	trend = trendArrow(first.Coverage, last.Coverage, false)
	sections = append(sections, fmt.Sprintf("  Coverage:    %s  %.1f%% → %.1f%% %s",
		covSpark, first.Coverage, last.Coverage, trend))

	// Duplication trend
	dupSpark := store.Sparkline(m.trendPoints, func(p store.TrendPoint) float64 {
		return p.Duplication
	})
	trend = trendArrow(first.Duplication, last.Duplication, true)
	sections = append(sections, fmt.Sprintf("  Duplication: %s  %.1f%% → %.1f%% %s",
		dupSpark, first.Duplication, last.Duplication, trend))

	sections = append(sections, "")
	sections = append(sections, lipgloss.NewStyle().Foreground(muted).Render(
		fmt.Sprintf("  %d data points from %s to %s",
			len(m.trendPoints),
			first.Date.Format("2006-01-02"),
			last.Date.Format("2006-01-02"),
		)))

	return strings.Join(sections, "\n")
}

func (m Model) renderHelp() string {
	var help string
	switch m.activeTab {
	case TabDashboard:
		help = "tab/←→: switch tabs • q: quit"
	case TabIssues:
		help = "↑↓/jk: navigate • a: all • b: bugs • v: vulns • s: smells • tab: switch • q: quit"
	case TabTrends:
		help = "tab/←→: switch tabs • q: quit"
	}
	return helpStyle.Render(help)
}

// Run starts the TUI
func Run(analysis *domain.AnalysisResult, trends []store.TrendPoint) error {
	m := New(analysis, trends)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Helper functions

func statusIcon(s domain.CheckStatus) string {
	switch s {
	case domain.StatusPassed:
		return "✅"
	case domain.StatusFailed:
		return "❌"
	case domain.StatusWarning:
		return "⚠️ "
	case domain.StatusSkipped:
		return "⏭️ "
	case domain.StatusError:
		return "💥"
	default:
		return "? "
	}
}

func statusStyleForCheck(s domain.CheckStatus) lipgloss.Style {
	switch s {
	case domain.StatusPassed:
		return statusPassed
	case domain.StatusFailed:
		return statusFailed
	case domain.StatusWarning:
		return statusWarning
	default:
		return lipgloss.NewStyle().Foreground(muted)
	}
}

func filterByType(issues []domain.Issue, t domain.IssueType) []domain.Issue {
	var result []domain.Issue
	for _, i := range issues {
		if i.Type == t {
			result = append(result, i)
		}
	}
	return result
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func formatDuration(d interface{ Seconds() float64 }) string {
	secs := d.Seconds()
	if secs < 1 {
		return fmt.Sprintf("%.0fms", secs*1000)
	}
	return fmt.Sprintf("%.1fs", secs)
}

func trendArrow(first, last float64, lowerIsBetter bool) string {
	if first == last {
		return lipgloss.NewStyle().Foreground(muted).Render("→")
	}

	improving := last < first
	if !lowerIsBetter {
		improving = last > first
	}

	if improving {
		return lipgloss.NewStyle().Foreground(success).Render("↓ improving")
	}
	return lipgloss.NewStyle().Foreground(danger).Render("↑ degrading")
}
