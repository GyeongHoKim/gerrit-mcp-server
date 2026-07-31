package gerrit

import (
	"context"
	"errors"
)

// errStubbed is returned by unimplemented endpoints.
var errStubbed = errors.New("not implemented")

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
func (*Client) SetTopic(_ context.Context, _, _ string) error {
	return errStubbed
}
