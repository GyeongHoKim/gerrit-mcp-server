package render_test

import (
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

func TestReviewers(t *testing.T) {
	t.Parallel()

	tests := map[string][]gerrit.SuggestedReviewerInfo{
		"reviewers_none": {},
		"reviewers_mixed": {
			{Account: &gerrit.AccountInfo{Name: "Bob Brown", Email: "bob@example.com", AccountID: 2}},
			{Account: &gerrit.AccountInfo{Username: "carol", AccountID: 3}},
			{Group: &gerrit.GroupBaseInfo{ID: "6a1e70e1", Name: "reviewers-core"}, Count: 12},
			{Group: &gerrit.GroupBaseInfo{ID: "abc", Name: "solo-group"}, Count: 1},
		},
	}

	for name, suggestions := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.Reviewers(suggestions))
		})
	}
}
