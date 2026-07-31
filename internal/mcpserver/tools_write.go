package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errStubbed is returned by unimplemented handlers.
var errStubbed = errors.New("not implemented")

// postReviewCommentInput is the argument schema for the post_review_comment
// tool.
type postReviewCommentInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// File is the path the comment belongs to.
	File string `json:"file" jsonschema:"path of the file to comment on, as returned by list_change_files"`
	// Message is the comment text.
	Message string `json:"message" jsonschema:"the comment text"`
	// InReplyTo answers an existing comment.
	InReplyTo string `json:"in_reply_to,omitempty" jsonschema:"id of the comment being answered, as returned by list_change_comments"`
	// Side selects which file the line refers to.
	Side string `json:"side,omitempty" jsonschema:"REVISION for the new file (default) or PARENT for the old one"`
	// Line is the line to attach to.
	Line int `json:"line,omitempty" jsonschema:"line number from get_file_diff; omit for a comment on the file as a whole"`
	// Unresolved marks the comment as needing an answer.
	Unresolved bool `json:"unresolved,omitempty" jsonschema:"mark the comment as needing a reply"`
}

// registerPostReviewComment installs the post_review_comment tool.
func registerPostReviewComment(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "post_review_comment",
		Description: "Stage a draft review comment on a line of a Gerrit change. " +
			"The comment stays unpublished until publish_drafts is called.",
		Annotations: &mcp.ToolAnnotations{},
	}, s.postReviewComment)
}

// postReviewComment stages a draft comment.
//
//nolint:gocritic // hugeParam: the SDK hands tool input to the handler by value
func (*server) postReviewComment(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ postReviewCommentInput,
) (*mcp.CallToolResult, any, error) {
	return nil, nil, errStubbed
}
