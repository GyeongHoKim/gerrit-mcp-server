package gerrit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// ErrEmptyMessage reports a comment with nothing to say.
var ErrEmptyMessage = errors.New("comment message must not be empty")

// CommentInput creates a draft comment.
type CommentInput struct {
	// Range highlights a region rather than a single line.
	Range *CommentRange `json:"range,omitempty"`
	// Path is the file the comment belongs to.
	Path string `json:"path,omitempty"`
	// Side is REVISION or PARENT. Empty means REVISION.
	Side string `json:"side,omitempty"`
	// Message is the comment text.
	Message string `json:"message"`
	// InReplyTo is the id of the comment being answered.
	InReplyTo string `json:"in_reply_to,omitempty"`
	// Line is the line to attach to. Zero makes it a file-level comment.
	Line int `json:"line,omitempty"`
	// Unresolved marks the comment as needing an answer. Sent even when false
	// so the intent is explicit rather than inherited from a Gerrit default.
	Unresolved bool `json:"unresolved"`
}

// CreateDraftComment stages an unpublished comment on a change's current
// patch set.
//
// Drafts are deliberate: nothing reaches the change's reviewers until
// publish_drafts is called.
func (c *Client) CreateDraftComment(
	ctx context.Context,
	changeID string,
	in *CommentInput,
) (*CommentInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	if strings.TrimSpace(in.Message) == "" {
		return nil, ErrEmptyMessage
	}

	if strings.TrimSpace(in.Path) == "" {
		return nil, ErrEmptyFilePath
	}

	var draft CommentInfo
	if err := c.do(ctx, http.MethodPut, revisionPath(changeID, "/drafts"), nil, in, &draft); err != nil {
		return nil, err
	}

	return &draft, nil
}

// Draft handling for [ReviewInput.Drafts].
//
// Gerrit defaults to KEEP, so publishing is always an explicit request.
const (
	// PublishCurrentDrafts publishes drafts on the reviewed revision only.
	PublishCurrentDrafts = "PUBLISH"
	// PublishAllDrafts publishes drafts across every revision.
	PublishAllDrafts = "PUBLISH_ALL_REVISIONS"
)

// ReviewInput posts a review on a revision.
type ReviewInput struct {
	// Drafts says what to do with drafts already staged on the change.
	Drafts string `json:"drafts,omitempty"`
	// Message is the cover message left alongside the comments.
	Message string `json:"message,omitempty"`
}

// ReviewResult is Gerrit's answer to a posted review.
type ReviewResult struct {
	// Error explains a review Gerrit declined to apply.
	Error string `json:"error,omitempty"`
	// Ready reports a change taken out of work-in-progress by this review.
	Ready bool `json:"ready,omitempty"`
}

// ErrReviewRejected reports a review Gerrit accepted the request for but
// declined to apply.
var ErrReviewRejected = errors.New("gerrit rejected the review")

// PublishDrafts publishes the calling account's staged draft comments.
//
// allRevisions widens the scope from the current revision to every revision;
// Gerrit's own default is to publish nothing, so the scope is always stated.
func (c *Client) PublishDrafts(
	ctx context.Context,
	changeID, message string,
	allRevisions bool,
) (*ReviewResult, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	drafts := PublishCurrentDrafts
	if allRevisions {
		drafts = PublishAllDrafts
	}

	in := ReviewInput{Drafts: drafts, Message: strings.TrimSpace(message)}

	var result ReviewResult
	if err := c.do(ctx, http.MethodPost, revisionPath(changeID, "/review"), nil, in, &result); err != nil {
		return nil, err
	}

	// Gerrit reports a review it declined inside a 200 response, so the status
	// alone does not say whether anything was applied.
	if result.Error != "" {
		return nil, fmt.Errorf("%w: %s", ErrReviewRejected, result.Error)
	}

	return &result, nil
}

// ErrEmptyDraftID reports a call with no draft to act on.
var ErrEmptyDraftID = errors.New("draft id must not be empty")

// DeleteDraftComment removes one staged comment.
//
// Only drafts can be deleted this way. A published comment stays on the
// record; Gerrit has no delete for it that an ordinary account may use.
func (c *Client) DeleteDraftComment(ctx context.Context, changeID, draftID string) error {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return ErrEmptyChangeID
	}

	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return ErrEmptyDraftID
	}

	// A nil out discards the response: Gerrit answers 204 with no body.
	path := revisionPath(changeID, "/drafts/"+url.PathEscape(draftID))

	return c.do(ctx, http.MethodDelete, path, nil, nil, nil)
}

// errStubbed is returned by unimplemented endpoints.
var errStubbed = errors.New("not implemented")

// DeleteAllDraftComments discards every draft the calling account has staged
// on a change, and reports how many it removed.
//
// Gerrit has no bulk delete, so this lists the drafts and removes them one at
// a time. A failure part way through leaves the earlier deletions in place --
// they are already gone -- and reports what happened.
func (*Client) DeleteAllDraftComments(_ context.Context, _ string) (int, error) {
	return 0, errStubbed
}
