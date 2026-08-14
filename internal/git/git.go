package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ChangedFile represents a file modified in the diff
type ChangedFile struct {
	Path      string
	Status    string // "A" added, "M" modified, "D" deleted, "R" renamed
	Additions int
	Deletions int
}

// ChangedLine represents a specific changed line range
type ChangedLine struct {
	File      string
	StartLine int
	EndLine   int
}

// IsGitRepo returns true if projectDir is inside a git repository
func IsGitRepo(projectDir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// CurrentBranch returns the current branch name
func CurrentBranch(ctx context.Context, projectDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DefaultBranch returns the default branch (main or master), verifying any
// name before returning it. It never returns an unverifiable ref: when no
// default can be resolved, it returns an error instead of guessing.
func DefaultBranch(ctx context.Context, projectDir string) (string, error) {
	// Try to detect from remote HEAD
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = projectDir
	if out, err := cmd.Output(); err == nil {
		if name := branchFromRef(strings.TrimSpace(string(out))); name != "" && name != "HEAD" {
			return name, nil
		}
	}

	// Fallback: check if main exists locally
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--verify", "main")
	cmd.Dir = projectDir
	if err := cmd.Run(); err == nil {
		return "main", nil
	}

	// Never return an unverified name: only return "master" if it resolves.
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--verify", "origin/master")
	cmd.Dir = projectDir
	if err := cmd.Run(); err == nil {
		return "master", nil
	}

	// Last resort: resolve origin/HEAD directly (some clones set the ref
	// without a symbolic link).
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--verify", "refs/remotes/origin/HEAD")
	cmd.Dir = projectDir
	if out, err := cmd.Output(); err == nil {
		if name := branchFromRef(strings.TrimSpace(string(out))); name != "" && name != "HEAD" {
			return name, nil
		}
	}

	return "", errors.New("cannot determine default branch (no origin/HEAD, main, or origin/master found)")
}

// ResolveBaseRef resolves the base ref name to a ref that verifiably exists,
// trying <base> first and falling back to origin/<base>. This mirrors CI
// checkouts (pull_request) where HEAD is detached and only remote refs exist,
// so "main" must resolve to "origin/main". When neither ref exists it returns
// an error citing both candidates and the truncated git stderr.
func ResolveBaseRef(ctx context.Context, projectDir, base string) (string, error) {
	if base == "" {
		return "", errors.New("empty base ref")
	}

	var lastErr string
	for _, ref := range []string{base, "origin/" + base} {
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", ref)
		cmd.Dir = projectDir
		out, err := cmd.CombinedOutput()
		if err == nil {
			return ref, nil
		}
		lastErr = truncateStderr(string(out))
	}

	return "", fmt.Errorf("cannot resolve git refs '%s' and 'origin/%s': %s", base, base, lastErr)
}

// CurrentCommit returns the full SHA of HEAD, or an error when not in a git
// repo.
func CurrentCommit(ctx context.Context, projectDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// branchFromRef extracts the branch name from a ref path like
// "refs/remotes/origin/main".
func branchFromRef(ref string) string {
	parts := strings.Split(ref, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// truncateStderr caps git stderr output at ~200 chars so error messages stay
// readable and do not leak full command output.
func truncateStderr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// GetChangedFiles returns files changed between base and head
func GetChangedFiles(ctx context.Context, projectDir, base, head string) ([]ChangedFile, error) {
	var args []string
	if head == "" {
		// Compare base vs working tree
		args = []string{"diff", "--numstat", "--diff-filter=ACMR", base}
	} else {
		args = []string{"diff", "--numstat", "--diff-filter=ACMR", base + "..." + head}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = projectDir

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []ChangedFile
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		additions, _ := strconv.Atoi(parts[0])
		deletions, _ := strconv.Atoi(parts[1])
		path := parts[2]

		files = append(files, ChangedFile{
			Path:      filepath.ToSlash(path),
			Status:    "M",
			Additions: additions,
			Deletions: deletions,
		})
	}

	return files, nil
}

var hunkRe = regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)

// GetChangedLines returns the specific line ranges that were added/modified
func GetChangedLines(ctx context.Context, projectDir, base, head string) ([]ChangedLine, error) {
	var args []string
	if head == "" {
		args = []string{"diff", "-U0", "--diff-filter=ACMR", base}
	} else {
		args = []string{"diff", "-U0", "--diff-filter=ACMR", base + "..." + head}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = projectDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var lines []ChangedLine
	currentFile := ""

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = filepath.ToSlash(line[6:])
			continue
		}

		if strings.HasPrefix(line, "@@") && currentFile != "" {
			matches := hunkRe.FindStringSubmatch(line)
			if matches == nil {
				continue
			}

			startLine, _ := strconv.Atoi(matches[1])
			count := 1
			if matches[2] != "" {
				count, _ = strconv.Atoi(matches[2])
			}

			if count == 0 {
				continue
			}

			lines = append(lines, ChangedLine{
				File:      currentFile,
				StartLine: startLine,
				EndLine:   startLine + count - 1,
			})
		}
	}

	return lines, nil
}

// IsNewCodeLine returns true if the given file:line is in the changed lines set
func IsNewCodeLine(changedLines []ChangedLine, file string, line int) bool {
	for _, cl := range changedLines {
		if cl.File == file && line >= cl.StartLine && line <= cl.EndLine {
			return true
		}
	}
	return false
}

// MergeBase returns the merge base commit between two refs
func MergeBase(ctx context.Context, projectDir, ref1, ref2 string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "merge-base", ref1, ref2)
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
