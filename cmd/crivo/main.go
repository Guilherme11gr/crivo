package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	qualitygate "github.com/guilherme11gr/crivo"
	"github.com/guilherme11gr/crivo/internal/check"
	"github.com/guilherme11gr/crivo/internal/check/providers/complexity"
	"github.com/guilherme11gr/crivo/internal/check/providers/coverage"
	"github.com/guilherme11gr/crivo/internal/check/providers/customrules"
	"github.com/guilherme11gr/crivo/internal/check/providers/deadcode"
	"github.com/guilherme11gr/crivo/internal/check/providers/duplication"
	"github.com/guilherme11gr/crivo/internal/check/providers/secrets"
	"github.com/guilherme11gr/crivo/internal/check/providers/semgrep"
	"github.com/guilherme11gr/crivo/internal/check/providers/typescript"
	"github.com/guilherme11gr/crivo/internal/config"
	"github.com/guilherme11gr/crivo/internal/domain"
	gitutil "github.com/guilherme11gr/crivo/internal/git"
	"github.com/guilherme11gr/crivo/internal/output"
	"github.com/guilherme11gr/crivo/internal/rating"
	"github.com/guilherme11gr/crivo/internal/store"
	"github.com/guilherme11gr/crivo/internal/tui"
)

var version = "3.4.0"

const helpText = `
  quality-gate — Lightweight quality gate for code analysis

  Usage:
    crivo [command] [options]

  Commands:
    run         Run quality gate checks (default)
    init        Initialize quality gate in current project
    trends      Show trend history (sparklines)
    version     Show version

  Options:
    --json       Output structured JSON (for AI agents)
    --sarif FILE Save SARIF 2.1.0 report to file
    --md FILE    Save markdown report to file
    --verbose    Show all details (not just failures)
    --new-code   Only analyze new/changed code (vs default branch)
    --branch X   Compare against branch X (default: auto-detect)
    --disable X  Disable one or more checks for this run (repeat or use commas)
    --tui        Interactive TUI dashboard (bubbletea)
    --save       Save results to local history (.qualitygate/history.db)
    --policy P   Gate policy: release (default), strict, informational
    --help       Show this help message
`

type options struct {
	command        string
	jsonOutput     bool
	sarifFile      string
	mdOutput       string
	verbose        bool
	newCode        bool
	branch         string
	disabledChecks map[string]bool
	save           bool
	tuiMode        bool
	policy         string
}

func main() {
	opts := parseArgs(os.Args[1:])

	switch opts.command {
	case "help":
		fmt.Print(helpText)
		os.Exit(0)
	case "version":
		fmt.Printf("quality-gate v%s\n", version)
		os.Exit(0)
	case "init":
		runInit()
	case "trends":
		runTrends()
	case "run":
		os.Exit(runAnalysis(opts))
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", opts.command)
		fmt.Print(helpText)
		os.Exit(1)
	}
}

func parseArgs(args []string) options {
	opts := options{
		command:        "run",
		disabledChecks: map[string]bool{},
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "run", "init", "version", "trends":
			opts.command = arg
		case "--json":
			opts.jsonOutput = true
		case "--sarif":
			if i+1 < len(args) {
				i++
				opts.sarifFile = args[i]
			}
		case "--md", "--markdown", "--output", "-o":
			if i+1 < len(args) {
				i++
				opts.mdOutput = args[i]
			}
		case "--verbose", "-v":
			opts.verbose = true
		case "--new-code":
			opts.newCode = true
		case "--branch":
			if i+1 < len(args) {
				i++
				opts.branch = args[i]
			}
		case "--disable":
			if i+1 < len(args) {
				i++
				for _, id := range parseCheckList(args[i]) {
					opts.disabledChecks[id] = true
				}
			}
		case "--tui":
			opts.tuiMode = true
		case "--save":
			opts.save = true
		case "--policy":
			if i+1 < len(args) {
				i++
				opts.policy = args[i]
			}
		case "--help", "-h":
			opts.command = "help"
		default:
			if len(arg) > 0 && arg[0] == '-' {
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", arg)
				os.Exit(1)
			}
		}
	}

	return opts
}

func parseCheckList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToLower(part))
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func runAnalysis(opts options) int {
	projectDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	releaseRunLock, err := acquireRunLock(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	defer releaseRunLock()

	// Load config
	cfg, configSource := config.Load(projectDir)

	// CLI --policy overrides config
	if opts.policy != "" {
		cfg.Policy = config.GatePolicy(opts.policy)
	}

	// Setup context with Ctrl+C cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\n  Cancelled.")
		cancel()
	}()

	// Register all providers
	registry := check.NewRegistry()
	registry.Register(typescript.New())
	registry.Register(coverage.New())
	registry.Register(duplication.New())
	registry.Register(complexity.New())
	registry.Register(semgrep.New())
	registry.Register(secrets.New())
	registry.Register(deadcode.New())
	registry.Register(customrules.New())

	// Detect git info
	branch := ""
	commit := ""
	if gitutil.IsGitRepo(projectDir) {
		branch, _ = gitutil.CurrentBranch(ctx, projectDir)
		if opts.branch != "" {
			branch = opts.branch
		}
	}

	// Print header (unless JSON mode)
	if !opts.jsonOutput {
		fmt.Println()
		fmt.Println(color("  🔍 Quality Gate v"+version, cyan, bold))
		fmt.Println(color(fmt.Sprintf("  Config: %s", configSource), dim))
		fmt.Println(color(fmt.Sprintf("  Policy: %s", cfg.Policy), dim))
		if branch != "" {
			fmt.Println(color(fmt.Sprintf("  Branch: %s", branch), dim))
		}
		if opts.newCode {
			fmt.Println(color("  Mode: new code only", dim))
		}
		fmt.Println()
	}

	var changedFiles []gitutil.ChangedFile
	var changedLines []gitutil.ChangedLine
	changedFileSet := map[string]bool{}
	if opts.newCode && gitutil.IsGitRepo(projectDir) {
		baseBranch := gitutil.DefaultBranch(ctx, projectDir)
		if opts.branch != "" {
			baseBranch = opts.branch
		}

		currentBranch, _ := gitutil.CurrentBranch(ctx, projectDir)

		diffRef := baseBranch
		headRef := "HEAD"
		if currentBranch == baseBranch {
			diffRef = "HEAD"
			headRef = ""
		}

		changedFiles, _ = gitutil.GetChangedFiles(ctx, projectDir, diffRef, headRef)
		changedLines, _ = gitutil.GetChangedLines(ctx, projectDir, diffRef, headRef)
		for _, f := range changedFiles {
			changedFileSet[f.Path] = true
		}
		ctx = check.WithNewCodeScope(ctx, check.NewScope(changedFiles, changedLines))
	}

	// Run checks with progress
	runner := check.NewRunner(registry, 0)

	var progressCh chan check.ProgressEvent
	if !opts.jsonOutput {
		progressCh = make(chan check.ProgressEvent, 20)
		go func() {
			for ev := range progressCh {
				if ev.Status == "started" {
					output.PrintProgress(ev.ProviderName, ev.Status)
				} else if ev.Result != nil {
					output.PrintProgress(ev.ProviderName, ev.Status)
					icon := statusIcon(ev.Result.Status)
					fmt.Printf("  %s %s: %s\n", icon, ev.Result.Name, ev.Result.Summary)
				}
			}
		}()
	}

	start := time.Now()
	results, err := runner.Run(ctx, projectDir, cfg, opts.disabledChecks, progressCh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	// Build analysis result
	analysis := &domain.AnalysisResult{
		Version:       version,
		ProjectDir:    projectDir,
		Checks:        results,
		TotalDuration: time.Since(start),
		Timestamp:     time.Now(),
	}

	// If --new-code, filter issues to only changed files/lines
	if opts.newCode && gitutil.IsGitRepo(projectDir) {
		if len(changedFileSet) > 0 {
			for i := range analysis.Checks {
				filterCheckToNewCode(&analysis.Checks[i], changedFileSet, changedLines)
			}

			if !opts.jsonOutput && !opts.tuiMode {
				fmt.Println(color(fmt.Sprintf("  📝 New code: %d changed files", len(changedFileSet)), dim))
			}
		}
	}

	// Baseline comparison: check for regressions against last saved run
	applyBaselineComparison(analysis, projectDir, opts)

	// Calculate ratings and evaluate quality gate
	rating.EvaluateQualityGate(analysis, string(cfg.Policy))

	// Save to history if requested
	if opts.save {
		s, err := store.Open(projectDir)
		if err == nil {
			defer s.Close()
			s.SaveAnalysis(analysis, branch, commit)
			s.SyncIssues(analysis.AllIssues())
		}
	}

	// Output
	if opts.tuiMode {
		// Load trends for TUI
		var trends []store.TrendPoint
		s, err := store.Open(projectDir)
		if err == nil {
			trends, _ = s.GetTrend(projectDir, 20)
			s.Close()
		}
		if err := tui.Run(analysis, trends); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %s\n", err)
			return 1
		}
	} else if opts.jsonOutput {
		if err := output.PrintJSON(analysis); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			return 1
		}
	} else {
		output.PrintConsoleReport(analysis, opts.verbose)
	}

	// Markdown output
	if opts.mdOutput != "" {
		md := output.GenerateMarkdown(analysis)
		outPath := resolveOutputPath(opts.mdOutput, projectDir)
		if err := writeOutputFile(outPath, []byte(md)); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing markdown: %s\n", err)
		} else if !opts.jsonOutput {
			fmt.Println(color(fmt.Sprintf("  📄 Report saved to %s", opts.mdOutput), dim))
			fmt.Println()
		}
	}

	// SARIF output
	if opts.sarifFile != "" {
		sarifData, err := output.ToSARIF(analysis)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating SARIF: %s\n", err)
		} else {
			outPath := resolveOutputPath(opts.sarifFile, projectDir)
			if err := writeOutputFile(outPath, sarifData); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing SARIF: %s\n", err)
			} else if !opts.jsonOutput {
				fmt.Println(color(fmt.Sprintf("  📄 SARIF saved to %s", opts.sarifFile), dim))
				fmt.Println()
			}
		}
	}

	if analysis.Status == domain.GatePassed {
		return 0
	}
	return 1
}

func filterCheckToNewCode(check *domain.CheckResult, changedFileSet map[string]bool, changedLines []gitutil.ChangedLine) {
	if len(check.Issues) == 0 {
		return
	}

	filtered := make([]domain.Issue, 0, len(check.Issues))
	for _, issue := range check.Issues {
		if !changedFileSet[issue.File] {
			continue
		}
		if len(changedLines) > 0 && !gitutil.IsNewCodeLine(changedLines, issue.File, issue.Line) {
			continue
		}
		filtered = append(filtered, issue)
	}

	check.Issues = filtered
	recomputeIssueDrivenCheck(check)
}

func recomputeIssueDrivenCheck(check *domain.CheckResult) {
	switch check.ID {
	case "typescript":
		recomputeTypescriptCheck(check)
	case "secrets":
		recomputeSecretsCheck(check)
	case "semgrep":
		recomputeSemgrepCheck(check)
	case "custom-rules":
		recomputeCustomRulesCheck(check)
	}
}

func recomputeTypescriptCheck(check *domain.CheckResult) {
	prodErrors := 0
	testErrors := 0
	for _, issue := range check.Issues {
		if issue.Severity == domain.SeverityMinor {
			testErrors++
			continue
		}
		prodErrors++
	}
	totalErrors := prodErrors + testErrors
	ensureMetrics(check)
	check.Metrics["errors"] = float64(totalErrors)
	check.Metrics["prod_errors"] = float64(prodErrors)
	check.Metrics["test_errors"] = float64(testErrors)

	check.Summary = fmt.Sprintf("%d prod error", prodErrors)
	if prodErrors != 1 {
		check.Summary += "s"
	}
	if testErrors > 0 {
		check.Summary += fmt.Sprintf(", %d in tests", testErrors)
	}
	if totalErrors == 0 {
		check.Status = domain.StatusPassed
		check.Summary = "0 errors"
		return
	}
	if prodErrors == 0 {
		check.Status = domain.StatusWarning
		return
	}
	check.Status = domain.StatusFailed
}

func recomputeSecretsCheck(check *domain.CheckResult) {
	count := len(check.Issues)
	realSecrets := 0
	for _, issue := range check.Issues {
		if issue.Severity != domain.SeverityInfo {
			realSecrets++
		}
	}
	ensureMetrics(check)
	check.Metrics["secrets"] = float64(realSecrets)
	if count == 0 {
		check.Status = domain.StatusPassed
		check.Summary = "0 secrets detected"
		return
	}
	check.Status = domain.StatusFailed
	check.Summary = fmt.Sprintf("%d secret", count)
	if count != 1 {
		check.Summary += "s"
	}
	check.Summary += " detected"
}

func recomputeSemgrepCheck(check *domain.CheckResult) {
	vulns := 0
	hotspots := 0
	for _, issue := range check.Issues {
		if issue.Type == domain.IssueTypeVulnerability {
			vulns++
		} else if issue.Type == domain.IssueTypeSecurityHotspot {
			hotspots++
		}
	}
	ensureMetrics(check)
	check.Metrics["vulnerabilities"] = float64(vulns)
	check.Metrics["hotspots"] = float64(hotspots)
	if vulns == 0 && hotspots == 0 {
		check.Status = domain.StatusPassed
		check.Summary = "0 findings"
		return
	}
	parts := []string{}
	if vulns > 0 {
		label := fmt.Sprintf("%d vulnerability", vulns)
		if vulns != 1 {
			label = fmt.Sprintf("%d vulnerabilities", vulns)
		}
		parts = append(parts, label)
	}
	if hotspots > 0 {
		label := fmt.Sprintf("%d hotspot", hotspots)
		if hotspots != 1 {
			label += "s"
		}
		parts = append(parts, label)
	}
	check.Summary = strings.Join(parts, " · ")
	if vulns > 0 {
		check.Status = domain.StatusFailed
		return
	}
	check.Status = domain.StatusWarning
}

func recomputeCustomRulesCheck(check *domain.CheckResult) {
	blockingViolations := 0
	advisoryViolations := 0
	hasBlocker := false
	for _, issue := range check.Issues {
		if issue.Advisory {
			advisoryViolations++
			continue
		}
		blockingViolations++
		if issue.Severity == domain.SeverityBlocker || issue.Severity == domain.SeverityCritical {
			hasBlocker = true
		}
	}
	ensureMetrics(check)
	check.Metrics["violations"] = float64(len(check.Issues))
	check.Metrics["blocking_violations"] = float64(blockingViolations)
	check.Metrics["advisory_violations"] = float64(advisoryViolations)

	if blockingViolations == 0 {
		check.Status = domain.StatusPassed
		if advisoryViolations == 0 {
			check.Summary = "0 violations in new code"
			return
		}
		check.Summary = fmt.Sprintf("%d advisory-only violations in new code", advisoryViolations)
		return
	}
	check.Summary = fmt.Sprintf("%d blocking violations in new code", blockingViolations)
	if hasBlocker {
		check.Status = domain.StatusFailed
		return
	}
	check.Status = domain.StatusWarning
}

func ensureMetrics(check *domain.CheckResult) {
	if check.Metrics == nil {
		check.Metrics = map[string]float64{}
	}
}

func acquireRunLock(projectDir string) (func(), error) {
	lockDir := filepath.Join(projectDir, ".qualitygate")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	lockPath := filepath.Join(lockDir, "run.lock")
	for attempt := 0; attempt < 2; attempt++ {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = fmt.Fprintf(lockFile, "pid=%d\nstarted_at=%s\n", os.Getpid(), time.Now().Format(time.RFC3339))

			return func() {
				_ = lockFile.Close()
				_ = os.Remove(lockPath)
			}, nil
		}

		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create run lock: %w", err)
		}

		stale, staleErr := isRunLockStale(lockPath)
		if staleErr != nil {
			return nil, fmt.Errorf("inspect existing run lock: %w", staleErr)
		}
		if !stale {
			return nil, fmt.Errorf("another crivo run is already in progress for this project (%s). Wait for it to finish or remove %s if the lock is stale", projectDir, lockPath)
		}

		if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale run lock: %w", removeErr)
		}
	}

	return nil, fmt.Errorf("unable to acquire run lock for %s", projectDir)
}

func isRunLockStale(lockPath string) (bool, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}

	pid, startedAt := parseRunLockMetadata(string(data))
	if !startedAt.IsZero() && time.Since(startedAt) > 6*time.Hour {
		return true, nil
	}
	if pid <= 0 {
		return false, nil
	}

	running, err := processRunning(pid)
	if err != nil {
		return false, err
	}
	return !running, nil
}

func parseRunLockMetadata(contents string) (int, time.Time) {
	var pid int
	var startedAt time.Time

	for _, line := range strings.Split(contents, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "pid":
			parsedPID, err := strconv.Atoi(value)
			if err == nil {
				pid = parsedPID
			}
		case "started_at":
			parsedTime, err := time.Parse(time.RFC3339, value)
			if err == nil {
				startedAt = parsedTime
			}
		}
	}

	return pid, startedAt
}

func processRunning(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}

	if os.PathSeparator == '\\' {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid)).CombinedOutput()
		if err != nil {
			return false, err
		}
		return strings.Contains(string(out), strconv.Itoa(pid)), nil
	}

	err := exec.Command("kill", "-0", strconv.Itoa(pid)).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func runTrends() {
	projectDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	s, err := store.Open(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No history found. Run with --save first.\n")
		os.Exit(1)
	}
	defer s.Close()

	points, err := s.GetTrend(projectDir, 20)
	if err != nil || len(points) == 0 {
		fmt.Println("  No trend data available yet. Run `crivo run --save` a few times first.")
		return
	}

	fmt.Println()
	fmt.Println(color("  📈 Quality Trends", cyan, bold))
	fmt.Println()

	// Issues sparkline
	issuesSpark := store.Sparkline(points, func(p store.TrendPoint) float64 {
		return float64(p.TotalIssues)
	})
	fmt.Printf("  Issues:      %s  (%d → %d)\n", issuesSpark, points[0].TotalIssues, points[len(points)-1].TotalIssues)

	// Coverage sparkline
	covSpark := store.Sparkline(points, func(p store.TrendPoint) float64 {
		return p.Coverage
	})
	fmt.Printf("  Coverage:    %s  (%.1f%% → %.1f%%)\n", covSpark, points[0].Coverage, points[len(points)-1].Coverage)

	// Duplication sparkline
	dupSpark := store.Sparkline(points, func(p store.TrendPoint) float64 {
		return p.Duplication
	})
	fmt.Printf("  Duplication: %s  (%.1f%% → %.1f%%)\n", dupSpark, points[0].Duplication, points[len(points)-1].Duplication)

	fmt.Println()
	fmt.Printf("  %d data points from %s to %s\n",
		len(points),
		points[0].Date.Format("2006-01-02"),
		points[len(points)-1].Date.Format("2006-01-02"),
	)
	fmt.Println()
}

func runInit() {
	projectDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println(color("  🚀 Initializing Quality Gate", cyan, bold))
	fmt.Println()
	fmt.Println(color("  🔍 Detecting project type...", cyan))

	// Create .qualitygate.yaml
	configPath := filepath.Join(projectDir, ".qualitygate.yaml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println(color("  ⏭️  .qualitygate.yaml already exists, skipping", yellow))
	} else {
		data, err := config.GenerateDetected(projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		summary := config.BuildDetectionSummary(projectDir)
		fmt.Println(color(fmt.Sprintf("  ✅ Created .qualitygate.yaml (detected: %s)", summary), green))
	}

	// Create GitHub Actions workflow
	workflowDir := filepath.Join(projectDir, ".github", "workflows")
	workflowPath := filepath.Join(workflowDir, "quality-gate.yml")

	if _, err := os.Stat(workflowPath); err == nil {
		fmt.Println(color("  ⏭️  GitHub Actions workflow already exists, skipping", yellow))
	} else {
		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create workflow dir: %s\n", err)
		} else {
			workflow := `name: Quality Gate

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

permissions:
  contents: read
  pull-requests: write

jobs:
  quality-gate:
    name: Quality Gate
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
      - run: npm ci
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install quality-gate
        run: go install github.com/guilherme11gr/crivo/cmd/crivo@latest
      - name: Run Quality Gate
        run: crivo run --md quality-gate-report.md --sarif quality-gate.sarif --save
      - name: Upload SARIF
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: quality-gate.sarif
      - name: Comment PR
        if: github.event_name == 'pull_request' && always()
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          path: quality-gate-report.md
`
			if err := os.WriteFile(workflowPath, []byte(workflow), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not create workflow: %s\n", err)
			} else {
				fmt.Println(color("  ✅ Created .github/workflows/quality-gate.yml", green))
			}
		}
	}

	// Install Claude Code skills
	installSkills(projectDir)

	fmt.Println()
	fmt.Println(color("  Done! Next steps:", bold))
	fmt.Println(color("  1. Review .qualitygate.yaml and adjust thresholds", dim))
	fmt.Println(color("  2. Run: crivo run", dim))
	fmt.Println(color("  3. Commit the config and workflow files", dim))
	fmt.Println()
}

func installSkills(projectDir string) {
	skillsRoot := ".claude/skills"
	embeddedRoot := ".claude/skills"

	installed := 0
	err := fs.WalkDir(qualitygate.SkillsFS, embeddedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		// path is like ".claude/skills/ci/SKILL.md"
		// We want to write to <projectDir>/.claude/skills/ci/SKILL.md
		destPath := filepath.Join(projectDir, filepath.FromSlash(path))

		if _, err := os.Stat(destPath); err == nil {
			return nil // already exists, skip
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		data, err := qualitygate.SkillsFS.ReadFile(path)
		if err != nil {
			return err
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return err
		}
		installed++
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not install skills: %s\n", err)
	} else if installed > 0 {
		fmt.Printf("  %s Installed %d skill files to %s/\n", color("✅", green), installed, skillsRoot)
	} else {
		fmt.Println(color("  ⏭️  Claude Code skills already installed, skipping", yellow))
	}
}

// color helpers
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
)

func color(text string, codes ...string) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	result := ""
	for _, c := range codes {
		result += c
	}
	return result + text + reset
}

// applyBaselineComparison compares current metrics against the last saved run.
// It annotates check results with regression info and downgrades coverage/complexity
// from "failed" to "warning" when values haven't regressed (legacy debt tolerance).
func applyBaselineComparison(analysis *domain.AnalysisResult, projectDir string, opts options) {
	s, err := store.Open(projectDir)
	if err != nil {
		return
	}
	defer s.Close()

	baseline, err := s.GetLastMetrics(projectDir)
	if err != nil || len(baseline) == 0 {
		return // No baseline yet — first run
	}

	for i := range analysis.Checks {
		check := &analysis.Checks[i]
		if check.Metrics == nil {
			check.Metrics = map[string]float64{}
		}

		switch check.ID {
		case "coverage":
			prevLines := baseline["coverage_lines"]
			currLines := check.Metrics["lines"]

			if prevLines > 0 {
				delta := currLines - prevLines
				check.Metrics["baseline_lines"] = prevLines
				check.Metrics["delta_lines"] = delta

				if delta < -1.0 {
					// Coverage dropped by more than 1% — regression
					check.Details = append(check.Details, "",
						fmt.Sprintf("REGRESSION: coverage dropped %.1f%% → %.1f%% (Δ%.1f%%)", prevLines, currLines, delta))
				} else if check.Status == domain.StatusFailed {
					// Coverage didn't regress — downgrade from failed to warning (legacy debt)
					check.Status = domain.StatusWarning
					check.Details = append(check.Details, "",
						fmt.Sprintf("Baseline: %.1f%% → %.1f%% (no regression, legacy debt tolerated)", prevLines, currLines))
				}
			}

		case "complexity":
			prevViolations := baseline["complexity_violations"]
			currViolations := check.Metrics["violations"]

			if prevViolations >= 0 {
				delta := currViolations - prevViolations
				check.Metrics["baseline_violations"] = prevViolations
				check.Metrics["delta_violations"] = delta

				if delta > 0 {
					check.Details = append(check.Details, "",
						fmt.Sprintf("REGRESSION: +%.0f new complexity violations vs baseline", delta))
				} else if check.Status == domain.StatusFailed && delta <= 0 {
					check.Status = domain.StatusWarning
					check.Details = append(check.Details, "",
						fmt.Sprintf("Baseline: %.0f → %.0f violations (no regression)", prevViolations, currViolations))
				}
			}
		}
	}

	if !opts.jsonOutput && !opts.tuiMode {
		fmt.Println(color("  📊 Baseline comparison active (vs last --save)", dim))
	}
}

// resolveOutputPath converts a user-provided output path to an absolute path.
// Handles Unix-style paths on Windows (e.g., /tmp/foo from WSL scripts).
func resolveOutputPath(userPath, projectDir string) string {
	if filepath.IsAbs(userPath) {
		return filepath.FromSlash(userPath)
	}
	return filepath.Join(projectDir, filepath.FromSlash(userPath))
}

// writeOutputFile writes data to a file, creating parent directories if needed.
func writeOutputFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	return os.WriteFile(path, data, 0644)
}

func statusIcon(s domain.CheckStatus) string {
	switch s {
	case domain.StatusPassed:
		return "✅"
	case domain.StatusFailed:
		return "❌"
	case domain.StatusWarning:
		return "⚠️"
	case domain.StatusSkipped:
		return "⏭️"
	case domain.StatusError:
		return "💥"
	default:
		return "?"
	}
}
