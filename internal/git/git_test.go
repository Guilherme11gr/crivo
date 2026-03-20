package git

import (
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
