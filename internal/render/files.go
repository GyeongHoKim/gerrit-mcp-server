package render

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// statusColumn is wide enough for the longest counts we print before the path
// column starts, so the listing stays scannable.
const statusColumn = 10

// Files renders the file list of a patch set, one line per file.
//
// Paths are sorted: Gerrit returns a map, and Go randomises map iteration, so
// unsorted output would differ between two calls on identical input.
func Files(files map[string]gerrit.FileInfo) string {
	if len(files) == 0 {
		return "No files.\n"
	}

	var out strings.Builder

	if len(files) == 1 {
		out.WriteString("1 file.\n")
	} else {
		out.WriteString(strconv.Itoa(len(files)))
		out.WriteString(" files.\n")
	}

	out.WriteString("\n")

	for _, path := range slices.Sorted(maps.Keys(files)) {
		writeFile(&out, path, files[path])
	}

	return out.String()
}

// writeFile appends one file line.
func writeFile(out *strings.Builder, path string, file gerrit.FileInfo) {
	status := file.Status
	if status == "" {
		// Gerrit omits the status for a plain modification rather than
		// spelling it out.
		status = "M"
	}

	counts := "binary"
	if !file.Binary {
		counts = "+" + strconv.Itoa(file.LinesInserted) + "/-" + strconv.Itoa(file.LinesDeleted)
	}

	out.WriteString("  ")
	out.WriteString(status)
	out.WriteString("  ")
	out.WriteString(counts)
	out.WriteString(strings.Repeat(" ", max(1, statusColumn-len(counts))))
	out.WriteString(path)

	if file.OldPath != "" {
		out.WriteString(" (was ")
		out.WriteString(file.OldPath)
		out.WriteString(")")
	}

	out.WriteString("\n")
}

// MaxDiffLines caps the diff body. A generated file can produce a diff larger
// than any context window, and a truncated diff the model knows is truncated
// beats one that silently exhausts the budget.
//
// Exported so a test can assert the exact cap rather than a looser bound that
// a regression could slip under.
const MaxDiffLines = 1500

// Diff renders a file diff in unified form.
func Diff(path string, diff *gerrit.DiffInfo) string {
	var out strings.Builder

	out.WriteString(path)
	out.WriteString(" (")
	out.WriteString(diff.ChangeType)
	out.WriteString(")\n\n")

	if diff.Binary {
		out.WriteString("Binary file, no textual diff.\n")

		return out.String()
	}

	// The legend costs one line and saves the model from guessing which side
	// a number refers to.
	legend := "Line numbers: '-' rows are the old file, others the new.\n\n"
	out.WriteString(legend)

	if writeDiffBody(&out, diff.Content) == 0 {
		// Nothing was written, so the legend describes nothing.
		body := out.String()
		out.Reset()
		out.WriteString(strings.TrimSuffix(body, legend))
		out.WriteString("No textual changes.\n")
	}

	return out.String()
}

// diffSide says which file a run of lines belongs to.
type diffSide int

const (
	// bothSides is context, present in the old file and the new one.
	bothSides diffSide = iota
	// oldSide is a removed line, numbered against the old file.
	oldSide
	// newSide is an added line, numbered against the new file.
	newSide
)

// diffGroup is one run of lines sharing a marker and a side.
type diffGroup struct {
	marker string
	lines  []string
	side   diffSide
}

// diffCursor tracks the current line on each side of the diff.
type diffCursor struct {
	old     int
	next    int
	written int
}

// writeDiffBody appends the diff regions and reports how many lines it wrote.
func writeDiffBody(out *strings.Builder, content []gerrit.DiffContent) int {
	cursor := diffCursor{old: 1, next: 1}

	for i := range content {
		region := &content[i]

		if region.Skip > 0 {
			out.WriteString("@@ ")
			out.WriteString(strconv.Itoa(region.Skip))
			out.WriteString(" lines skipped @@\n")

			// A skip elides the same run from both sides at once. Advancing
			// only one would shift every number after it.
			cursor.old += region.Skip
			cursor.next += region.Skip
			cursor.written++
		}

		groups := []diffGroup{
			{lines: region.A, marker: "- ", side: oldSide},
			{lines: region.B, marker: "+ ", side: newSide},
			{lines: region.AB, marker: "  ", side: bothSides},
		}

		for _, group := range groups {
			if writeDiffGroup(out, &cursor, group) {
				return cursor.written
			}
		}
	}

	return cursor.written
}

// writeDiffGroup appends one run of lines, reporting whether the cap was hit.
func writeDiffGroup(out *strings.Builder, cursor *diffCursor, group diffGroup) bool {
	for _, line := range group.lines {
		if cursor.written >= MaxDiffLines {
			out.WriteString("... diff truncated after ")
			out.WriteString(strconv.Itoa(MaxDiffLines))
			out.WriteString(" lines ...\n")

			return true
		}

		number := cursor.next
		if group.side == oldSide {
			number = cursor.old
		}

		writeNumberedLine(out, number, group.marker, line)

		switch group.side {
		case oldSide:
			cursor.old++
		case newSide:
			cursor.next++
		default:
			cursor.old++
			cursor.next++
		}

		cursor.written++
	}

	return false
}

// writeNumberedLine appends one diff row in a fixed-width numbered column.
func writeNumberedLine(out *strings.Builder, number int, marker, line string) {
	const numberWidth = 4

	text := strconv.Itoa(number)
	if pad := numberWidth - len(text); pad > 0 {
		out.WriteString(strings.Repeat(" ", pad))
	}

	out.WriteString(text)
	out.WriteString(" ")
	out.WriteString(marker)
	out.WriteString(line)
	out.WriteString("\n")
}
