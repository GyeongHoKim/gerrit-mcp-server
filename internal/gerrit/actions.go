package gerrit

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// TopicInput sets a change's topic.
type TopicInput struct {
	// Topic is the new topic.
	Topic string `json:"topic"`
}

// SetTopic sets or clears a change's topic.
//
// An empty topic is a deletion, not an empty string: Gerrit has a separate
// verb for it, and PUTting "" would leave the change with a blank topic
// rather than none.
func (c *Client) SetTopic(ctx context.Context, changeID, topic string) error {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return ErrEmptyChangeID
	}

	path := changePath(changeID, "/topic")

	if topic = strings.TrimSpace(topic); topic == "" {
		return c.do(ctx, http.MethodDelete, path, nil, nil, nil)
	}

	return c.do(ctx, http.MethodPut, path, nil, TopicInput{Topic: topic}, nil)
}

// MessageInput carries the optional note Gerrit attaches to a state change.
type MessageInput struct {
	// Message is posted on the change alongside the action.
	Message string `json:"message,omitempty"`
}

// SetReadyForReview takes a change out of work-in-progress and notifies its
// reviewers.
func (c *Client) SetReadyForReview(ctx context.Context, changeID, message string) error {
	return c.postMessage(ctx, changeID, "/ready", message)
}

// SetWorkInProgress marks a change as not yet asking for review.
func (c *Client) SetWorkInProgress(ctx context.Context, changeID, message string) error {
	return c.postMessage(ctx, changeID, "/wip", message)
}

// postMessage posts an action that takes nothing but an optional note and
// answers with no body.
func (c *Client) postMessage(ctx context.Context, changeID, suffix, message string) error {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return ErrEmptyChangeID
	}

	in := MessageInput{Message: strings.TrimSpace(message)}

	return c.do(ctx, http.MethodPost, changePath(changeID, suffix), nil, in, nil)
}

// AbandonChange stops work on a change without merging it.
//
// Abandoning is reversible in Gerrit's UI but not through this server, which
// is why the tool that calls it is marked destructive.
func (c *Client) AbandonChange(ctx context.Context, changeID, message string) (*ChangeInfo, error) {
	return c.postForChange(ctx, changeID, "/abandon", message)
}

// RevertChange creates a new change that undoes a merged one.
func (c *Client) RevertChange(ctx context.Context, changeID, message string) (*ChangeInfo, error) {
	return c.postForChange(ctx, changeID, "/revert", message)
}

// postForChange posts an action that takes an optional note and answers with
// a ChangeInfo.
func (c *Client) postForChange(ctx context.Context, changeID, suffix, message string) (*ChangeInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	in := MessageInput{Message: strings.TrimSpace(message)}

	var change ChangeInfo
	if err := c.do(ctx, http.MethodPost, changePath(changeID, suffix), nil, in, &change); err != nil {
		return nil, err
	}

	return &change, nil
}

// ChangeInput creates a change.
type ChangeInput struct {
	// Project is the repository the change targets.
	Project string `json:"project"`
	// Branch is the target branch, without refs/heads/.
	Branch string `json:"branch"`
	// Subject is the first line of the commit message.
	Subject string `json:"subject"`
	// Topic groups the change with others.
	Topic string `json:"topic,omitempty"`
	// WorkInProgress starts the change without asking for review.
	WorkInProgress bool `json:"work_in_progress,omitempty"`
}

// ErrIncompleteChange reports a change missing something Gerrit requires.
var ErrIncompleteChange = errors.New("project, branch and subject are all required")

// CreateChange opens a new empty change.
//
// The change has no content until an edit is published to it; this only
// creates the review to hang that on.
// Taken by value: the struct is small, the trimming below has to happen on a
// copy so the caller gets its own back as it was, and a value parameter is
// that copy without a nil to dereference.
func (c *Client) CreateChange(ctx context.Context, in ChangeInput) (*ChangeInfo, error) {
	in.Project = strings.TrimSpace(in.Project)
	in.Branch = strings.TrimSpace(in.Branch)
	in.Subject = strings.TrimSpace(in.Subject)

	if in.Project == "" || in.Branch == "" || in.Subject == "" {
		return nil, ErrIncompleteChange
	}

	var change ChangeInfo
	if err := c.do(ctx, http.MethodPost, "/changes/", nil, in, &change); err != nil {
		return nil, err
	}

	return &change, nil
}

// RevertSubmissionInfo names the changes created to undo a submission.
type RevertSubmissionInfo struct {
	// RevertChanges are the reverts, one per change in the submission.
	RevertChanges []ChangeInfo `json:"revert_changes"`
}

// RevertSubmission creates a change reverting every change submitted
// alongside this one.
func (c *Client) RevertSubmission(
	ctx context.Context,
	changeID, message string,
) (*RevertSubmissionInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	in := MessageInput{Message: strings.TrimSpace(message)}

	var reverts RevertSubmissionInfo
	if err := c.do(ctx, http.MethodPost, changePath(changeID, "/revert_submission"), nil, in, &reverts); err != nil {
		return nil, err
	}

	return &reverts, nil
}
