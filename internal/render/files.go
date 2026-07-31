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

	if writeDiffBody(&out, diff.Content) == 0 {
		out.WriteString("No textual changes.\n")
	}

	return out.String()
}

// writeDiffBody appends the diff regions and reports how many lines it wrote.
func writeDiffBody(out *strings.Builder, content []gerrit.DiffContent) int {
	written := 0

	for i := range content {
		region := &content[i]

		if region.Skip > 0 {
			out.WriteString("@@ ")
			out.WriteString(strconv.Itoa(region.Skip))
			out.WriteString(" lines skipped @@\n")

			written++
		}

		for _, group := range []struct {
			marker string
			lines  []string
		}{
			{marker: "- ", lines: region.A},
			{marker: "+ ", lines: region.B},
			{marker: "  ", lines: region.AB},
		} {
			for _, line := range group.lines {
				if written >= MaxDiffLines {
					out.WriteString("... diff truncated after ")
					out.WriteString(strconv.Itoa(MaxDiffLines))
					out.WriteString(" lines ...\n")

					return written
				}

				out.WriteString(group.marker)
				out.WriteString(line)
				out.WriteString("\n")

				written++
			}
		}
	}

	return written
}
