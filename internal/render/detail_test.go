package render_test

import (
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

func TestChangeDetail(t *testing.T) {
	t.Parallel()

	tests := map[string]*gerrit.ChangeDetail{
		"detail_full": {
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
			TotalCommentCount:      5,
			UnresolvedCommentCount: 2,
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
		},
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
