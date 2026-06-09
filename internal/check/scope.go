package check

import (
	"context"

	gitutil "github.com/guilherme11gr/crivo/internal/git"
)

type newCodeScopeKey struct{}

// NewCodeScope carries changed files/lines so providers can optimize work in --new-code mode.
type NewCodeScope struct {
	ChangedFiles   []string
	ChangedLines   []gitutil.ChangedLine
	changedFileSet map[string]bool
}

func NewScope(changedFiles []gitutil.ChangedFile, changedLines []gitutil.ChangedLine) NewCodeScope {
	paths := make([]string, 0, len(changedFiles))
	fileSet := make(map[string]bool, len(changedFiles))
	for _, file := range changedFiles {
		paths = append(paths, file.Path)
		fileSet[file.Path] = true
	}
	return NewCodeScope{
		ChangedFiles:   paths,
		ChangedLines:   changedLines,
		changedFileSet: fileSet,
	}
}

func WithNewCodeScope(ctx context.Context, scope NewCodeScope) context.Context {
	return context.WithValue(ctx, newCodeScopeKey{}, scope)
}

func NewCodeScopeFromContext(ctx context.Context) (NewCodeScope, bool) {
	scope, ok := ctx.Value(newCodeScopeKey{}).(NewCodeScope)
	return scope, ok
}

func (s NewCodeScope) HasFile(path string) bool {
	return s.changedFileSet[path]
}
