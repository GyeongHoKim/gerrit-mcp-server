package render

import (
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// Files renders the file list of a patch set, one line per file.
func Files(_ map[string]gerrit.FileInfo) string {
	return ""
}
