package customrules

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	defaultFileGlob = "src/**/*.{ts,tsx,js,jsx}"
	maxFileSize     = 1 << 20 // 1MB
	maxDepth        = 50      // prevent symlink loops
)

var defaultExcludeDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	".next":        true,
	"coverage":     true,
	".git":         true,
	"vendor":       true,
	"build":        true,
}

// WalkFiles returns files matching the glob pattern under projectDir,
// skipping excluded directories and binary/large files.
// Uses WalkDir for performance (avoids os.Stat per entry).
func WalkFiles(ctx context.Context, projectDir string, fileGlob string, exclude []string) ([]string, error) {
	if fileGlob == "" {
		fileGlob = defaultFileGlob
	}

	excludeSet := make(map[string]bool, len(defaultExcludeDirs)+len(exclude))
	for k, v := range defaultExcludeDirs {
		excludeSet[k] = v
	}
	for _, e := range exclude {
		excludeSet[strings.TrimSuffix(e, "/")] = true
	}

	var matches []string

	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}

		// Check context cancellation periodically
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, _ := filepath.Rel(projectDir, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			base := filepath.Base(path)
			if excludeSet[base] {
				return filepath.SkipDir
			}
			// Depth guard against symlink loops
			if strings.Count(rel, "/") > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks to avoid loops
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		// Skip large files (need FileInfo for size)
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > maxFileSize {
			return nil
		}

		if matchGlob(fileGlob, rel) {
			matches = append(matches, rel)
		}
		return nil
	})

	return matches, err
}

// IsAllowedIn checks if a file path matches any of the allow-in globs.
func IsAllowedIn(filePath string, allowIn []string) bool {
	for _, pattern := range allowIn {
		if matchGlob(pattern, filePath) {
			return true
		}
	}
	return false
}

// IsTextFile checks if file content is valid UTF-8 text by sampling the first bytes.
func IsTextFile(data []byte) bool {
	// Check first 512 bytes for null bytes or invalid UTF-8
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	if !utf8.Valid(sample) {
		return false
	}
	for _, b := range sample {
		if b == 0 {
			return false
		}
	}
	return true
}

// matchGlob matches a path against a glob pattern supporting:
//   - * matches any non-separator characters
//   - ** matches any path segments (zero or more directories)
//   - ? matches a single non-separator character
//   - {a,b} alternation
func matchGlob(pattern, path string) bool {
	// Expand {a,b} alternation first
	if idx := strings.IndexByte(pattern, '{'); idx >= 0 {
		end := strings.IndexByte(pattern[idx:], '}')
		if end > 0 {
			prefix := pattern[:idx]
			suffix := pattern[idx+end+1:]
			alts := strings.Split(pattern[idx+1:idx+end], ",")
			for _, alt := range alts {
				if matchGlob(prefix+alt+suffix, path) {
					return true
				}
			}
			return false
		}
	}

	return matchGlobSimple(pattern, path)
}

func matchGlobSimple(pattern, path string) bool {
	for len(pattern) > 0 {
		switch {
		case strings.HasPrefix(pattern, "**/"): // ** at start/middle
			pattern = pattern[3:]
			// Try matching ** against zero or more path segments
			for {
				if matchGlobSimple(pattern, path) {
					return true
				}
				slash := strings.IndexByte(path, '/')
				if slash < 0 {
					return false
				}
				path = path[slash+1:]
			}

		case pattern == "**": // ** at end
			return true

		case len(path) == 0:
			return len(pattern) == 0

		case pattern[0] == '*':
			pattern = pattern[1:]
			// * matches any non-separator characters
			for i := 0; i <= len(path); i++ {
				if i > 0 && path[i-1] == '/' {
					break
				}
				if matchGlobSimple(pattern, path[i:]) {
					return true
				}
			}
			return false

		case pattern[0] == '?':
			if path[0] == '/' {
				return false
			}
			pattern = pattern[1:]
			path = path[1:]

		case pattern[0] == path[0]:
			pattern = pattern[1:]
			path = path[1:]

		default:
			return false
		}
	}

	return len(path) == 0
}
