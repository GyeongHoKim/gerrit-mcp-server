package render_test

import (
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

func TestFiles(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]gerrit.FileInfo{
		"files_empty": {},
		"files_mixed": {
			// Deliberately unsorted, and Gerrit's pseudo-entry first.
			"src/widget.go":   {LinesInserted: 42, LinesDeleted: 3, SizeDelta: 900, Size: 4200},
			"/COMMIT_MSG":     {Status: "A", LinesInserted: 9, SizeDelta: 320, Size: 320},
			"assets/logo.png": {Status: "A", Binary: true, SizeDelta: 2048, Size: 2048},
			"src/legacy.go":   {Status: "D", LinesDeleted: 18, SizeDelta: -600},
			"src/new.go":      {Status: "R", OldPath: "src/old.go", LinesInserted: 2, LinesDeleted: 2},
		},
	}

	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.Files(files))
		})
	}
}

func TestFilesSortsPathsDeterministically(t *testing.T) {
	t.Parallel()

	files := map[string]gerrit.FileInfo{
		"z.go": {}, "a.go": {}, "m.go": {}, "/COMMIT_MSG": {},
	}

	first := render.Files(files)
	for range 20 {
		if got := render.Files(files); got != first {
			t.Fatalf("output is not stable across calls:\n--- first ---\n%s\n--- later ---\n%s", first, got)
		}
	}
}
