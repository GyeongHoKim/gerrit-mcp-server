package render_test

import (
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

func sampleComments(t *testing.T) map[string][]gerrit.CommentInfo {
	t.Helper()

	return map[string][]gerrit.CommentInfo{
		// Unsorted on purpose.
		"src/widget.go": {
			{
				ID: "c2", Line: 42, Message: "extract this", PatchSet: 2, Unresolved: true,
				Updated: at(t, "2026-07-30 09:00:00.000000000"),
				Author:  &gerrit.AccountInfo{Name: "Carol Chen", AccountID: 3},
				Range:   &gerrit.CommentRange{StartLine: 42, EndLine: 44},
			},
			{
				ID: "c3", Line: 42, Message: "done", PatchSet: 2, InReplyTo: "c2",
				Updated: at(t, "2026-07-30 10:00:00.000000000"),
				Author:  &gerrit.AccountInfo{Name: "Alice Adams", AccountID: 1},
			},
			{
				ID: "c4", Message: "file-level note", PatchSet: 2,
				Updated: at(t, "2026-07-30 11:00:00.000000000"),
				Author:  &gerrit.AccountInfo{Name: "Alice Adams", AccountID: 1},
			},
		},
		"/COMMIT_MSG": {
			{
				ID: "c1", Line: 7, Message: "typo in the subject", PatchSet: 2, Unresolved: true,
				Updated: at(t, "2026-07-31 06:04:05.000000000"),
				Author:  &gerrit.AccountInfo{Name: "Bob Brown", AccountID: 2},
			},
		},
	}
}

func TestComments(t *testing.T) {
	t.Parallel()

	t.Run("comments_none", func(t *testing.T) {
		t.Parallel()

		golden(t, "comments_none", render.Comments(map[string][]gerrit.CommentInfo{}))
	})

	t.Run("comments_threaded", func(t *testing.T) {
		t.Parallel()

		golden(t, "comments_threaded", render.Comments(sampleComments(t)))
	})
}

func TestDrafts(t *testing.T) {
	t.Parallel()

	t.Run("drafts_none", func(t *testing.T) {
		t.Parallel()

		golden(t, "drafts_none", render.Drafts(map[string][]gerrit.CommentInfo{}))
	})

	t.Run("drafts_some", func(t *testing.T) {
		t.Parallel()

		// A draft has no author: it is always the calling account's own.
		golden(t, "drafts_some", render.Drafts(map[string][]gerrit.CommentInfo{
			"src/widget.go": {{
				ID: "d1", Line: 10, Message: "still thinking about this", PatchSet: 3,
				Updated: at(t, "2026-07-31 06:04:05.000000000"),
			}},
		}))
	})
}

func TestCommentsDistinguishesTheOldSide(t *testing.T) {
	t.Parallel()

	// Two comments on "line 42" of the same file, one on each side. Rendering
	// them identically leaves the model no way to tell them apart.
	golden(t, "comments_sides", render.Comments(map[string][]gerrit.CommentInfo{
		"src/widget.go": {
			{
				ID: "s1", Line: 42, Message: "this line is going away", PatchSet: 2, Side: "PARENT",
				Updated: at(t, "2026-07-31 06:04:05.000000000"),
				Author:  &gerrit.AccountInfo{Name: "Bob Brown", AccountID: 2},
			},
			{
				ID: "s2", Line: 42, Message: "and this replaces it", PatchSet: 2, Side: "REVISION",
				Updated: at(t, "2026-07-31 06:05:00.000000000"),
				Author:  &gerrit.AccountInfo{Name: "Bob Brown", AccountID: 2},
			},
		},
	}))
}

func TestCommentsSortsFilesDeterministically(t *testing.T) {
	t.Parallel()

	comments := sampleComments(t)

	first := render.Comments(comments)
	for range 20 {
		if got := render.Comments(comments); got != first {
			t.Fatalf("output is not stable across calls:\n--- first ---\n%s\n--- later ---\n%s", first, got)
		}
	}
}
