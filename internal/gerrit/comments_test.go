package gerrit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
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

	var (
		gotMethod string
		gotPath   string
	)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
