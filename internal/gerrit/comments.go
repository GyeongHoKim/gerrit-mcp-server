package gerrit

import (
	"context"
	"net/http"
	"strings"
)

// CommentRange is a character range within a file.
type CommentRange struct {
	// StartLine is the first line of the range, counting from one.
	StartLine int `json:"start_line"`
	// StartCharacter is the offset in the start line, counting from zero.
	StartCharacter int `json:"start_character"`
	// EndLine is the last line of the range, counting from one.
	EndLine int `json:"end_line"`
	// EndCharacter is the offset in the end line, counting from zero.
	EndCharacter int `json:"end_character"`
}

// CommentInfo is one review comment, inline or on the file as a whole.
type CommentInfo struct {
	// Author is who wrote it. Gerrit omits it on your own drafts.
	Author *AccountInfo `json:"author,omitempty"`
	// Range is the highlighted region, when the comment has one.
	Range *CommentRange `json:"range,omitempty"`
	// Updated is when the comment last changed. Placed among the pointers so
	// the struct's scannable prefix stays short; the JSON is keyed by name
	// regardless.
	Updated Timestamp `json:"updated"`
	// ID is the comment's UUID, needed to reply to or delete it.
	ID string `json:"id"`
	// Message is the comment text.
	Message string `json:"message,omitempty"`
	// InReplyTo is the id of the comment this one answers.
	InReplyTo string `json:"in_reply_to,omitempty"`
	// Side is REVISION or PARENT.
	Side string `json:"side,omitempty"`
	// PatchSet is the patch set the comment was left on.
	PatchSet int `json:"patch_set,omitempty"`
	// Line is the line number, absent for a file-level comment.
	Line int `json:"line,omitempty"`
	// Unresolved reports a comment still marked as needing an answer.
	Unresolved bool `json:"unresolved,omitempty"`
}

// ListComments retrieves the published comments on a change, keyed by file
// path.
func (c *Client) ListComments(ctx context.Context, changeID string) (map[string][]CommentInfo, error) {
	return c.comments(ctx, changeID, "/comments")
}

// ListDraftComments retrieves the calling account's unpublished draft
// comments on a change, keyed by file path.
func (c *Client) ListDraftComments(ctx context.Context, changeID string) (map[string][]CommentInfo, error) {
	return c.comments(ctx, changeID, "/drafts")
}

// comments fetches a comment collection keyed by file path.
func (c *Client) comments(ctx context.Context, changeID, suffix string) (map[string][]CommentInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	byFile := map[string][]CommentInfo{}
	if err := c.do(ctx, http.MethodGet, changePath(changeID, suffix), nil, nil, &byFile); err != nil {
		return nil, err
	}

	return byFile, nil
}
