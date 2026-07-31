package gerrit

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
)

func TestSuggestReviewers(t *testing.T) {
	t.Parallel()

	const body = `[
	  {"account": {"_account_id": 2, "name": "Bob Brown", "email": "bob@example.com"}},
	  {"group": {"id": "6a1e70e1a88782771a91808c8af9bbb7a9871389", "name": "reviewers-core"}, "count": 12}
	]`

	var gotQuery url.Values

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.Query()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.SuggestReviewers(t.Context(), "12345", "bo", 5)
	if err != nil {
		t.Fatalf("SuggestReviewers() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/12345/suggest_reviewers"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if gotQuery.Get("q") != "bo" || gotQuery.Get("n") != "5" {
		t.Errorf("query = %v, want q=bo and n=5", gotQuery)
	}

	if len(got) != 2 {
		t.Fatalf("len(suggestions) = %d, want 2", len(got))
	}

	// One suggestion is a person and the other a group; conflating them would
	// lose the member count that makes a group suggestion worth judging.
	if got[0].Account == nil || got[0].Group != nil {
		t.Errorf("first suggestion = %+v, want an account", got[0])
	}

	if got[1].Group == nil || got[1].Count != 12 {
		t.Errorf("second suggestion = %+v, want a group of 12", got[1])
	}
}

func TestSuggestReviewersOmitsEmptyParameters(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		if _, err := w.Write([]byte(xssiPrefix + "\n[]")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	if _, err := client.SuggestReviewers(t.Context(), "12345", "  ", 0); err != nil {
		t.Fatalf("SuggestReviewers() returned an unexpected error: %v", err)
	}

	// An empty q is not the same as no q: Gerrit ranks differently for each.
	if gotQuery.Has("q") || gotQuery.Has("n") {
		t.Errorf("query = %v, want both parameters omitted so Gerrit's defaults apply", gotQuery)
	}
}

func TestSuggestReviewersRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("SuggestReviewers() reached the server, want it to refuse before that")
	})

	if _, err := client.SuggestReviewers(t.Context(), "", "bo", 5); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("SuggestReviewers() error = %v, want ErrEmptyChangeID", err)
	}
}
