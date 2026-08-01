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

func TestAbandonAndRevert(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		call     func(*Client, string) (*ChangeInfo, error)
		wantPath string
		wantBody string
	}{
		"abandon": {
			call: func(c *Client, id string) (*ChangeInfo, error) {
				return c.AbandonChange(t.Context(), id, "superseded")
			},
			wantPath: "/a/changes/12345/abandon",
			wantBody: "superseded",
		},
		"revert": {
			call: func(c *Client, id string) (*ChangeInfo, error) {
				return c.RevertChange(t.Context(), id, "broke the build")
			},
			wantPath: "/a/changes/12345/revert",
			wantBody: "broke the build",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				gotPath string
				gotBody map[string]any
			)

			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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

				const change = `{"_number": 99, "project": "p", "branch": "main",
				  "subject": "Revert \"first\"", "status": "NEW"}`

				if _, writeErr := w.Write([]byte(xssiPrefix + "\n" + change)); writeErr != nil {
					t.Errorf("writing test response: %v", writeErr)
				}
			})

			got, err := test.call(client, "12345")
			if err != nil {
				t.Fatalf("call returned an unexpected error: %v", err)
			}

			if gotPath != test.wantPath {
				t.Errorf("path = %q, want %q", gotPath, test.wantPath)
			}

			if gotBody["message"] != test.wantBody {
				t.Errorf("message = %v, want %q", gotBody["message"], test.wantBody)
			}

			// Both answer with a ChangeInfo. For a revert that is a different
			// change from the one asked about, which is the whole point.
			if got.Number != 99 {
				t.Errorf("Number = %d, want 99 from the response", got.Number)
			}
		})
	}
}

func TestAbandonChangeRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("AbandonChange() reached the server, want it to refuse before that")
	})

	if _, err := client.AbandonChange(t.Context(), "", ""); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("AbandonChange() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestCreateChange(t *testing.T) {
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

		const created = `{"_number": 500, "project": "p", "branch": "main",
		  "subject": "add the thing", "status": "NEW", "work_in_progress": true}`

		if _, writeErr := w.Write([]byte(xssiPrefix + "\n" + created)); writeErr != nil {
			t.Errorf("writing test response: %v", writeErr)
		}
	})

	got, err := client.CreateChange(t.Context(), ChangeInput{
		Project:        "p",
		Branch:         "main",
		Subject:        "add the thing",
		WorkInProgress: true,
	})
	if err != nil {
		t.Fatalf("CreateChange() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}

	// Creation posts to the collection, not under a change.
	if want := "/a/changes/"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if gotBody["work_in_progress"] != true {
		t.Errorf("work_in_progress = %v, want true", gotBody["work_in_progress"])
	}

	if got.Number != 500 {
		t.Errorf("Number = %d, want 500", got.Number)
	}
}

func TestCreateChangeTrimsWhatItSends(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading request body: %v", readErr)
		}

		if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
			t.Errorf("decoding request body %q: %v", body, decodeErr)
		}

		if _, writeErr := w.Write([]byte(xssiPrefix + "\n" + `{"_number": 500}`)); writeErr != nil {
			t.Errorf("writing test response: %v", writeErr)
		}
	})

	in := ChangeInput{Project: " p ", Branch: " main ", Subject: " add the thing "}

	if _, err := client.CreateChange(t.Context(), in); err != nil {
		t.Fatalf("CreateChange() returned an unexpected error: %v", err)
	}

	for field, want := range map[string]string{
		"project": "p",
		"branch":  "main",
		"subject": "add the thing",
	} {
		if gotBody[field] != want {
			t.Errorf("%s = %v, want %q", field, gotBody[field], want)
		}
	}
}

func TestCreateChangeRequiresProjectBranchAndSubject(t *testing.T) {
	t.Parallel()

	tests := map[string]ChangeInput{
		"no project": {Branch: "main", Subject: "s"},
		"no branch":  {Project: "p", Subject: "s"},
		"no subject": {Project: "p", Branch: "main"},
		"all blank":  {Project: " ", Branch: " ", Subject: " "},
	}

	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("CreateChange() reached the server, want it to refuse before that")
			})

			if _, err := client.CreateChange(t.Context(), in); !errors.Is(err, ErrIncompleteChange) {
				t.Errorf("CreateChange() error = %v, want ErrIncompleteChange", err)
			}
		})
	}
}

func TestRevertSubmission(t *testing.T) {
	t.Parallel()

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		const reverts = `{"revert_changes": [
		  {"_number": 101, "project": "p", "branch": "main", "subject": "Revert one", "status": "NEW"},
		  {"_number": 102, "project": "q", "branch": "main", "subject": "Revert two", "status": "NEW"}
		]}`

		if _, err := w.Write([]byte(xssiPrefix + "\n" + reverts)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.RevertSubmission(t.Context(), "12345", "broke the build")
	if err != nil {
		t.Fatalf("RevertSubmission() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/12345/revert_submission"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	// One request produces several changes, across projects. Collapsing that
	// to a single result would hide reverts the caller now owns.
	if len(got.RevertChanges) != 2 {
		t.Fatalf("len(RevertChanges) = %d, want 2", len(got.RevertChanges))
	}

	if got.RevertChanges[1].Project != "q" {
		t.Errorf("second revert project = %q, want q", got.RevertChanges[1].Project)
	}
}
