package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

// queryChangesInput is the argument schema for the query_changes tool.
type queryChangesInput struct {
	// Query is a Gerrit search expression.
	Query string `json:"query" jsonschema:"Gerrit search query, for example 'status:open owner:self' or 'project:platform/base is:open'"`
	// Limit caps the number of results.
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of changes to return; leave unset for the server default"`
}

// registerQueryChanges installs the query_changes tool.
func registerQueryChanges(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_changes",
		Description: "Search Gerrit changes with Gerrit query syntax and return a compact summary of each match.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.queryChanges)
}

// queryChanges searches for changes and renders the matches.
func (s *server) queryChanges(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in queryChangesInput,
) (*mcp.CallToolResult, any, error) {
	changes, err := s.gerrit.QueryChanges(ctx, in.Query, in.Limit)
	if err != nil {
		// The SDK turns a returned error into an IsError result, so the model
		// sees why Gerrit refused instead of the session dying.
		return nil, nil, fmt.Errorf("querying changes: %w", err)
	}

	return text(render.Changes(changes)), nil, nil
}

// changeIDInput is the argument schema for tools that act on one change.
type changeIDInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
}

// registerGetChangeDetails installs the get_change_details tool.
func registerGetChangeDetails(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_change_details",
		Description: "Retrieve one Gerrit change with its status, labels, reviewers and comment counts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.getChangeDetails)
}

// getChangeDetails retrieves one change and renders its review state.
func (s *server) getChangeDetails(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in changeIDInput,
) (*mcp.CallToolResult, any, error) {
	detail, err := s.gerrit.GetChange(ctx, in.ChangeID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching change %s: %w", in.ChangeID, err)
	}

	return text(render.ChangeDetail(detail)), nil, nil
}

// registerGetCommitMessage installs the get_commit_message tool.
func registerGetCommitMessage(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_commit_message",
		Description: "Retrieve the full commit message of a Gerrit change's current patch set.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.getCommitMessage)
}

// getCommitMessage retrieves the commit message of a change.
func (s *server) getCommitMessage(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in changeIDInput,
) (*mcp.CallToolResult, any, error) {
	message, err := s.gerrit.GetCommitMessage(ctx, in.ChangeID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching commit message for %s: %w", in.ChangeID, err)
	}

	return text(render.CommitMessage(message)), nil, nil
}

// registerListChangeFiles installs the list_change_files tool.
func registerListChangeFiles(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_change_files",
		Description: "List the files modified by a Gerrit change's current patch set, with per-file line counts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.listChangeFiles)
}

// listChangeFiles lists the files a change touches.
func (s *server) listChangeFiles(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in changeIDInput,
) (*mcp.CallToolResult, any, error) {
	files, err := s.gerrit.ListFiles(ctx, in.ChangeID)
	if err != nil {
		return nil, nil, fmt.Errorf("listing files for %s: %w", in.ChangeID, err)
	}

	return text(render.Files(files)), nil, nil
}

// fileDiffInput is the argument schema for the get_file_diff tool.
type fileDiffInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// File is the path within the change.
	File string `json:"file" jsonschema:"path of the file within the change, as returned by list_change_files"`
}

// registerGetFileDiff installs the get_file_diff tool.
func registerGetFileDiff(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_file_diff",
		Description: "Retrieve the diff of a single file in a Gerrit change's current patch set.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.getFileDiff)
}

// getFileDiff retrieves and renders one file's diff.
func (*server) getFileDiff(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ fileDiffInput,
) (*mcp.CallToolResult, any, error) {
	return nil, nil, errStubbed
}

// errStubbed is returned by unimplemented handlers.
var errStubbed = errors.New("not implemented")
