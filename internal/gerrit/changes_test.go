package gerrit

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestTimestampRoundTrips(t *testing.T) {
	t.Parallel()

	// Timestamp embeds time.Time, which promotes an RFC 3339 MarshalJSON. A
	// type that decodes Gerrit's format and encodes another one is a trap for
	// whoever first puts a Timestamp in a request body.
	tests := map[string]string{
		"gerrit format with nanoseconds":                    `"2026-07-31 06:04:05.000000000"`,
		"fractional seconds are kept":                       `"2026-07-31 06:04:05.123456789"`,
		"the zero time is the empty string it decoded from": `""`,
	}

	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var decoded Timestamp
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				t.Fatalf("Unmarshal(%s) returned an unexpected error: %v", raw, err)
			}

			encoded, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("Marshal() returned an unexpected error: %v", err)
			}

			if diff := cmp.Diff(raw, string(encoded)); diff != "" {
				t.Errorf("round trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTimestampUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want    time.Time
		raw     string
		wantErr bool
	}{
		"gerrit format with nanoseconds": {
			raw:  `"2026-07-31 06:04:05.000000000"`,
			want: time.Date(2026, time.July, 31, 6, 4, 5, 0, time.UTC),
		},
		"fractional seconds are kept": {
			raw:  `"2026-07-31 06:04:05.123456789"`,
			want: time.Date(2026, time.July, 31, 6, 4, 5, 123456789, time.UTC),
		},
		"empty string is the zero time": {
			raw:  `""`,
			want: time.Time{},
		},
		"rfc 3339 is not gerrit's format": {
			raw:     `"2026-07-31T06:04:05Z"`,
			wantErr: true,
		},
		"not a string": {
			raw:     `12345`,
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got Timestamp

			err := json.Unmarshal([]byte(test.raw), &got)
			if test.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) succeeded, want an error", test.raw)
				}

				return
			}

			if err != nil {
				t.Fatalf("Unmarshal(%s) returned an unexpected error: %v", test.raw, err)
			}

			if !got.Equal(test.want) {
				t.Errorf("time = %v, want %v", got.Time, test.want)
			}

			if got.Location() != time.UTC && !got.IsZero() {
				t.Errorf("location = %v, want UTC", got.Location())
			}
		})
	}
}

func TestAccountInfoDisplay(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want    string
		account AccountInfo
	}{
		"name wins": {
			account: AccountInfo{Name: "Alice Adams", Email: "alice@example.com", Username: "alice"},
			want:    "Alice Adams",
		},
		"username when there is no name": {
			account: AccountInfo{Email: "alice@example.com", Username: "alice"},
			want:    "alice",
		},
		"email when there is neither": {
			account: AccountInfo{Email: "alice@example.com"},
			want:    "alice@example.com",
		},
		"account id as a last resort": {
			account: AccountInfo{AccountID: 1000096},
			want:    "1000096",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := test.account.Display(); got != test.want {
				t.Errorf("Display() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestQueryChanges(t *testing.T) {
	t.Parallel()

	const body = `[
	  {
	    "id": "platform%2Fbase~main~I8473b95934b5732ac55d26311a706c9c2bde9940",
	    "project": "platform/base",
	    "branch": "main",
	    "subject": "fix the widget alignment",
	    "status": "NEW",
	    "updated": "2026-07-31 06:04:05.000000000",
	    "insertions": 42,
	    "deletions": 3,
	    "_number": 12345,
	    "owner": {"_account_id": 1000096, "name": "Alice Adams", "username": "alice"},
	    "work_in_progress": true
	  }
	]`

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.QueryChanges(t.Context(), "status:open owner:self", 25)
	if err != nil {
		t.Fatalf("QueryChanges() returned an unexpected error: %v", err)
	}

	if want := "status:open owner:self"; gotQuery.Get("q") != want {
		t.Errorf("q = %q, want %q", gotQuery.Get("q"), want)
	}

	if want := "25"; gotQuery.Get("n") != want {
		t.Errorf("n = %q, want %q", gotQuery.Get("n"), want)
	}

	// Without this the owner comes back as a bare account id and every change
	// renders as a number instead of a person.
	if want := "DETAILED_ACCOUNTS"; gotQuery.Get("o") != want {
		t.Errorf("o = %q, want %q", gotQuery.Get("o"), want)
	}

	want := []ChangeInfo{{
		ID:             "platform%2Fbase~main~I8473b95934b5732ac55d26311a706c9c2bde9940",
		Project:        "platform/base",
		Branch:         "main",
		Subject:        "fix the widget alignment",
		Status:         "NEW",
		Updated:        Timestamp{time.Date(2026, time.July, 31, 6, 4, 5, 0, time.UTC)},
		Owner:          AccountInfo{Name: "Alice Adams", Username: "alice", AccountID: 1000096},
		Number:         12345,
		Insertions:     42,
		Deletions:      3,
		WorkInProgress: true,
	}}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("QueryChanges() mismatch (-want +got):\n%s", diff)
	}
}

func TestQueryChangesReportsTruncation(t *testing.T) {
	t.Parallel()

	// Gerrit sets _more_changes on the last entry when the result set was cut
	// short. Dropping it makes a truncated answer look complete.
	const body = `[
	  {"_number": 1, "project": "p", "branch": "main", "subject": "one", "status": "NEW"},
	  {"_number": 2, "project": "p", "branch": "main", "subject": "two", "status": "NEW", "_more_changes": true}
	]`

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.QueryChanges(t.Context(), "status:open", 2)
	if err != nil {
		t.Fatalf("QueryChanges() returned an unexpected error: %v", err)
	}

	if got[0].MoreChanges {
		t.Error("MoreChanges = true on a non-final entry, want false")
	}

	if !got[1].MoreChanges {
		t.Error("MoreChanges = false on the final entry, want the truncation flag to survive")
	}
}

func TestQueryChangesOmitsLimitWhenUnset(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		if _, err := w.Write([]byte(xssiPrefix + "\n[]")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	if _, err := client.QueryChanges(t.Context(), "status:open", 0); err != nil {
		t.Fatalf("QueryChanges() returned an unexpected error: %v", err)
	}

	if gotQuery.Has("n") {
		t.Errorf("n = %q, want the parameter to be omitted so Gerrit's default applies", gotQuery.Get("n"))
	}
}

func TestQueryChangesRejectsEmptyQuery(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("QueryChanges() reached the server, want it to refuse before that")
	})

	_, err := client.QueryChanges(t.Context(), "   ", 10)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("QueryChanges() error = %v, want ErrEmptyQuery", err)
	}
}

func TestQueryChangesPropagatesAPIErrors(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := client.QueryChanges(t.Context(), "status:open", 0)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("QueryChanges() error = %v, want ErrForbidden", err)
	}
}

func TestGetChange(t *testing.T) {
	t.Parallel()

	const body = `{
	  "id": "platform%2Fbase~main~I8473b959",
	  "project": "platform/base", "branch": "main",
	  "change_id": "I8473b95934b5732ac55d26311a706c9c2bde9940",
	  "subject": "fix the widget alignment", "status": "NEW",
	  "updated": "2026-07-31 06:04:05.000000000",
	  "insertions": 42, "deletions": 3, "_number": 12345,
	  "current_revision": "abc123",
	  "total_comment_count": 5, "unresolved_comment_count": 2,
	  "owner": {"_account_id": 1, "name": "Alice Adams"},
	  "labels": {
	    "Code-Review": {"all": [{"_account_id": 2, "name": "Bob Brown", "value": 2}]}
	  },
	  "reviewers": {
	    "REVIEWER": [{"_account_id": 2, "name": "Bob Brown"}],
	    "CC": [{"_account_id": 3, "name": "Dave Davis"}]
	  }
	}`

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.GetChange(t.Context(), "platform/base~main~I8473b959")
	if err != nil {
		t.Fatalf("GetChange() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/platform%2Fbase~main~I8473b959/detail"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if got.Number != 12345 {
		t.Errorf("Number = %d, want 12345", got.Number)
	}

	if got.ChangeID != "I8473b95934b5732ac55d26311a706c9c2bde9940" {
		t.Errorf("ChangeID = %q, want the Change-Id footer", got.ChangeID)
	}

	if got.UnresolvedCommentCount != 2 {
		t.Errorf("UnresolvedCommentCount = %d, want 2", got.UnresolvedCommentCount)
	}

	// The embedded ChangeInfo must decode alongside the detail-only fields.
	if got.Subject != "fix the widget alignment" {
		t.Errorf("Subject = %q, want the embedded ChangeInfo to decode", got.Subject)
	}

	votes := got.Labels["Code-Review"].All
	if len(votes) != 1 || votes[0].Value != 2 || votes[0].Name != "Bob Brown" {
		t.Errorf("Code-Review votes = %+v, want one +2 from Bob Brown", votes)
	}

	if len(got.Reviewers["CC"]) != 1 {
		t.Errorf("CC reviewers = %+v, want one", got.Reviewers["CC"])
	}
}

func TestGetChangeRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("GetChange() reached the server, want it to refuse before that")
	})

	if _, err := client.GetChange(t.Context(), "  "); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("GetChange() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestGetChangeMapsNotFound(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := client.GetChange(t.Context(), "99999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetChange() error = %v, want ErrNotFound", err)
	}
}

func TestGetCommitMessage(t *testing.T) {
	t.Parallel()

	const body = `{
	  "subject": "Add feature X",
	  "full_message": "Add feature X\n\nFeature X helps with foo.\n\nBug: 123\nChange-Id: I1039447\n",
	  "footers": {"Bug": "123", "Change-Id": "I1039447"}
	}`

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.GetCommitMessage(t.Context(), "a/b~main~I1")
	if err != nil {
		t.Fatalf("GetCommitMessage() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/a%2Fb~main~I1/message"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if want := "Add feature X"; got.Subject != want {
		t.Errorf("Subject = %q, want %q", got.Subject, want)
	}

	if !strings.Contains(got.FullMessage, "Feature X helps with foo.") {
		t.Errorf("FullMessage = %q, want the body to survive", got.FullMessage)
	}

	if want := "123"; got.Footers["Bug"] != want {
		t.Errorf("Footers[Bug] = %q, want %q", got.Footers["Bug"], want)
	}
}

func TestGetCommitMessageRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("GetCommitMessage() reached the server, want it to refuse before that")
	})

	if _, err := client.GetCommitMessage(t.Context(), ""); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("GetCommitMessage() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestChangesSubmittedTogether(t *testing.T) {
	t.Parallel()

	const body = `[
	  {"_number": 12345, "project": "p", "branch": "main", "subject": "first", "status": "NEW"},
	  {"_number": 12346, "project": "p", "branch": "main", "subject": "second", "status": "NEW"}
	]`

	var (
		gotPath  string
		gotQuery url.Values
	)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.Query()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.ChangesSubmittedTogether(t.Context(), "12345")
	if err != nil {
		t.Fatalf("ChangesSubmittedTogether() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/12345/submitted_together"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	// These render the same block as a query result, so they need the same
	// account detail to name an owner.
	if want := "DETAILED_ACCOUNTS"; gotQuery.Get("o") != want {
		t.Errorf("o = %q, want %q", gotQuery.Get("o"), want)
	}

	if len(got) != 2 {
		t.Errorf("len(changes) = %d, want 2", len(got))
	}
}

func TestChangesSubmittedTogetherAloneIsEmptyNotAnError(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(xssiPrefix + "\n[]")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	// Gerrit returns an empty list when the change submits by itself. That is
	// an answer, not a failure.
	got, err := client.ChangesSubmittedTogether(t.Context(), "12345")
	if err != nil {
		t.Fatalf("ChangesSubmittedTogether() returned an unexpected error: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("len(changes) = %d, want 0", len(got))
	}
}

func TestCommitMessageBugs(t *testing.T) {
	t.Parallel()

	const footerBug = "Bug"

	tests := map[string]struct {
		footers map[string]string
		want    []string
	}{
		"no footers at all": {
			footers: nil,
			want:    nil,
		},
		"only a change id": {
			footers: map[string]string{"Change-Id": "I1039447"},
			want:    nil,
		},
		"a single bug": {
			footers: map[string]string{footerBug: "123", "Change-Id": "I1"},
			want:    []string{"123"},
		},
		"several ids in one footer": {
			footers: map[string]string{footerBug: "123, 456"},
			want:    []string{"123", "456"},
		},
		"space separated ids": {
			footers: map[string]string{footerBug: "b/123 b/456"},
			want:    []string{"b/123", "b/456"},
		},
		"several footer kinds": {
			footers: map[string]string{footerBug: "123", "Fixes": "456", "Closes": "789"},
			want:    []string{"123", "789", "456"},
		},
		// The same id under two footers is one issue, not two.
		"duplicates collapse": {
			footers: map[string]string{footerBug: "123", "Related-Bug": "123"},
			want:    []string{"123"},
		},
		"empty footer value": {
			footers: map[string]string{footerBug: "   "},
			want:    nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := &CommitMessageInfo{Footers: test.footers}

			if diff := cmp.Diff(test.want, message.Bugs()); diff != "" {
				t.Errorf("Bugs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
