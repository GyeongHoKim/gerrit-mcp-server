package gerrit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestSetTopic(t *testing.T) {
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

		if len(body) > 0 {
			if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
				t.Errorf("decoding request body %q: %v", body, decodeErr)
			}
		}

		if _, writeErr := w.Write([]byte(xssiPrefix + "\n\"cleanup\"")); writeErr != nil {
			t.Errorf("writing test response: %v", writeErr)
		}
	})

	if err := client.SetTopic(t.Context(), "12345", "cleanup"); err != nil {
		t.Fatalf("SetTopic() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}

	if want := "/a/changes/12345/topic"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if gotBody["topic"] != "cleanup" {
		t.Errorf("topic = %v, want the new topic forwarded", gotBody["topic"])
	}
}

func TestSetTopicClearsWithDelete(t *testing.T) {
	t.Parallel()

	var gotMethod string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	// Clearing is a separate verb. PUTting an empty string would leave the
	// change with a blank topic rather than with none.
	if err := client.SetTopic(t.Context(), "12345", "  "); err != nil {
		t.Fatalf("SetTopic() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE for an empty topic", gotMethod)
	}
}

func TestSetTopicRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("SetTopic() reached the server, want it to refuse before that")
	})

	if err := client.SetTopic(t.Context(), "", "cleanup"); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("SetTopic() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestSetReadyAndWorkInProgress(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		call     func(*Client, string) error
		wantPath string
	}{
		"ready for review": {
			call:     func(c *Client, id string) error { return c.SetReadyForReview(t.Context(), id, "done") },
			wantPath: "/a/changes/12345/ready",
		},
		"work in progress": {
			call:     func(c *Client, id string) error { return c.SetWorkInProgress(t.Context(), id, "done") },
			wantPath: "/a/changes/12345/wip",
		},
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

				if len(body) > 0 {
					if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
						t.Errorf("decoding request body %q: %v", body, decodeErr)
					}
				}

				// Gerrit answers these with 200 and no body.
				w.WriteHeader(http.StatusOK)
			})

			if err := test.call(client, "12345"); err != nil {
				t.Fatalf("call returned an unexpected error: %v", err)
			}

			if gotMethod != http.MethodPost {
				t.Errorf("method = %s, want POST", gotMethod)
			}

			if gotPath != test.wantPath {
				t.Errorf("path = %q, want %q", gotPath, test.wantPath)
			}

			if gotBody["message"] != "done" {
				t.Errorf("message = %v, want the note forwarded", gotBody["message"])
			}
		})
	}
}

func TestSetReadyForReviewRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("SetReadyForReview() reached the server, want it to refuse before that")
	})

	if err := client.SetReadyForReview(t.Context(), " ", ""); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("SetReadyForReview() error = %v, want ErrEmptyChangeID", err)
	}
}
