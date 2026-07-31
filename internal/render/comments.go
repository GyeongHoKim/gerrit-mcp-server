package render

import (
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// Comments renders published review comments grouped by file.
func Comments(_ map[string][]gerrit.CommentInfo) string {
	return ""
}

// Drafts renders unpublished draft comments grouped by file.
func Drafts(_ map[string][]gerrit.CommentInfo) string {
	return ""
}
