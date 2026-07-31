package gerrit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const commentsBody = `{
  "/COMMIT_MSG": [
    {"id": "c1", "line": 7, "message": "typo in the subject", "updated": "2026-07-31 06:04:05.000000000",
     "author": {"_account_id": 2, "name": "Bob Brown"}, "patch_set": 2, "unresolved": true}
  ],
  "src/widget.go": [
    {"id": "c2", "line": 42, "message": "extract this", "updated": "2026-07-30 09:00:00.000000000",
     "author": {"_account_id": 3, "name": "Carol Chen"}, "patch_set": 2, "unresolved": true,
     "range": {"start_line": 42, "start_character": 4, "end_line": 44, "end_character": 1}},
    {"id": "c3", "in_reply_to": "c2", "line": 42, "message": "done", "updated": "2026-07-30 10:00:00.000000000",
     "author": {"_account_id": 1, "name": "Alice Adams"}, "patch_set": 2},
    {"id": "c4", "message": "file-level note", "updated": "2026-07-30 11:00:00.000000000",
     "author": {"_account_id": 1, "name": "Alice Adams"}, "patch_set": 2}
  ]
}`

func TestListComments(t *testing.T) {
	t.Parallel()

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + commentsBody)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.ListComments(t.Context(), "12345")
	if err != nil {
		t.Fatalf("ListComments() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/12345/comments"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if len(got["src/widget.go"]) != 3 {
		t.Fatalf("comments on src/widget.go = %d, want 3", len(got["src/widget.go"]))
	}

	first := got["src/widget.go"][0]
	if first.Range == nil || first.Range.EndLine != 44 {
		t.Errorf("Range = %+v, want a range ending on line 44", first.Range)
	}

	if !first.Unresolved {
		t.Error("Unresolved = false, want true")
	}

	// A reply carries in_reply_to; without it a thread cannot be reconstructed.
	if got["src/widget.go"][1].InReplyTo != "c2" {
		t.Errorf("InReplyTo = %q, want c2", got["src/widget.go"][1].InReplyTo)
	}

	// A file-level comment has no line at all, which is not the same as line 0.
	if got["src/widget.go"][2].Line != 0 {
		t.Errorf("Line = %d, want 0 for a file-level comment", got["src/widget.go"][2].Line)
	}
}

func TestListCommentsRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("ListComments() reached the server, want it to refuse before that")
	})

	if _, err := client.ListComments(t.Context(), " "); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("ListComments() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestListDraftComments(t *testing.T) {
	t.Parallel()

	const body = `{
	  "src/widget.go": [
	    {"id": "d1", "line": 10, "message": "still thinking", "patch_set": 3,
	     "updated": "2026-07-31 06:04:05.000000000"}
	  ]
	}`

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.ListDraftComments(t.Context(), "12345")
	if err != nil {
		t.Fatalf("ListDraftComments() returned an unexpected error: %v", err)
	}

	// The drafts endpoint is the one AGENTS.md warns moves between versions,
	// so its path is worth pinning rather than assuming.
	if want := "/a/changes/12345/drafts"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	// Gerrit omits the author on your own drafts; they are always yours.
	if draft := got["src/widget.go"][0]; draft.Author != nil {
		t.Errorf("Author = %+v, want it absent on a draft", draft.Author)
	}
}

func TestListDraftCommentsRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("ListDraftComments() reached the server, want it to refuse before that")
	})

	if _, err := client.ListDraftComments(t.Context(), ""); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("ListDraftComments() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestPublishDrafts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		wantDrafts   string
		allRevisions bool
	}{
		// Gerrit's own default is KEEP, so the scope is always stated
		// explicitly rather than inherited.
		"current revision only": {allRevisions: false, wantDrafts: PublishCurrentDrafts},
		"every revision":        {allRevisions: true, wantDrafts: PublishAllDrafts},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				gotMethod string
				gotPath   string
				gotBody   map[string]any
			)

			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.EscapedPath()

				body, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					t.Errorf("reading request body: %v", readErr)
				}

				if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
					t.Errorf("decoding request body %q: %v", body, decodeErr)
				}

				if _, writeErr := w.Write([]byte(xssiPrefix + "\n{}")); writeErr != nil {
					t.Errorf("writing test response: %v", writeErr)
				}
			})

			if _, err := client.PublishDrafts(t.Context(), "12345", "looks good", test.allRevisions); err != nil {
				t.Fatalf("PublishDrafts() returned an unexpected error: %v", err)
			}

			if gotMethod != http.MethodPost {
				t.Errorf("method = %s, want POST", gotMethod)
			}

			if want := "/a/changes/12345/revisions/current/review"; gotPath != want {
				t.Errorf("path = %q, want %q", gotPath, want)
			}

			if gotBody["drafts"] != test.wantDrafts {
				t.Errorf("drafts = %v, want %q", gotBody["drafts"], test.wantDrafts)
			}

			if gotBody["message"] != "looks good" {
				t.Errorf("message = %v, want the cover message forwarded", gotBody["message"])
			}
		})
	}
}

func TestPublishDraftsSurfacesAReviewGerritDeclined(t *testing.T) {
	t.Parallel()

	// Gerrit answers 200 and reports the refusal in the body, so a status
	// check alone would call this a success.
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		const declined = `{"error": "Applying label Code-Review: -2 is restricted"}`

		if _, err := w.Write([]byte(xssiPrefix + "\n" + declined)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	_, err := client.PublishDrafts(t.Context(), "12345", "", false)
	if !errors.Is(err, ErrReviewRejected) {
		t.Errorf("PublishDrafts() error = %v, want ErrReviewRejected", err)
	}
}

func TestPublishDraftsRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("PublishDrafts() reached the server, want it to refuse before that")
	})

	if _, err := client.PublishDrafts(t.Context(), " ", "", false); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("PublishDrafts() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestDeleteDraftComment(t *testing.T) {
	t.Parallel()

	// The draft was written on patch set 2 while the change is further along,
	// which is the case that a delete against "current" gets wrong.
	const drafts = `{"src/widget.go": [{"id": "d1", "patch_set": 2, "message": "one"}]}`

	var (
		gotMethod string
		gotPath   string
	)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if _, err := w.Write([]byte(xssiPrefix + "\n" + drafts)); err != nil {
				t.Errorf("writing test response: %v", err)
			}

			return
		}

		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()

		// Gerrit answers a draft deletion with 204 and no body at all.
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeleteDraftComment(t.Context(), "12345", "d1"); err != nil {
		t.Fatalf("DeleteDraftComment() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}

	if want := "/a/changes/12345/revisions/2/drafts/d1"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestDeleteDraftCommentWithoutAKnownPatchSet(t *testing.T) {
	t.Parallel()

	// Nothing staged carries this id, so there is no patch set to aim at and
	// the current revision is the only thing left to try.
	const drafts = `{"src/widget.go": [{"id": "d2", "patch_set": 2}]}`

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if _, err := w.Write([]byte(xssiPrefix + "\n" + drafts)); err != nil {
				t.Errorf("writing test response: %v", err)
			}

			return
		}

		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeleteDraftComment(t.Context(), "12345", "d1"); err != nil {
		t.Fatalf("DeleteDraftComment() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/12345/revisions/current/drafts/d1"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestDeleteDraftCommentRejectsEmptyArguments(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want              error
		changeID, draftID string
	}{
		"no change": {changeID: " ", draftID: "d1", want: ErrEmptyChangeID},
		"no draft":  {changeID: "12345", draftID: "  ", want: ErrEmptyDraftID},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("DeleteDraftComment() reached the server, want it to refuse before that")
			})

			if err := client.DeleteDraftComment(t.Context(), test.changeID, test.draftID); !errors.Is(err, test.want) {
				t.Errorf("DeleteDraftComment() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDeleteAllDraftComments(t *testing.T) {
	t.Parallel()

	// Drafts survive a new patch set, so a change under review for any length
	// of time has them spread across several. Each has to be deleted where it
	// actually lives.
	const drafts = `{
	  "src/widget.go": [
	    {"id": "d1", "patch_set": 1, "line": 10, "message": "one"},
	    {"id": "d2", "patch_set": 3, "line": 20, "message": "two"}
	  ],
	  "src/other.go": [{"id": "d3", "message": "three"}]
	}`

	var deleted []string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if _, err := w.Write([]byte(xssiPrefix + "\n" + drafts)); err != nil {
				t.Errorf("writing test response: %v", err)
			}

			return
		}

		deleted = append(deleted, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	})

	count, err := client.DeleteAllDraftComments(t.Context(), "12345")
	if err != nil {
		t.Fatalf("DeleteAllDraftComments() returned an unexpected error: %v", err)
	}

	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	slices.Sort(deleted)

	// d3 named no patch set, so it can only be deleted against the current one.
	want := []string{
		"/a/changes/12345/revisions/1/drafts/d1",
		"/a/changes/12345/revisions/3/drafts/d2",
		"/a/changes/12345/revisions/current/drafts/d3",
	}

	if diff := cmp.Diff(want, deleted); diff != "" {
		t.Errorf("deleted drafts mismatch (-want +got):\n%s", diff)
	}
}

func TestDeleteAllDraftCommentsWithNothingStaged(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s request, want no deletions when nothing is staged", r.Method)
		}

		if _, err := w.Write([]byte(xssiPrefix + "\n{}")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	count, err := client.DeleteAllDraftComments(t.Context(), "12345")
	if err != nil {
		t.Fatalf("DeleteAllDraftComments() returned an unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestDeleteAllDraftCommentsReportsPartialProgress(t *testing.T) {
	t.Parallel()

	const drafts = `{"src/widget.go": [{"id": "d1"}, {"id": "d2"}]}`

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if _, err := w.Write([]byte(xssiPrefix + "\n" + drafts)); err != nil {
				t.Errorf("writing test response: %v", err)
			}

			return
		}

		// The second deletion fails. The first one already happened and
		// cannot be taken back, so the count has to say so.
		if path.Base(r.URL.EscapedPath()) == "d2" {
			w.WriteHeader(http.StatusConflict)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	count, err := client.DeleteAllDraftComments(t.Context(), "12345")
	if err == nil {
		t.Fatal("DeleteAllDraftComments() succeeded, want the failed deletion reported")
	}

	if count != 1 {
		t.Errorf("count = %d, want 1 -- the first deletion is not undone by the second failing", count)
	}
}
