package render

import (
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// ChangeDetail renders one change with its review state.
//
// Each aspect gets a single line. A model asking for detail usually wants to
// know who has voted and what is unresolved, not to read a JSON document.
func ChangeDetail(_ *gerrit.ChangeDetail) string {
	return ""
}
