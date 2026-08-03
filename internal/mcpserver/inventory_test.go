package mcpserver

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The tool names this server promises, sorted -- which is how toolNames hands
// them back, not the order ListTools happens to use.
//
// This is a contract rather than a discovery: the names appear in README.md
// and in users' MCP client configs, so renaming one is a breaking change and
// should have to be written down here first.
var (
	wantReadTools = []string{
		"changes_submitted_together",
		"get_bugs_from_cl",
		"get_change_details",
		"get_commit_message",
		"get_file_diff",
		"list_change_comments",
		"list_change_files",
		"list_draft_comments",
		"query_changes",
		"suggest_reviewers",
	}

	wantWriteTools = []string{
		"abandon_change",
		"add_reviewer",
		"create_change",
		"delete_draft_comment",
		"delete_draft_comments",
		"post_review_comment",
		"publish_drafts",
		"revert_change",
		"revert_submission",
		"set_ready_for_review",
		"set_topic",
		"set_work_in_progress",
	}
)

func TestReadOnlyInventory(t *testing.T) {
	t.Parallel()

	if diff := cmp.Diff(wantReadTools, toolNames(t, New(newGerrit(t), false))); diff != "" {
		t.Errorf("read-only tool inventory mismatch (-want +got):\n%s", diff)
	}
}

func TestWritableInventory(t *testing.T) {
	t.Parallel()

	want := slices.Concat(wantReadTools, wantWriteTools)
	slices.Sort(want)

	if diff := cmp.Diff(want, toolNames(t, New(newGerrit(t), true))); diff != "" {
		t.Errorf("writable tool inventory mismatch (-want +got):\n%s", diff)
	}
}

func TestEveryToolDescribesItself(t *testing.T) {
	t.Parallel()

	listed, err := connect(t, New(newGerrit(t), true)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	for _, tool := range listed.Tools {
		// The description is the only thing a model reads before deciding
		// whether a tool is the one it wants.
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}

		if tool.InputSchema == nil {
			t.Errorf("%s has no input schema", tool.Name)
		}
	}
}

func TestReadToolsAreAnnotatedReadOnly(t *testing.T) {
	t.Parallel()

	listed, err := connect(t, New(newGerrit(t), true)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	for _, tool := range listed.Tools {
		if !slices.Contains(wantReadTools, tool.Name) {
			continue
		}

		// A client may run read-only tools without asking. It can only do that
		// if every one of them says so.
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is a read tool but carries no read-only annotation", tool.Name)
		}
	}
}

func TestWriteToolsAreNotAnnotatedReadOnly(t *testing.T) {
	t.Parallel()

	listed, err := connect(t, New(newGerrit(t), true)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	for _, tool := range listed.Tools {
		if !slices.Contains(wantWriteTools, tool.Name) {
			continue
		}

		// The complement of TestReadToolsAreAnnotatedReadOnly, and the
		// direction that actually costs something: the read registrars all
		// carry this annotation as a literal, so copy-pasting one onto a write
		// registrar is a plausible edit. Nothing else would catch it -- the
		// tool would still be gated on GERRIT_ALLOW_WRITE and still carry its
		// destructive hint -- and a client would run it without asking.
		if tool.Annotations != nil && tool.Annotations.ReadOnlyHint {
			t.Errorf("%s mutates Gerrit but is annotated read-only", tool.Name)
		}
	}
}
