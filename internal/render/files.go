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
