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

// errStubbed is returned by unimplemented endpoints.
var errStubbed = errors.New("not implemented")

// MessageInput carries the optional note Gerrit attaches to a state change.
type MessageInput struct {
	// Message is posted on the change alongside the action.
	Message string `json:"message,omitempty"`
}

// SetReadyForReview takes a change out of work-in-progress and notifies its
// reviewers.
func (*Client) SetReadyForReview(_ context.Context, _, _ string) error {
	return errStubbed
}

// SetWorkInProgress marks a change as not yet asking for review.
func (*Client) SetWorkInProgress(_ context.Context, _, _ string) error {
	return errStubbed
}
