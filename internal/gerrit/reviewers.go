package gerrit

import (
	"context"
	"errors"
)

// errStubbed is returned by unimplemented endpoints.
var errStubbed = errors.New("not implemented")

// GroupBaseInfo identifies a Gerrit group.
type GroupBaseInfo struct {
	// ID is the group's UUID.
	ID string `json:"id"`
	// Name is the group's display name.
	Name string `json:"name"`
}

// SuggestedReviewerInfo is one reviewer suggestion.
//
// Exactly one of Account and Group is set: Gerrit suggests both people and
// groups from the same endpoint.
type SuggestedReviewerInfo struct {
	// Account is the suggested person, when the suggestion is a person.
	Account *AccountInfo `json:"account,omitempty"`
	// Group is the suggested group, when the suggestion is a group.
	Group *GroupBaseInfo `json:"group,omitempty"`
	// Count is how many members the suggested group has.
	Count int `json:"count,omitempty"`
}

// SuggestReviewers asks Gerrit who could review a change.
//
// query is matched against names, emails and group names. A limit of zero
// leaves the server's default in place.
func (*Client) SuggestReviewers(_ context.Context, _, _ string, _ int) ([]SuggestedReviewerInfo, error) {
	return nil, errStubbed
}
