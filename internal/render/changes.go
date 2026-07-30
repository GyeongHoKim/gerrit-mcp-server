// Package render turns Gerrit types into the compact text a model reads.
//
// Gerrit's JSON is far larger than anything the model needs. Everything the
// tools return passes through here, which keeps responses inside a sensible
// token budget and keeps the output readable to a person debugging a session.
package render

import (
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// Changes renders a change list, one block per change.
//
// The first line carries the identifiers worth acting on -- number, status and
// size -- so a model scanning for the right change rarely needs the second.
func Changes(_ []gerrit.ChangeInfo) string {
	return ""
}
