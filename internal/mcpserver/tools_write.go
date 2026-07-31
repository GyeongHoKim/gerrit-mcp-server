package mcpserver

import (
	"context"
	"fmt"
	"strings"

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

// deleteDraftCommentInput is the argument schema for the
// delete_draft_comment tool.
type deleteDraftCommentInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// DraftID identifies the staged comment.
	DraftID string `json:"draft_id" jsonschema:"id of the draft to discard, as returned by list_draft_comments"`
}

// registerDeleteDraftComment installs the delete_draft_comment tool.
func registerDeleteDraftComment(s *server, srv *mcp.Server) {
	destructive := true

	mcp.AddTool(srv, &mcp.Tool{
		Name: "delete_draft_comment",
		Description: "Discard one of your staged draft comments on a Gerrit change. " +
			"The draft is gone; this cannot be undone.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive},
	}, s.deleteDraftComment)
}

// deleteDraftComment discards one staged comment.
func (s *server) deleteDraftComment(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in deleteDraftCommentInput,
) (*mcp.CallToolResult, any, error) {
	if err := s.gerrit.DeleteDraftComment(ctx, in.ChangeID, in.DraftID); err != nil {
		return nil, nil, fmt.Errorf("discarding draft %s on %s: %w", in.DraftID, in.ChangeID, err)
	}

	return text(render.DraftDeleted(in.ChangeID, in.DraftID)), nil, nil
}

// registerDeleteDraftComments installs the delete_draft_comments tool.
func registerDeleteDraftComments(s *server, srv *mcp.Server) {
	destructive := true

	mcp.AddTool(srv, &mcp.Tool{
		Name: "delete_draft_comments",
		Description: "Discard every draft comment you have staged on a Gerrit change. " +
			"The drafts are gone; this cannot be undone.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive},
	}, s.deleteDraftComments)
}

// deleteDraftComments discards every staged comment on a change.
func (s *server) deleteDraftComments(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in changeIDInput,
) (*mcp.CallToolResult, any, error) {
	deleted, err := s.gerrit.DeleteAllDraftComments(ctx, in.ChangeID)
	if err != nil {
		// The count says how far it got. Reporting only the failure would
		// hide deletions that already happened and cannot be undone.
		return nil, nil, fmt.Errorf("discarding drafts on %s after removing %d: %w", in.ChangeID, deleted, err)
	}

	return text(render.DraftsDeleted(in.ChangeID, deleted)), nil, nil
}

// addReviewerInput is the argument schema for the add_reviewer tool.
type addReviewerInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// Reviewer names the account or group to add.
	Reviewer string `json:"reviewer" jsonschema:"account id, username, email or group name, as returned by suggest_reviewers"`
	// State selects whether they are expected to vote.
	State string `json:"state,omitempty" jsonschema:"REVIEWER for someone expected to vote (default) or CC to only keep them informed"`
	// Confirm acknowledges adding a large group.
	Confirm bool `json:"confirm,omitempty" jsonschema:"confirm adding a group large enough that Gerrit asks first"`
}

// registerAddReviewer installs the add_reviewer tool.
func registerAddReviewer(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_reviewer",
		Description: "Add a reviewer or CC to a Gerrit change. Accepts a person or a group.",
		Annotations: &mcp.ToolAnnotations{},
	}, s.addReviewer)
}

// addReviewer adds one reviewer or CC.
func (s *server) addReviewer(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in addReviewerInput,
) (*mcp.CallToolResult, any, error) {
	result, err := s.gerrit.AddReviewer(ctx, in.ChangeID, &gerrit.ReviewerInput{
		Reviewer:  in.Reviewer,
		State:     in.State,
		Confirmed: in.Confirm,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("adding %s to %s: %w", in.Reviewer, in.ChangeID, err)
	}

	return text(render.ReviewerAdded(in.ChangeID, result)), nil, nil
}

// setTopicInput is the argument schema for the set_topic tool.
type setTopicInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// Topic is the new topic.
	Topic string `json:"topic" jsonschema:"the new topic; pass an empty string to clear it"`
}

// registerSetTopic installs the set_topic tool.
func registerSetTopic(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_topic",
		Description: "Set the topic of a Gerrit change, or clear it by passing an empty topic.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.setTopic)
}

// setTopic sets or clears a change's topic.
func (s *server) setTopic(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in setTopicInput,
) (*mcp.CallToolResult, any, error) {
	topic := strings.TrimSpace(in.Topic)

	if err := s.gerrit.SetTopic(ctx, in.ChangeID, topic); err != nil {
		return nil, nil, fmt.Errorf("setting the topic on %s: %w", in.ChangeID, err)
	}

	return text(render.TopicSet(in.ChangeID, topic)), nil, nil
}

// changeMessageInput is the argument schema for the tools that flip a change's
// state with an optional note.
type changeMessageInput struct {
	// ChangeID identifies the change.
	ChangeID string `json:"change_id" jsonschema:"the change number (12345) or the triplet project~branch~Change-Id"`
	// Message is posted on the change alongside the action.
	Message string `json:"message,omitempty" jsonschema:"optional note posted on the change explaining the action"`
}

// registerSetReadyForReview installs the set_ready_for_review tool.
func registerSetReadyForReview(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_ready_for_review",
		Description: "Take a Gerrit change out of work-in-progress, which notifies its reviewers " +
			"that it is ready to look at.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.setReadyForReview)
}

// setReadyForReview marks a change ready.
func (s *server) setReadyForReview(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in changeMessageInput,
) (*mcp.CallToolResult, any, error) {
	if err := s.gerrit.SetReadyForReview(ctx, in.ChangeID, in.Message); err != nil {
		return nil, nil, fmt.Errorf("marking %s ready for review: %w", in.ChangeID, err)
	}

	return text(render.ReadyForReviewSet(in.ChangeID)), nil, nil
}

// registerSetWorkInProgress installs the set_work_in_progress tool.
func registerSetWorkInProgress(s *server, srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_work_in_progress",
		Description: "Mark a Gerrit change as work-in-progress, so it stops asking its reviewers " +
			"for attention.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.setWorkInProgress)
}

// setWorkInProgress marks a change work-in-progress.
func (s *server) setWorkInProgress(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in changeMessageInput,
) (*mcp.CallToolResult, any, error) {
	if err := s.gerrit.SetWorkInProgress(ctx, in.ChangeID, in.Message); err != nil {
		return nil, nil, fmt.Errorf("marking %s work-in-progress: %w", in.ChangeID, err)
	}

	return text(render.WorkInProgressSet(in.ChangeID)), nil, nil
}
