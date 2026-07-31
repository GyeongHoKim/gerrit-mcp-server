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
func (*server) getChangeDetails(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ changeIDInput,
) (*mcp.CallToolResult, any, error) {
	return nil, nil, errStubbed
}

// errStubbed is returned by unimplemented handlers.
var errStubbed = errors.New("not implemented")
