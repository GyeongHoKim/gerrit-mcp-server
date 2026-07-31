// Package render turns Gerrit types into the compact text a model reads.
//
// Gerrit's JSON is far larger than anything the model needs. Everything the
// tools return passes through here, which keeps responses inside a sensible
// token budget and keeps the output readable to a person debugging a session.
package render

import (
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// minuteLayout prints a timestamp to the minute. Gerrit records nanoseconds;
// nobody reviewing a change needs them.
const minuteLayout = "2006-01-02 15:04"

// Changes renders a change list, one block per change.
//
// The first line carries the identifiers worth acting on -- number, status and
// size -- so a model scanning for the right change rarely needs the second.
func Changes(changes []gerrit.ChangeInfo) string {
	if len(changes) == 0 {
		return "No changes matched.\n"
	}

	var out strings.Builder

	out.WriteString(strconv.Itoa(len(changes)))

	if len(changes) == 1 {
		out.WriteString(" change")
	} else {
		out.WriteString(" changes")
	}

	// Gerrit flags a result set it cut short on the final entry. Reporting the
	// count without saying so would state that a project has 25 open changes
	// when it has nine hundred.
	if truncated(changes) {
		out.WriteString(", and more were not returned. Narrow the query or raise the limit.\n")
	} else {
		out.WriteString(".\n")
	}

	// Indexed rather than ranged by value: a ChangeInfo is over 200 bytes and
	// copying one per iteration is pure waste.
	for i := range changes {
		out.WriteString("\n")
		writeChange(&out, &changes[i])
	}

	return out.String()
}

// truncated reports whether Gerrit cut the result set short.
func truncated(changes []gerrit.ChangeInfo) bool {
	for i := range changes {
		if changes[i].MoreChanges {
			return true
		}
	}

	return false
}

// writeChange appends the two-line block for one change.
func writeChange(out *strings.Builder, change *gerrit.ChangeInfo) {
	out.WriteString("[")
	out.WriteString(strconv.Itoa(change.Number))
	out.WriteString("] ")
	out.WriteString(change.Status)

	if change.WorkInProgress {
		out.WriteString(" WIP")
	}

	if change.IsPrivate {
		out.WriteString(" PRIVATE")
	}

	out.WriteString(" +")
	out.WriteString(strconv.Itoa(change.Insertions))
	out.WriteString("/-")
	out.WriteString(strconv.Itoa(change.Deletions))
	out.WriteString(" — ")
	out.WriteString(change.Subject)
	out.WriteString("\n  ")
	out.WriteString(change.Project)
	out.WriteString(" ~ ")
	out.WriteString(change.Branch)

	if change.Topic != "" {
		out.WriteString(" · topic ")
		out.WriteString(change.Topic)
	}

	out.WriteString(" · ")
	out.WriteString(change.Owner.Display())

	if !change.Updated.IsZero() {
		out.WriteString(" · ")
		out.WriteString(change.Updated.Format(minuteLayout))
		out.WriteString(" UTC")
	}

	out.WriteString("\n")
}

// SubmittedTogether renders the changes that would submit alongside another.
//
// An empty list is not "no results": it means the change submits on its own,
// which is what the caller actually wanted to know.
func SubmittedTogether(changes []gerrit.ChangeInfo) string {
	if len(changes) == 0 {
		return "This change submits by itself.\n"
	}

	var out strings.Builder

	out.WriteString(strconv.Itoa(len(changes)))
	out.WriteString(" changes submit together, including this one.\n")

	for i := range changes {
		out.WriteString("\n")
		writeChange(&out, &changes[i])
	}

	return out.String()
}
