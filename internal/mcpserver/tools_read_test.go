package mcpserver

import (
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

// newServerAgainst builds a read-only server talking to a stub Gerrit.
func newServerAgainst(t *testing.T, handler http.HandlerFunc) *mcp.Server {
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
	}), false)
}

// callTool invokes a tool by name and returns its result.
func callTool(t *testing.T, srv *mcp.Server, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	result, err := connect(t, srv).CallTool(t.Context(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}

	return result
}

// resultText returns the text content of a tool result.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}

	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *mcp.TextContent", result.Content[0])
	}

	return content.Text
}

func TestQueryChangesToolIsRegisteredReadOnly(t *testing.T) {
	t.Parallel()

	srv := newServerAgainst(t, func(_ http.ResponseWriter, _ *http.Request) {})

	result, err := connect(t, srv).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	index := slices.IndexFunc(result.Tools, func(tool *mcp.Tool) bool {
		return tool.Name == "query_changes"
	})
	if index < 0 {
		t.Fatal("query_changes is not registered on a read-only server")
	}

	tool := result.Tools[index]

	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Error("query_changes is missing the read-only annotation")
	}

	if tool.Description == "" {
		t.Error("query_changes has no description for the model to read")
	}
}

func TestQueryChangesToolRendersMatches(t *testing.T) {
	t.Parallel()

	const body = `[
	  {
	    "project": "platform/base", "branch": "main",
	    "subject": "fix the widget alignment", "status": "NEW",
	    "updated": "2026-07-31 06:04:05.000000000",
	    "insertions": 42, "deletions": 3, "_number": 12345,
	    "owner": {"_account_id": 1000096, "name": "Alice Adams"}
	  }
	]`

	var gotQuery url.Values

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "query_changes", map[string]any{
		"query": "status:open owner:self",
		"limit": 25,
	})

	if result.IsError {
		t.Fatalf("query_changes reported an error: %s", resultText(t, result))
	}

	if want := "status:open owner:self"; gotQuery.Get("q") != want {
		t.Errorf("q = %q, want %q", gotQuery.Get("q"), want)
	}

	if want := "25"; gotQuery.Get("n") != want {
		t.Errorf("n = %q, want %q", gotQuery.Get("n"), want)
	}

	// The stub names its owner, which a real Gerrit only does when asked. Drop
	// the option and this test still passes while the tool reports account ids.
	if want := "DETAILED_ACCOUNTS"; gotQuery.Get("o") != want {
		t.Errorf("o = %q, want %q", gotQuery.Get("o"), want)
	}

	got := resultText(t, result)

	for _, want := range []string{"12345", "fix the widget alignment", "NEW", "Alice Adams"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}

	// The whole point of the render layer: no raw Gerrit JSON reaches the model.
	if strings.Contains(got, "_account_id") || strings.Contains(got, `"subject"`) {
		t.Errorf("output leaks raw Gerrit JSON:\n%s", got)
	}
}

func TestQueryChangesToolReportsGerritFailures(t *testing.T) {
	t.Parallel()

	srv := newServerAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	result := callTool(t, srv, "query_changes", map[string]any{"query": "status:open"})

	if !result.IsError {
		t.Fatal("query_changes succeeded, want the Gerrit refusal reported as a tool error")
	}

	// The model has to be able to tell why, so the message must survive.
	if got := resultText(t, result); !strings.Contains(strings.ToLower(got), "forbidden") {
		t.Errorf("error text = %q, want it to explain the refusal", got)
	}
}

func TestGetChangeDetailsTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "project": "platform/base", "branch": "main",
	  "change_id": "I8473b959", "subject": "fix the widget alignment",
	  "status": "NEW", "_number": 12345,
	  "total_comment_count": 5, "unresolved_comment_count": 2,
	  "owner": {"_account_id": 1, "name": "Alice Adams"},
	  "labels": {"Code-Review": {"all": [{"_account_id": 2, "name": "Bob Brown", "value": 2}]}}
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "get_change_details", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("get_change_details reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/detail"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	for _, want := range []string{"12345", "fix the widget alignment", "Code-Review: +2 Bob Brown", "2 unresolved"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}
}

func TestGetChangeDetailsToolIsReadOnly(t *testing.T) {
	t.Parallel()

	srv := newServerAgainst(t, func(_ http.ResponseWriter, _ *http.Request) {})

	result, err := connect(t, srv).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	index := slices.IndexFunc(result.Tools, func(tool *mcp.Tool) bool {
		return tool.Name == "get_change_details"
	})
	if index < 0 {
		t.Fatal("get_change_details is not registered on a read-only server")
	}

	if annotations := result.Tools[index].Annotations; annotations == nil || !annotations.ReadOnlyHint {
		t.Error("get_change_details is missing the read-only annotation")
	}
}

func TestGetCommitMessageTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "subject": "Add feature X",
	  "full_message": "Add feature X\n\nFeature X helps with foo.\n\nBug: 123\n",
	  "footers": {"Bug": "123"}
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "get_commit_message", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("get_commit_message reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/message"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	if !strings.Contains(got, "Feature X helps with foo.") {
		t.Errorf("output lost the message body:\n%s", got)
	}

	if strings.Contains(got, "full_message") {
		t.Errorf("output leaks raw Gerrit JSON:\n%s", got)
	}
}

func TestListChangeFilesTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "/COMMIT_MSG": {"status": "A", "lines_inserted": 9, "size_delta": 320, "size": 320},
	  "src/widget.go": {"lines_inserted": 42, "lines_deleted": 3, "size_delta": 900, "size": 4200}
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "list_change_files", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("list_change_files reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/revisions/current/files/"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	for _, want := range []string{"src/widget.go", "+42/-3", "/COMMIT_MSG"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "size_delta") {
		t.Errorf("output leaks raw Gerrit JSON:\n%s", got)
	}
}

func TestGetFileDiffTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "change_type": "MODIFIED",
	  "meta_a": {"name": "src/widget.go", "lines": 2},
	  "meta_b": {"name": "src/widget.go", "lines": 3},
	  "content": [
	    {"ab": ["package widget"]},
	    {"a": ["func old() {}"], "b": ["func new() {}"]}
	  ]
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "get_file_diff", map[string]any{
		"change_id": "12345",
		"file":      "src/widget.go",
	})

	if result.IsError {
		t.Fatalf("get_file_diff reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/revisions/current/files/src%2Fwidget.go/diff"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	for _, want := range []string{"src/widget.go", "- func old() {}", "+ func new() {}"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}
}

func TestListChangeCommentsTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "src/widget.go": [
	    {"id": "c2", "line": 42, "message": "extract this", "patch_set": 2, "unresolved": true,
	     "updated": "2026-07-30 09:00:00.000000000",
	     "author": {"_account_id": 3, "name": "Carol Chen"}}
	  ]
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "list_change_comments", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("list_change_comments reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/comments"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	for _, want := range []string{"src/widget.go", "extract this", "Carol Chen", "UNRESOLVED"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}
}

func TestListDraftCommentsTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "src/widget.go": [
	    {"id": "d1", "line": 10, "message": "still thinking", "patch_set": 3,
	     "updated": "2026-07-31 06:04:05.000000000"}
	  ]
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "list_draft_comments", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("list_draft_comments reported an error: %s", resultText(t, result))
	}

	// Drafts come from /drafts, not /comments. Getting this wrong would show
	// the model everyone's published review instead of its own notes.
	if want := "/a/changes/12345/drafts"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	if !strings.Contains(got, "draft comment") {
		t.Errorf("output does not say these are drafts:\n%s", got)
	}

	if !strings.Contains(got, "still thinking") {
		t.Errorf("output lost the draft body:\n%s", got)
	}
}

func TestChangesSubmittedTogetherTool(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n[]")); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "changes_submitted_together", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("changes_submitted_together reported an error: %s", resultText(t, result))
	}

	if want := "/a/changes/12345/submitted_together"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	// An empty list is an answer. Reporting it as "no results" would suggest
	// the query failed rather than that the change stands alone.
	if got := resultText(t, result); !strings.Contains(got, "by itself") {
		t.Errorf("output = %q, want it to say the change submits alone", got)
	}
}

func TestSuggestReviewersTool(t *testing.T) {
	t.Parallel()

	const body = `[
	  {"account": {"_account_id": 2, "name": "Bob Brown", "email": "bob@example.com"}},
	  {"group": {"id": "abc", "name": "reviewers-core"}, "count": 12}
	]`

	var gotQuery url.Values

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "suggest_reviewers", map[string]any{
		"change_id": "12345",
		"query":     "bo",
	})

	if result.IsError {
		t.Fatalf("suggest_reviewers reported an error: %s", resultText(t, result))
	}

	if want := "bo"; gotQuery.Get("q") != want {
		t.Errorf("q = %q, want %q", gotQuery.Get("q"), want)
	}

	got := resultText(t, result)

	for _, want := range []string{"Bob Brown", "bob@example.com", "reviewers-core", "12 members"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}
}

func TestGetBugsFromCLTool(t *testing.T) {
	t.Parallel()

	const body = `{
	  "subject": "Add feature X",
	  "full_message": "Add feature X\n\nBug: 123\nFixes: 456\nChange-Id: I1\n",
	  "footers": {"Bug": "123", "Fixes": "456", "Change-Id": "I1"}
	}`

	var gotPath string

	srv := newServerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(")]}'\n" + body)); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	})

	result := callTool(t, srv, "get_bugs_from_cl", map[string]any{"change_id": "12345"})

	if result.IsError {
		t.Fatalf("get_bugs_from_cl reported an error: %s", resultText(t, result))
	}

	// Gerrit parses the footers for us, so this rides on /message rather than
	// re-parsing the prose.
	if want := "/a/changes/12345/message"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	got := resultText(t, result)

	for _, want := range []string{"123", "456"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}

	// Change-Id is a footer too, and it is not an issue reference.
	if strings.Contains(got, "I1") {
		t.Errorf("output treats the Change-Id as a bug:\n%s", got)
	}
}
