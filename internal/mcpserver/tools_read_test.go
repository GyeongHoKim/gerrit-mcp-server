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

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
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

	return New(gerrit.New(config.Config{
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
