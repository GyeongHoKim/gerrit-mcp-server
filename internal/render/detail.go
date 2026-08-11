package render

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// reviewerSections maps Gerrit's reviewer states onto the labels we print, in
// the order they appear.
var reviewerSections = []struct{ state, label string }{
	{state: gerrit.StateReviewer, label: "Reviewers"},
	{state: gerrit.StateCC, label: "CC"},
}

// ChangeDetail renders one change with its review state.
//
// Each aspect gets a single line. A model asking for detail usually wants to
// know who has voted and what is unresolved, not to read a JSON document.
func ChangeDetail(detail *gerrit.ChangeDetail) string {
	var out strings.Builder

	writeChange(&out, &detail.ChangeInfo)

	// Both of the lines below are optional, so the blank line that separates
	// them from the change block is only worth writing when one of them
	// appeared. Built separately rather than guarded twice: on an older Gerrit
	// with no Change-Id in the payload, neither is written and the output
	// would otherwise end on a stray blank line.
	if meta := changeMeta(detail); meta != "" {
		out.WriteString("\n")
		out.WriteString(meta)
	}

	writeLabels(&out, detail.Labels)
	writeReviewers(&out, detail.Reviewers)

	return out.String()
}

// changeMeta renders the change's identity and comment counts, either of which
// Gerrit may not have sent.
func changeMeta(detail *gerrit.ChangeDetail) string {
	var out strings.Builder

	if detail.ChangeID != "" {
		out.WriteString("Change-Id: ")
		out.WriteString(detail.ChangeID)
		out.WriteString("\n")
	}

	// Gerrit did not report comment counts before 3.0. Saying nothing is the
	// honest answer there; "0 total" would be a number we made up.
	//
	// Only the total is required. If a host ever omits the unresolved count
	// when it is zero, demanding both would drop the line from every change
	// with nothing outstanding.
	if detail.TotalCommentCount != nil {
		out.WriteString("Comments: ")
		out.WriteString(strconv.Itoa(*detail.TotalCommentCount))
		out.WriteString(" total, ")
		out.WriteString(strconv.Itoa(count(detail.UnresolvedCommentCount)))
		out.WriteString(" unresolved\n")
	}

	return out.String()
}

// count reads an optional count, treating an absent one as zero.
func count(value *int) int {
	if value == nil {
		return 0
	}

	return *value
}

// writeLabels appends the vote state, sorted by label name.
//
// Sorting is not cosmetic: Go randomises map iteration, so unsorted output
// would differ between two calls on identical input.
func writeLabels(out *strings.Builder, labels map[string]gerrit.LabelInfo) {
	if len(labels) == 0 {
		return
	}

	out.WriteString("\nLabels:\n")

	for _, name := range slices.Sorted(maps.Keys(labels)) {
		out.WriteString("  ")
		out.WriteString(name)
		out.WriteString(": ")
		writeVotes(out, labels[name].All)
		out.WriteString("\n")
	}
}

// writeVotes appends the scored votes for one label.
func writeVotes(out *strings.Builder, votes []gerrit.ApprovalInfo) {
	scored := 0

	for i := range votes {
		// A zero is a reviewer who has not scored, which is not information.
		if votes[i].Value == 0 {
			continue
		}

		if scored > 0 {
			out.WriteString(", ")
		}

		if votes[i].Value > 0 {
			out.WriteString("+")
		}

		out.WriteString(strconv.Itoa(votes[i].Value))
		out.WriteString(" ")
		out.WriteString(votes[i].Display())

		scored++
	}

	if scored == 0 {
		out.WriteString("no votes")
	}
}

// writeReviewers appends the reviewer and CC lines that have anyone on them.
func writeReviewers(out *strings.Builder, reviewers map[string][]gerrit.AccountInfo) {
	opened := false

	for _, section := range reviewerSections {
		accounts := reviewers[section.state]
		if len(accounts) == 0 {
			continue
		}

		if !opened {
			out.WriteString("\n")

			opened = true
		}

		names := make([]string, 0, len(accounts))
		for i := range accounts {
			names = append(names, accounts[i].Display())
		}

		out.WriteString(section.label)
		out.WriteString(": ")
		out.WriteString(strings.Join(names, ", "))
		out.WriteString("\n")
	}
}

// CommitMessage renders a commit message.
//
// The message is already prose written for humans, so it is passed through
// unchanged apart from guaranteeing the trailing newline that Gerrit does not
// always send.
func CommitMessage(message *gerrit.CommitMessageInfo) string {
	body := message.FullMessage
	if body == "" {
		body = message.Subject
	}

	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}

	return body
}

// Bugs renders the issue references extracted from a commit message.
func Bugs(bugs []string) string {
	if len(bugs) == 0 {
		return "No issue references in the commit message.\n"
	}

	var out strings.Builder

	writeCount(&out, len(bugs), "issue reference")
	out.WriteString(".\n\n")

	for _, bug := range bugs {
		out.WriteString("  ")
		out.WriteString(bug)
		out.WriteString("\n")
	}

	return out.String()
}
