package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsNewCodeLine(t *testing.T) {
	lines := []ChangedLine{
		{File: "src/foo.ts", StartLine: 10, EndLine: 20},
		{File: "src/bar.ts", StartLine: 5, EndLine: 5},
	}

	tests := []struct {
		file string
		line int
		want bool
	}{
		{"src/foo.ts", 10, true},
		{"src/foo.ts", 15, true},
		{"src/foo.ts", 20, true},
		{"src/foo.ts", 9, false},
		{"src/foo.ts", 21, false},
		{"src/bar.ts", 5, true},
		{"src/bar.ts", 6, false},
		{"src/baz.ts", 10, false},
	}

	for _, tt := range tests {
		got := IsNewCodeLine(lines, tt.file, tt.line)
		if got != tt.want {
			t.Errorf("IsNewCodeLine(%s:%d) = %v, want %v", tt.file, tt.line, got, tt.want)
		}
	}
}

// gitRun runs a git command in dir and returns trimmed stdout. It fails the
// test on error so fixtures stay deterministic.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// initTestRepo creates a temp repo with a single commit on the given branch.
func initTestRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial commit")
	return dir
}

func TestResolveBaseRef_LocalBaseExists(t *testing.T) {
	dir := initTestRepo(t, "main")

	ref, err := ResolveBaseRef(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("ResolveBaseRef() error = %v", err)
	}
	if ref != "main" {
		t.Fatalf("ResolveBaseRef() = %q, want %q", ref, "main")
	}
}

func TestResolveBaseRef_FallsBackToOrigin(t *testing.T) {
	dir := initTestRepo(t, "main")
	sha := gitRun(t, dir, "rev-parse", "HEAD")
	// CI-like state: only refs/remotes/origin/main exists, local main deleted.
	gitRun(t, dir, "update-ref", "refs/remotes/origin/main", sha)
	gitRun(t, dir, "update-ref", "-d", "refs/heads/main")

	ref, err := ResolveBaseRef(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("ResolveBaseRef() error = %v", err)
	}
	if ref != "origin/main" {
		t.Fatalf("ResolveBaseRef() = %q, want %q", ref, "origin/main")
	}
}

func TestResolveBaseRef_NeitherExists(t *testing.T) {
	dir := initTestRepo(t, "trunk") // no main, no origin/main

	_, err := ResolveBaseRef(context.Background(), dir, "main")
	if err == nil {
		t.Fatal("ResolveBaseRef() expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'main'") || !strings.Contains(msg, "'origin/main'") {
		t.Fatalf("error %q does not cite both tried refs", msg)
	}
}

func TestResolveBaseRef_EmptyBase(t *testing.T) {
	if _, err := ResolveBaseRef(context.Background(), t.TempDir(), ""); err == nil {
		t.Fatal("ResolveBaseRef() with empty base expected error, got nil")
	}
}

func TestDefaultBranch_TrunkViaOriginHead(t *testing.T) {
	dir := initTestRepo(t, "trunk")
	sha := gitRun(t, dir, "rev-parse", "HEAD")
	// Simulate a clone whose remote default is trunk.
	gitRun(t, dir, "update-ref", "refs/remotes/origin/trunk", sha)
	gitRun(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")

	branch, err := DefaultBranch(context.Background(), dir)
	if err != nil {
		t.Fatalf("DefaultBranch() error = %v", err)
	}
	if branch != "trunk" {
		t.Fatalf("DefaultBranch() = %q, want %q", branch, "trunk")
	}
}

func TestDefaultBranch_MasterFallback(t *testing.T) {
	dir := initTestRepo(t, "master")
	sha := gitRun(t, dir, "rev-parse", "HEAD")
	// No origin/HEAD and no local main: falls back to verified origin/master.
	gitRun(t, dir, "update-ref", "refs/remotes/origin/master", sha)
	gitRun(t, dir, "update-ref", "-d", "refs/heads/master")

	branch, err := DefaultBranch(context.Background(), dir)
	if err != nil {
		t.Fatalf("DefaultBranch() error = %v", err)
	}
	if branch != "master" {
		t.Fatalf("DefaultBranch() = %q, want %q", branch, "master")
	}
}

func TestDefaultBranch_NoVerifiableDefault(t *testing.T) {
	dir := initTestRepo(t, "trunk") // no origin/HEAD, no main, no origin/master

	if _, err := DefaultBranch(context.Background(), dir); err == nil {
		t.Fatal("DefaultBranch() expected error, got nil")
	}
}

func TestCurrentBranchAndCommit_DetachedHead(t *testing.T) {
	dir := initTestRepo(t, "main")
	gitRun(t, dir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "feature commit")
	sha := gitRun(t, dir, "rev-parse", "HEAD")
	gitRun(t, dir, "checkout", "--detach", sha)
	// CI-like: delete local main, keep only origin/main.
	gitRun(t, dir, "update-ref", "refs/remotes/origin/main", sha)
	gitRun(t, dir, "update-ref", "-d", "refs/heads/main")

	ctx := context.Background()
	branch, err := CurrentBranch(ctx, dir)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "HEAD" {
		t.Fatalf("CurrentBranch() = %q, want %q (detached)", branch, "HEAD")
	}

	commit, err := CurrentCommit(ctx, dir)
	if err != nil {
		t.Fatalf("CurrentCommit() error = %v", err)
	}
	if commit != sha {
		t.Fatalf("CurrentCommit() = %q, want %q", commit, sha)
	}

	ref, err := ResolveBaseRef(ctx, dir, "main")
	if err != nil {
		t.Fatalf("ResolveBaseRef() error = %v", err)
	}
	if ref != "origin/main" {
		t.Fatalf("ResolveBaseRef() = %q, want %q", ref, "origin/main")
	}
}

func TestCurrentCommit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := CurrentCommit(context.Background(), dir); err == nil {
		t.Fatal("CurrentCommit() in non-git dir expected error, got nil")
	}
}
