package render_test

import (
	"strings"
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

func TestDiff(t *testing.T) {
	t.Parallel()

	tests := map[string]*gerrit.DiffInfo{
		"diff_modified": {
			ChangeType: "MODIFIED",
			MetaA:      &gerrit.DiffFileMeta{Name: "src/widget.go", Lines: 120},
			MetaB:      &gerrit.DiffFileMeta{Name: "src/widget.go", Lines: 159},
			Content: []gerrit.DiffContent{
				{AB: []string{"package widget", ""}},
				{A: []string{"func old() {}"}, B: []string{"func new() {}", "func extra() {}"}},
				{Skip: 40},
				{B: []string{"// trailing addition"}},
			},
		},
		"diff_added": {
			ChangeType: "ADDED",
			MetaB:      &gerrit.DiffFileMeta{Name: "src/new.go", Lines: 2},
			Content:    []gerrit.DiffContent{{B: []string{"package new", ""}}},
		},
		"diff_binary": {
			ChangeType: "ADDED",
			MetaB:      &gerrit.DiffFileMeta{Name: "assets/logo.png"},
			Binary:     true,
		},
		"diff_empty": {
			ChangeType: "MODIFIED",
			MetaA:      &gerrit.DiffFileMeta{Name: "src/untouched.go", Lines: 3},
			MetaB:      &gerrit.DiffFileMeta{Name: "src/untouched.go", Lines: 3},
		},
	}

	for name, diff := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.Diff("src/widget.go", diff))
		})
	}
}

func TestDiffNumbersLines(t *testing.T) {
	t.Parallel()

	// Without line numbers the model can quote a diff but cannot say where the
	// problem is, and post_review_comment needs a line to attach to.
	got := render.Diff("src/widget.go", &gerrit.DiffInfo{
		ChangeType: "MODIFIED",
		MetaA:      &gerrit.DiffFileMeta{Name: "src/widget.go", Lines: 120},
		MetaB:      &gerrit.DiffFileMeta{Name: "src/widget.go", Lines: 159},
		Content: []gerrit.DiffContent{
			{AB: []string{"package widget", ""}},
			{A: []string{"func old() {}"}, B: []string{"func new() {}", "func extra() {}"}},
			// A skipped region advances both sides, or every number after it
			// is wrong.
			{Skip: 40},
			{B: []string{"// trailing addition"}},
		},
	})

	for _, want := range []string{
		"   1   package widget",
		"   3 - func old() {}",
		"   3 + func new() {}",
		"   4 + func extra() {}",
		"  45 + // trailing addition",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
}

func TestDiffTruncatesEnormousDiffs(t *testing.T) {
	t.Parallel()

	// A generated file can produce a diff far larger than any context window.
	// The renderer has to cut it off and say so.
	added := make([]string, 0, 50_000)
	for range 50_000 {
		added = append(added, "a line of a machine-generated file")
	}

	got := render.Diff("generated.go", &gerrit.DiffInfo{
		ChangeType: "ADDED",
		MetaB:      &gerrit.DiffFileMeta{Name: "generated.go", Lines: len(added)},
		Content:    []gerrit.DiffContent{{B: added}},
	})

	// Asserting a bound looser than the cap would let a regression that
	// raised the cap, or an off-by-one past it, go unnoticed. The output is
	// the capped body plus the header, the blank line and the notice.
	const overhead = 5

	if lines := strings.Count(got, "\n"); lines > render.MaxDiffLines+overhead {
		t.Errorf("rendered %d lines, want at most %d", lines, render.MaxDiffLines+overhead)
	}

	if !strings.Contains(got, "truncated") {
		t.Errorf("output was cut but does not say so:\n%s", got[max(0, len(got)-300):])
	}
}
