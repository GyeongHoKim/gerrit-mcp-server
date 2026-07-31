package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

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
func (s *server) postReviewComment(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in postReviewCommentInput,
) (*mcp.CallToolResult, any, error) {
	draft, err := s.gerrit.CreateDraftComment(ctx, in.ChangeID, &gerrit.CommentInput{
		Path:       in.File,
		Side:       in.Side,
		Message:    in.Message,
		InReplyTo:  in.InReplyTo,
		Line:       in.Line,
		Unresolved: in.Unresolved,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("staging a draft comment on %s: %w", in.ChangeID, err)
	}

	return text(render.DraftCreated(draft)), nil, nil
}

// publishDraftsInput is the argument schema for the publish_drafts tool.
type publishDraftsInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// Message is the cover message posted with the comments.
	Message string `json:"message,omitempty" jsonschema:"cover message posted alongside the comments"`
	// AllRevisions widens the scope beyond the current patch set.
	AllRevisions bool `json:"all_revisions,omitempty" jsonschema:"also publish drafts left on earlier patch sets; defaults to the current one only"`
}

// registerPublishDrafts installs the publish_drafts tool.
func registerPublishDrafts(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "publish_drafts",
		Description: "Publish your staged draft comments on a Gerrit change, making them visible to reviewers. " +
			"This cannot be undone.",
		Annotations: &mcp.ToolAnnotations{},
	}, s.publishDrafts)
}

// publishDrafts publishes the staged comments.
func (s *server) publishDrafts(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in publishDraftsInput,
) (*mcp.CallToolResult, any, error) {
	if _, err := s.gerrit.PublishDrafts(ctx, in.ChangeID, in.Message, in.AllRevisions); err != nil {
		return nil, nil, fmt.Errorf("publishing drafts on %s: %w", in.ChangeID, err)
	}

	return text(render.DraftsPublished(in.ChangeID, in.AllRevisions)), nil, nil
}
