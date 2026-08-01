package mcpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// newWritableServerAgainst builds a server with the write tools registered.
func newWritableServerAgainst(t *testing.T, handler http.HandlerFunc) *mcp.Server {
	t.Helper()

	stub := httptest.NewServer(handler)
	t.Cleanup(stub.Close)

	base, err := url.Parse(stub.URL)
	if err != nil {
		t.Fatalf("parsing stub url: %v", err)
	}

	return New(gerrit.New(gerrit.Options{
		BaseURL: base,
		User:    "alice",
		Token:   "s3cret",
		Timeout: 5 * time.Second,
	}), true)
}

func TestWriteToolsAreAbsentUntilWritesAreAllowed(t *testing.T) {
	t.Parallel()

	nothing := func(_ http.ResponseWriter, _ *http.Request) {}

	readOnly := toolNames(t, newServerAgainst(t, nothing))
	writable := toolNames(t, newWritableServerAgainst(t, nothing))

	// This is the guarantee GERRIT_ALLOW_WRITE exists to make: an agent
	// pointed at a read-only server cannot even see the mutating tools.
	if slices.Contains(readOnly, "post_review_comment") {
		t.Error("post_review_comment is exposed on a read-only server")
	}

	if !slices.Contains(writable, "post_review_comment") {
		t.Error("post_review_comment is missing once writes are allowed")
	}
}

func TestPostReviewCommentTool(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]any
	)

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()

		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading request body: %v", readErr)
		}

		if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
			t.Errorf("decoding request body %q: %v", body, decodeErr)
		}

		if _, writeErr := w.Write([]byte(")]}'\n" + `{"id": "d1", "line": 42, "message": "extract this", "patch_set": 3,
		  "updated": "2026-07-31 06:04:05.000000000"}`)); writeErr != nil {
			t.Errorf("writing stub response: %v", writeErr)
		}
	})

	result := callTool(t, srv, "post_review_comment", map[string]any{
		"change_id":  "12345",
		"file":       "src/widget.go",
		"line":       42,
		"message":    "extract this",
		"unresolved": true,
	})

	if result.IsError {
		t.Fatalf("post_review_comment reported an error: %s", resultText(t, result))
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}

	if want := "/a/changes/12345/revisions/current/drafts"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if gotBody["path"] != "src/widget.go" || gotBody["message"] != "extract this" {
		t.Errorf("body = %v, want the file and message forwarded", gotBody)
	}

	if gotBody["line"] != float64(42) {
		t.Errorf("line = %v, want 42", gotBody["line"])
	}

	if gotBody["unresolved"] != true {
		t.Errorf("unresolved = %v, want true", gotBody["unresolved"])
	}

	// The reply must make clear nothing is visible to reviewers yet.
	if got := resultText(t, result); !strings.Contains(strings.ToLower(got), "draft") {
		t.Errorf("output = %q, want it to say the comment is still a draft", got)
	}
}

func TestPostReviewCommentToolRejectsAnEmptyMessage(t *testing.T) {
	t.Parallel()

	srv := newWritableServerAgainst(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("post_review_comment reached the server, want it refused before that")
	})

	result := callTool(t, srv, "post_review_comment", map[string]any{
		"change_id": "12345",
		"file":      "src/widget.go",
		"message":   "   ",
	})

	if !result.IsError {
		t.Fatal("post_review_comment accepted an empty message")
	}

	// Asserting only IsError would be satisfied by any failure at all,
	// including one that never checked the message.
	if got := resultText(t, result); !strings.Contains(got, "message") {
		t.Errorf("error text = %q, want it to name the empty message", got)
	}
}

func TestPublishDraftsTool(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading request body: %v", readErr)
		}

		if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
			t.Errorf("decoding request body %q: %v", body, decodeErr)
		}

		if _, writeErr := w.Write([]byte(")]}'\n{}")); writeErr != nil {
			t.Errorf("writing stub response: %v", writeErr)
		}
	})

	result := callTool(t, srv, "publish_drafts", map[string]any{
		"change_id": "12345",
		"message":   "looks good",
	})

	if result.IsError {
		t.Fatalf("publish_drafts reported an error: %s", resultText(t, result))
	}

	// Omitting all_revisions must publish the current patch set only, not
	// quietly sweep up stale drafts from earlier ones.
	if gotBody["drafts"] != "PUBLISH" {
		t.Errorf("drafts = %v, want PUBLISH for the default scope", gotBody["drafts"])
	}

	if got := resultText(t, result); !strings.Contains(strings.ToLower(got), "published") {
		t.Errorf("output = %q, want it to confirm the comments are now visible", got)
	}
}

func TestPublishDraftsToolIsGatedOnWrites(t *testing.T) {
	t.Parallel()

	names := toolNames(t, newServerAgainst(t, func(_ http.ResponseWriter, _ *http.Request) {}))

	if slices.Contains(names, "publish_drafts") {
		t.Error("publish_drafts is exposed on a read-only server")
	}
}

func TestDeleteDraftCommentTool(t *testing.T) {
	t.Parallel()

	// The tool is handed an id and nothing else, so the client looks the draft
	// up to find the patch set it belongs to before deleting it there.
	const drafts = `{"src/widget.go": [{"id": "d1", "patch_set": 2}]}`

	var gotPath string

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if _, err := w.Write([]byte(")]}'\n" + drafts)); err != nil {
				t.Errorf("writing stub response: %v", err)
			}

			return
		}

		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	})

	result := callTool(t, srv, "delete_draft_comment", map[string]any{
		"change_id": "12345",
		"draft_id":  "d1",
	})

	if result.IsError {
		t.Fatalf("delete_draft_comment reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/revisions/2/drafts/d1"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestDestructiveToolsCarryTheDestructiveHint(t *testing.T) {
	t.Parallel()

	srv := newWritableServerAgainst(t, func(_ http.ResponseWriter, _ *http.Request) {})

	listed, err := connect(t, srv).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	// A client may warn or confirm before running a destructive tool, but only
	// if we tell it which ones those are. Every tool that discards work or
	// undoes a submission belongs here.
	destructive := []string{
		"abandon_change",
		"delete_draft_comment",
		"delete_draft_comments",
		"revert_change",
		"revert_submission",
	}

	for _, name := range destructive {
		index := slices.IndexFunc(listed.Tools, func(tool *mcp.Tool) bool { return tool.Name == name })
		if index < 0 {
			t.Errorf("%s is not registered on a writable server", name)

			continue
		}

		annotations := listed.Tools[index].Annotations
		if annotations == nil || annotations.DestructiveHint == nil || !*annotations.DestructiveHint {
			t.Errorf("%s is missing the destructive annotation", name)
		}
	}
}

func TestDeleteDraftCommentsTool(t *testing.T) {
	t.Parallel()

	const drafts = `{"src/widget.go": [{"id": "d1"}, {"id": "d2"}]}`

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if _, err := w.Write([]byte(")]}'\n" + drafts)); err != nil {
				t.Errorf("writing stub response: %v", err)
			}

			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	result := callTool(t, srv, "delete_draft_comments", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("delete_draft_comments reported an error: %s", resultText(t, result))
	}

	if got := resultText(t, result); !strings.Contains(got, "2") {
		t.Errorf("output = %q, want it to say how many drafts were discarded", got)
	}
}

func TestAddReviewerTool(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading request body: %v", readErr)
		}

		if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
			t.Errorf("decoding request body %q: %v", body, decodeErr)
		}

		const added = `{"ccs": [{"_account_id": 3, "name": "Carol Chen"}]}`

		if _, writeErr := w.Write([]byte(")]}'\n" + added)); writeErr != nil {
			t.Errorf("writing stub response: %v", writeErr)
		}
	})

	result := callTool(t, srv, "add_reviewer", map[string]any{
		"change_id": "12345",
		"reviewer":  "carol@example.com",
		"state":     "CC",
	})

	if result.IsError {
		t.Fatalf("add_reviewer reported an error: %s", resultText(t, result))
	}

	if gotBody["state"] != "CC" {
		t.Errorf("state = %v, want CC to be forwarded rather than defaulted away", gotBody["state"])
	}

	if got := resultText(t, result); !strings.Contains(got, "Carol Chen") {
		t.Errorf("output = %q, want it to name who was added", got)
	}
}

func TestSetTopicTool(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		topic      string
		wantMethod string
	}{
		"setting a topic puts it":  {topic: "cleanup", wantMethod: http.MethodPut},
		"clearing a topic deletes": {topic: "", wantMethod: http.MethodDelete},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotMethod string

			srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method

				if _, err := w.Write([]byte(")]}'\n\"cleanup\"")); err != nil {
					t.Errorf("writing stub response: %v", err)
				}
			})

			result := callTool(t, srv, "set_topic", map[string]any{
				"change_id": "12345",
				"topic":     test.topic,
			})

			if result.IsError {
				t.Fatalf("set_topic reported an error: %s", resultText(t, result))
			}

			if gotMethod != test.wantMethod {
				t.Errorf("method = %s, want %s", gotMethod, test.wantMethod)
			}
		})
	}
}

func TestReadyAndWorkInProgressTools(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		tool     string
		wantPath string
	}{
		"ready":            {tool: "set_ready_for_review", wantPath: "/a/changes/12345/ready"},
		"work in progress": {tool: "set_work_in_progress", wantPath: "/a/changes/12345/wip"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotPath string

			srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.EscapedPath()
				w.WriteHeader(http.StatusOK)
			})

			result := callTool(t, srv, test.tool, map[string]any{"change_id": "12345"})

			if result.IsError {
				t.Fatalf("%s reported an error: %s", test.tool, resultText(t, result))
			}

			if gotPath != test.wantPath {
				t.Errorf("path = %q, want %q", gotPath, test.wantPath)
			}
		})
	}
}

func TestRevertChangeToolNamesTheNewChange(t *testing.T) {
	t.Parallel()

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		const reverted = `{"_number": 99, "project": "p", "branch": "main",
		  "subject": "Revert \"first\"", "status": "NEW"}`

		if _, err := w.Write([]byte(")]}'\n" + reverted)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "revert_change", map[string]any{
		"change_id": "12345",
		"message":   "broke the build",
	})

	if result.IsError {
		t.Fatalf("revert_change reported an error: %s", resultText(t, result))
	}

	// The revert is a new change that still needs review. Reporting only
	// success would leave an agent believing the revert had landed.
	got := resultText(t, result)

	if !strings.Contains(got, "99") {
		t.Errorf("output = %q, want it to name the change created", got)
	}

	if !strings.Contains(strings.ToLower(got), "review") {
		t.Errorf("output = %q, want it to say the revert still needs review", got)
	}
}

func TestRevertSubmissionToolNamesEveryRevert(t *testing.T) {
	t.Parallel()

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		const reverts = `{"revert_changes": [
		  {"_number": 101, "project": "p", "branch": "main", "subject": "Revert one", "status": "NEW"},
		  {"_number": 102, "project": "q", "branch": "main", "subject": "Revert two", "status": "NEW"}
		]}`

		if _, err := w.Write([]byte(")]}'\n" + reverts)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "revert_submission", map[string]any{
		"change_id": "12345",
		"message":   "broke the build",
	})

	if result.IsError {
		t.Fatalf("revert_submission reported an error: %s", resultText(t, result))
	}

	// Every revert has to be named. An agent that sees only one will not
	// follow up on the others it just created.
	got := resultText(t, result)

	for _, want := range []string{"101", "102"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not name revert %s:\n%s", want, got)
		}
	}
}

func TestCreateChangeTool(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		const created = `{"_number": 500, "project": "p", "branch": "main",
		  "subject": "add the thing", "status": "NEW"}`

		if _, err := w.Write([]byte(")]}'\n" + created)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "create_change", map[string]any{
		"project": "p",
		"branch":  "main",
		"subject": "add the thing",
	})

	if result.IsError {
		t.Fatalf("create_change reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if got := resultText(t, result); !strings.Contains(got, "500") {
		t.Errorf("output = %q, want it to name the change created", got)
	}
}

func TestAbandonChangeTool(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]any
	)

	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()

		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading request body: %v", readErr)
		}

		if decodeErr := json.Unmarshal(body, &gotBody); decodeErr != nil {
			t.Errorf("decoding request body %q: %v", body, decodeErr)
		}

		if _, writeErr := w.Write([]byte(")]}'\n" + `{"_number": 12345, "project": "platform/base",
		  "branch": "main", "subject": "fix the widget alignment", "status": "ABANDONED"}`)); writeErr != nil {
			t.Errorf("writing stub response: %v", writeErr)
		}
	})

	result := callTool(t, srv, "abandon_change", map[string]any{
		"change_id": "12345",
		"message":   "superseded by 12400",
	})

	if result.IsError {
		t.Fatalf("abandon_change reported an error: %s", resultText(t, result))
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}

	if want := "/a/changes/12345/abandon"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if gotBody["message"] != "superseded by 12400" {
		t.Errorf("message = %v, want the note forwarded", gotBody["message"])
	}

	// The agent has to be able to tell which change it just stopped work on,
	// and the number alone is not what a person reading the session recognises.
	got := resultText(t, result)
	for _, want := range []string{"12345", "fix the widget alignment"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to name %q", got, want)
		}
	}
}

func TestAbandonChangeToolSurfacesAMergedChangeConflict(t *testing.T) {
	t.Parallel()

	// rest-api-changes.txt, Abandon Change: "If the change cannot be abandoned
	// because the change state doesn't allow abandoning of the change, the
	// response is 409 Conflict and the error message is contained in the
	// response body." Telling the agent only that it failed would leave it
	// retrying a merged change forever.
	srv := newWritableServerAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)

		if _, err := io.WriteString(w, "change is merged"); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "abandon_change", map[string]any{"change_id": "12345"})

	if !result.IsError {
		t.Fatalf("abandon_change succeeded on a merged change, want the refusal reported")
	}

	if got := resultText(t, result); !strings.Contains(got, "change is merged") {
		t.Errorf("output = %q, want Gerrit's explanation to reach the model", got)
	}
}
