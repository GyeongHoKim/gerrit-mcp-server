package mcpserver

import (
	"context"
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
