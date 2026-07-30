package gerrit

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

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
