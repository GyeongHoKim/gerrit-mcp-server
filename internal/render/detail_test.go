package render_test

import (
	"strings"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

func TestChangeDetail(t *testing.T) {
	t.Parallel()

	// detail_2x is detail_full as a Gerrit older than 3.0 answers it: the same
	// change, with no comment counts in the payload. It is built from the same
	// literal below so that the two goldens can only differ by that line.
	full := &gerrit.ChangeDetail{
		ChangeInfo: gerrit.ChangeInfo{
			Number:     12345,
			Project:    "platform/base",
			Branch:     "main",
			Subject:    "fix the widget alignment",
			Status:     "NEW",
			Updated:    at(t, "2026-07-31 06:04:05.000000000"),
			Owner:      gerrit.AccountInfo{Name: "Alice Adams", AccountID: 1},
			Insertions: 42,
			Deletions:  3,
		},
		ChangeID:               "I8473b95934b5732ac55d26311a706c9c2bde9940",
		TotalCommentCount:      new(5),
		UnresolvedCommentCount: new(2),
		Labels: map[string]gerrit.LabelInfo{
			// Deliberately out of alphabetical order: the rendering has to
			// sort, or the output changes between runs.
			"Verified": {All: []gerrit.ApprovalInfo{
				{AccountInfo: gerrit.AccountInfo{Name: "CI Bot", AccountID: 9}, Value: 1},
			}},
			"Code-Review": {All: []gerrit.ApprovalInfo{
				{AccountInfo: gerrit.AccountInfo{Name: "Bob Brown", AccountID: 2}, Value: 2},
				{AccountInfo: gerrit.AccountInfo{Name: "Carol Chen", AccountID: 3}, Value: -1},
				// A zero vote means the reviewer has not scored, so it is
				// noise rather than information.
				{AccountInfo: gerrit.AccountInfo{Name: "Dave Davis", AccountID: 4}, Value: 0},
			}},
		},
		Reviewers: map[string][]gerrit.AccountInfo{
			"REVIEWER": {
				{Name: "Bob Brown", AccountID: 2},
				{Name: "Carol Chen", AccountID: 3},
			},
			"CC": {{Name: "Dave Davis", AccountID: 4}},
		},
	}

	old := *full
	old.TotalCommentCount = nil
	old.UnresolvedCommentCount = nil

	tests := map[string]*gerrit.ChangeDetail{
		"detail_full": full,
		"detail_2x":   &old,
		"detail_minimal": {
			ChangeInfo: gerrit.ChangeInfo{
				Number:  1,
				Project: "p",
				Branch:  "main",
				Subject: "initial commit",
				Status:  "MERGED",
				Owner:   gerrit.AccountInfo{AccountID: 7},
			},
		},
		"detail_no_votes": {
			ChangeInfo: gerrit.ChangeInfo{
				Number:     7,
				Project:    "tools",
				Branch:     "main",
				Subject:    "awaiting review",
				Status:     "NEW",
				Updated:    at(t, "2026-07-31 06:04:05.000000000"),
				Owner:      gerrit.AccountInfo{Username: "alice", AccountID: 1},
				Insertions: 1,
			},
			ChangeID:  "Iabc",
			Labels:    map[string]gerrit.LabelInfo{"Code-Review": {}},
			Reviewers: map[string][]gerrit.AccountInfo{"REVIEWER": {{Name: "Bob Brown", AccountID: 2}}},
		},
	}

	for name, detail := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.ChangeDetail(detail))
		})
	}
}

// A count of zero and no count at all are different answers, and the whole
// reason these fields are pointers. Golden files pin the absent case; this
// pins that a real zero still prints, so that nobody "simplifies" the render
// into skipping the line whenever the number is zero.
func TestChangeDetailDistinguishesZeroCommentsFromNone(t *testing.T) {
	t.Parallel()

	base := gerrit.ChangeInfo{Number: 1, Project: "p", Branch: "main", Subject: "s", Status: "NEW"}

	tests := map[string]struct {
		detail *gerrit.ChangeDetail
		want   string
	}{
		"a change nobody has commented on": {
			detail: &gerrit.ChangeDetail{ChangeInfo: base, TotalCommentCount: new(0)},
			want:   "Comments: 0 total, 0 unresolved\n",
		},
		// If a host ever sends the total and omits the unresolved count when it
		// is zero, the line still has to appear.
		"a total without an unresolved count": {
			detail: &gerrit.ChangeDetail{ChangeInfo: base, TotalCommentCount: new(4)},
			want:   "Comments: 4 total, 0 unresolved\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := render.ChangeDetail(test.detail); !strings.Contains(got, test.want) {
				t.Errorf("ChangeDetail() = %q, want it to contain %q", got, test.want)
			}
		})
	}
}

func TestChangeDetailSortsLabelsDeterministically(t *testing.T) {
	t.Parallel()

	detail := &gerrit.ChangeDetail{
		ChangeInfo: gerrit.ChangeInfo{Number: 1, Project: "p", Branch: "main", Subject: "s", Status: "NEW"},
		Labels: map[string]gerrit.LabelInfo{
			"Zeta": {}, "Alpha": {}, "Mu": {},
		},
	}

	// Map iteration order is randomised per run, so repeating the call is a
	// real check rather than a formality.
	first := render.ChangeDetail(detail)
	for range 20 {
		if got := render.ChangeDetail(detail); got != first {
			t.Fatalf("output is not stable across calls:\n--- first ---\n%s\n--- later ---\n%s", first, got)
		}
	}
}

func TestCommitMessage(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		message *gerrit.CommitMessageInfo
		want    string
	}{
		"passed through unchanged": {
			message: &gerrit.CommitMessageInfo{
				Subject:     "Add feature X",
				FullMessage: "Add feature X\n\nFeature X helps with foo.\n\nBug: 123\nChange-Id: I1039447\n",
			},
			want: "Add feature X\n\nFeature X helps with foo.\n\nBug: 123\nChange-Id: I1039447\n",
		},
		"a missing trailing newline is added": {
			message: &gerrit.CommitMessageInfo{Subject: "Terse", FullMessage: "Terse"},
			want:    "Terse\n",
		},
		"an empty message falls back to the subject": {
			message: &gerrit.CommitMessageInfo{Subject: "Only a subject"},
			want:    "Only a subject\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := render.CommitMessage(test.message); got != test.want {
				t.Errorf("CommitMessage() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBugs(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want string
		bugs []string
	}{
		"none": {
			bugs: nil,
			want: "No issue references in the commit message.\n",
		},
		"one": {
			bugs: []string{"123"},
			want: "1 issue reference.\n\n  123\n",
		},
		"several": {
			bugs: []string{"123", "b/456"},
			want: "2 issue references.\n\n  123\n  b/456\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := render.Bugs(test.bugs); got != test.want {
				t.Errorf("Bugs() = %q, want %q", got, test.want)
			}
		})
	}
}
