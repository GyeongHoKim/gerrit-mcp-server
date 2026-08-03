package render_test

import (
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

// These renderers are the only signal an agent gets that an irreversible thing
// happened. Until now they were checked with strings.Contains spot-checks from
// the tool tests, which cannot tell "the revert still has to be reviewed" from
// a sentence that says the opposite -- both contain "review".

func TestTopicSet(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"topic_set": "cleanup",
		// An empty topic is a deletion, and the sentence has to say so rather
		// than reporting that the topic was set to nothing.
		"topic_cleared": "",
	}

	for name, topic := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.TopicSet("12345", topic))
		})
	}
}

func TestReadyForReviewSet(t *testing.T) {
	t.Parallel()

	golden(t, "ready_for_review", render.ReadyForReviewSet("12345"))
}

func TestWorkInProgressSet(t *testing.T) {
	t.Parallel()

	golden(t, "work_in_progress", render.WorkInProgressSet("12345"))
}

func TestChangeAbandoned(t *testing.T) {
	t.Parallel()

	change := &gerrit.ChangeInfo{
		Number:  12345,
		Project: "platform/base",
		Branch:  "main",
		Subject: "fix the widget alignment",
		Status:  "ABANDONED",
	}

	golden(t, "change_abandoned", render.ChangeAbandoned(change))
}

func TestChangeReverted(t *testing.T) {
	t.Parallel()

	// The revert is a new change awaiting review, not an undo that has already
	// happened. An agent that reads this as "done" stops too early.
	revert := &gerrit.ChangeInfo{
		Number:     99,
		Project:    "platform/base",
		Branch:     "main",
		Subject:    `Revert "fix the widget alignment"`,
		Status:     "NEW",
		Owner:      gerrit.AccountInfo{Username: "alice", AccountID: 1},
		Insertions: 3,
		Deletions:  42,
	}

	golden(t, "change_reverted", render.ChangeReverted("12345", revert))
}

func TestChangeCreated(t *testing.T) {
	t.Parallel()

	change := &gerrit.ChangeInfo{
		Number:  500,
		Project: "platform/base",
		Branch:  "main",
		Topic:   "cleanup",
		Subject: "add the thing",
		Status:  "NEW",
		Owner:   gerrit.AccountInfo{Name: "Alice Adams", AccountID: 1},
	}

	golden(t, "change_created", render.ChangeCreated(change))
}

func TestSubmissionReverted(t *testing.T) {
	t.Parallel()

	first := gerrit.ChangeInfo{
		Number: 101, Project: "p", Branch: "main", Subject: `Revert "first"`, Status: "NEW",
		Owner: gerrit.AccountInfo{Username: "alice", AccountID: 1}, Deletions: 10,
	}
	second := gerrit.ChangeInfo{
		Number: 102, Project: "q", Branch: "main", Subject: `Revert "second"`, Status: "NEW",
		Owner: gerrit.AccountInfo{Username: "bob", AccountID: 2}, Deletions: 4,
	}

	tests := map[string][]gerrit.ChangeInfo{
		// Gerrit answering with no reverts is a result, not an error, and the
		// sentence has to leave the agent knowing nothing was created.
		"submission_reverted_none": {},
		"submission_reverted_one":  {first},
		"submission_reverted_many": {first, second},
	}

	for name, changes := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			reverts := &gerrit.RevertSubmissionInfo{RevertChanges: changes}
			golden(t, name, render.SubmissionReverted("12345", reverts))
		})
	}
}

func TestDraftCreated(t *testing.T) {
	t.Parallel()

	tests := map[string]*gerrit.CommentInfo{
		"draft_created": {
			ID:         "d1",
			Message:    "extract this",
			Line:       42,
			PatchSet:   3,
			Unresolved: true,
		},
		// A file-level draft has no line, and a resolved one has no marker;
		// neither absence may make the confirmation read as a failure.
		"draft_created_file_level": {
			ID:      "d2",
			Message: "this file needs a header comment",
		},
		"draft_created_multiline": {
			ID:       "d3",
			Message:  "two problems here:\nthe name, and the type",
			Line:     7,
			PatchSet: 1,
		},
	}

	for name, draft := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.DraftCreated(draft))
		})
	}
}

func TestDraftsPublished(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"drafts_published_current": false,
		// The scope is the whole point: publishing across every patch set can
		// surface comments the caller wrote weeks ago and forgot.
		"drafts_published_all": true,
	}

	for name, allRevisions := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.DraftsPublished("12345", allRevisions))
		})
	}
}

func TestDraftDeleted(t *testing.T) {
	t.Parallel()

	golden(t, "draft_deleted", render.DraftDeleted("12345", "d1"))
}

func TestDraftsDeleted(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		// Nothing staged is not a failure, and must not read as one.
		"drafts_deleted_none": 0,
		"drafts_deleted_one":  1,
		"drafts_deleted_many": 3,
	}

	for name, count := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.DraftsDeleted("12345", count))
		})
	}
}

func TestReviewerAdded(t *testing.T) {
	t.Parallel()

	tests := map[string]*gerrit.ReviewerResult{
		"reviewer_added": {
			Reviewers: []gerrit.AccountInfo{{Name: "Alice Adams", AccountID: 1}},
		},
		"reviewer_added_with_cc": {
			Reviewers: []gerrit.AccountInfo{{Name: "Alice Adams", AccountID: 1}},
			CCs:       []gerrit.AccountInfo{{Username: "bob", AccountID: 2}},
		},
		// Gerrit accepts the request and names nobody when the account is
		// already on the change. Silence would read as success without saying
		// what happened.
		"reviewer_added_nobody": {},
	}

	for name, result := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.ReviewerAdded("12345", result))
		})
	}
}
