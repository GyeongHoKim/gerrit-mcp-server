package gerrit

import (
	"encoding/json"
	"errors"
	"io"
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

func TestAddReviewer(t *testing.T) {
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

		const added = `{"reviewers": [{"_account_id": 2, "name": "Bob Brown"}]}`

		if _, writeErr := w.Write([]byte(xssiPrefix + "\n" + added)); writeErr != nil {
			t.Errorf("writing test response: %v", writeErr)
		}
	})

	got, err := client.AddReviewer(t.Context(), "12345", &ReviewerInput{Reviewer: "bob@example.com"})
	if err != nil {
		t.Fatalf("AddReviewer() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}

	if want := "/a/changes/12345/reviewers"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if gotBody["reviewer"] != "bob@example.com" {
		t.Errorf("reviewer = %v, want the address forwarded", gotBody["reviewer"])
	}

	if len(got.Reviewers) != 1 {
		t.Errorf("Reviewers = %+v, want one", got.Reviewers)
	}
}

func TestAddReviewerSurfacesGerritsRefusal(t *testing.T) {
	t.Parallel()

	// Gerrit answers 200 and reports the refusal in the body, so a status
	// check alone would call this a success.
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		const refused = `{"error": "Account nobody not found"}`

		if _, err := w.Write([]byte(xssiPrefix + "\n" + refused)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	_, err := client.AddReviewer(t.Context(), "12345", &ReviewerInput{Reviewer: "nobody"})
	if !errors.Is(err, ErrReviewerRejected) {
		t.Errorf("AddReviewer() error = %v, want ErrReviewerRejected", err)
	}
}

func TestAddReviewerAsksBeforeAddingALargeGroup(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		const needsConfirming = `{"confirm": true, "error": "The group everyone has 300 members"}`

		if _, err := w.Write([]byte(xssiPrefix + "\n" + needsConfirming)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	// Confirmation is a distinct outcome from a refusal: the caller can
	// retry, and the error has to say so rather than reading as a dead end.
	_, err := client.AddReviewer(t.Context(), "12345", &ReviewerInput{Reviewer: "everyone"})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("AddReviewer() error = %v, want ErrConfirmationRequired", err)
	}
}

func TestAddReviewerRejectsEmptyArguments(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want               error
		changeID, reviewer string
	}{
		"no change":   {changeID: " ", reviewer: "bob", want: ErrEmptyChangeID},
		"no reviewer": {changeID: "12345", reviewer: "  ", want: ErrEmptyReviewer},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("AddReviewer() reached the server, want it to refuse before that")
			})

			in := &ReviewerInput{Reviewer: test.reviewer}
			if _, err := client.AddReviewer(t.Context(), test.changeID, in); !errors.Is(err, test.want) {
				t.Errorf("AddReviewer() error = %v, want %v", err, test.want)
			}
		})
	}
}
