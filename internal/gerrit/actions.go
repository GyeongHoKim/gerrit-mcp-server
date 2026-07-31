package gerrit

import (
	"context"
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
