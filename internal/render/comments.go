package render

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// Comments renders published review comments grouped by file.
//
// The header carries the unresolved count, because that is the question a
// reviewer is actually asking when they list comments.
func Comments(byFile map[string][]gerrit.CommentInfo) string {
	counts := countComments(byFile)
	if counts.total == 0 {
		return "No published comments.\n"
	}

	var out strings.Builder

	writeCommentHeader(&out, counts.total, len(byFile), "comment")
	out.WriteString(", ")
	out.WriteString(strconv.Itoa(counts.unresolved))
	out.WriteString(" unresolved.\n")
	writeCommentBodies(&out, byFile, true)

	return out.String()
}

// Drafts renders unpublished draft comments grouped by file.
//
// Gerrit omits the author on your own drafts -- they are always yours -- so
// these are rendered separately rather than through a shared path that would
// print an empty name on every line.
func Drafts(byFile map[string][]gerrit.CommentInfo) string {
	counts := countComments(byFile)
	if counts.total == 0 {
		return "No draft comments.\n"
	}

	var out strings.Builder

	writeCommentHeader(&out, counts.total, len(byFile), "draft comment")
	out.WriteString(".\n")
	writeCommentBodies(&out, byFile, false)

	return out.String()
}

// commentCounts summarises a comment collection.
//
// A struct rather than two ints: nonamedreturns forbids naming the results and
// revive objects to two unnamed results of the same type, so the pair has to
// carry its own names. It reads better at the call site either way.
type commentCounts struct {
	total      int
	unresolved int
}

// countComments totals a comment collection.
func countComments(byFile map[string][]gerrit.CommentInfo) commentCounts {
	var counts commentCounts

	for _, comments := range byFile {
		for i := range comments {
			counts.total++

			if comments[i].Unresolved {
				counts.unresolved++
			}
		}
	}

	return counts
}

// writeCommentHeader appends the "N comments on M files" summary.
func writeCommentHeader(out *strings.Builder, total, files int, noun string) {
	out.WriteString(strconv.Itoa(total))
	out.WriteString(" ")
	out.WriteString(noun)

	if total != 1 {
		out.WriteString("s")
	}

	out.WriteString(" on ")
	out.WriteString(strconv.Itoa(files))
	out.WriteString(" file")

	if files != 1 {
		out.WriteString("s")
	}
}

// writeCommentBodies appends one section per file, sorted by path.
func writeCommentBodies(out *strings.Builder, byFile map[string][]gerrit.CommentInfo, withAuthor bool) {
	for _, path := range slices.Sorted(maps.Keys(byFile)) {
		out.WriteString("\n")
		out.WriteString(path)
		out.WriteString("\n")

		comments := byFile[path]
		for i := range comments {
			writeComment(out, &comments[i], withAuthor)
		}
	}
}

// writeComment appends one comment and its message.
func writeComment(out *strings.Builder, comment *gerrit.CommentInfo, withAuthor bool) {
	out.WriteString("  [")
	out.WriteString(comment.ID)
	out.WriteString("] ")
	out.WriteString(commentLocation(comment))

	if withAuthor && comment.Author != nil {
		out.WriteString(" · ")
		out.WriteString(comment.Author.Display())
	}

	out.WriteString(" · ps")
	out.WriteString(strconv.Itoa(comment.PatchSet))

	if comment.InReplyTo != "" {
		out.WriteString(" · in reply to ")
		out.WriteString(comment.InReplyTo)
	}

	if comment.Unresolved {
		out.WriteString(" · UNRESOLVED")
	}

	out.WriteString("\n    ")
	out.WriteString(strings.ReplaceAll(comment.Message, "\n", "\n    "))
	out.WriteString("\n")
}

// commentLocation describes where a comment sits: a range, a line, or the file
// as a whole.
//
// A PARENT-side comment is marked, because line 42 of the old file and line 42
// of the new one are different lines and would otherwise render identically.
func commentLocation(comment *gerrit.CommentInfo) string {
	location := "file"

	switch {
	case comment.Range != nil && comment.Range.EndLine != comment.Range.StartLine:
		location = "lines " + strconv.Itoa(comment.Range.StartLine) + "-" + strconv.Itoa(comment.Range.EndLine)
	case comment.Line > 0:
		location = "line " + strconv.Itoa(comment.Line)
	}

	if comment.Side == "PARENT" {
		location += " (old side)"
	}

	return location
}

// DraftCreated confirms a staged comment.
func DraftCreated(draft *gerrit.CommentInfo) string {
	var out strings.Builder

	// Saying "draft" plainly matters: nothing here is visible to reviewers
	// until publish_drafts runs.
	out.WriteString("Draft comment staged, not yet published.\n\n  [")
	out.WriteString(draft.ID)
	out.WriteString("] ")
	out.WriteString(commentLocation(draft))

	if draft.PatchSet > 0 {
		out.WriteString(" · ps")
		out.WriteString(strconv.Itoa(draft.PatchSet))
	}

	if draft.Unresolved {
		out.WriteString(" · UNRESOLVED")
	}

	out.WriteString("\n    ")
	out.WriteString(strings.ReplaceAll(draft.Message, "\n", "\n    "))
	out.WriteString("\n")

	return out.String()
}

// DraftsPublished confirms that staged comments are now visible.
func DraftsPublished(changeID string, allRevisions bool) string {
	scope := "the current patch set"
	if allRevisions {
		scope = "every patch set"
	}

	return "Draft comments on " + scope + " of change " + changeID +
		" are published and now visible to reviewers.\n"
}

// DraftDeleted confirms one staged comment was discarded.
func DraftDeleted(changeID, draftID string) string {
	return "Draft comment " + draftID + " on change " + changeID + " is discarded.\n"
}

// DraftsDeleted confirms how many staged comments were discarded.
func DraftsDeleted(changeID string, count int) string {
	if count == 0 {
		return "No draft comments were staged on change " + changeID + ".\n"
	}

	noun := " draft comments"
	if count == 1 {
		noun = " draft comment"
	}

	return "Discarded " + strconv.Itoa(count) + noun + " on change " + changeID + ".\n"
}
