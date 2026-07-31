package gerrit

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

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
func (c *Client) SuggestReviewers(
	ctx context.Context,
	changeID, query string,
	limit int,
) ([]SuggestedReviewerInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	// An empty q is not the same as no q: Gerrit ranks differently for each,
	// so a blank one is left off rather than sent through.
	values := url.Values{}
	if query = strings.TrimSpace(query); query != "" {
		values.Set("q", query)
	}

	if limit > 0 {
		values.Set("n", strconv.Itoa(limit))
	}

	var suggestions []SuggestedReviewerInfo

	path := changePath(changeID, "/suggest_reviewers")
	if err := c.do(ctx, http.MethodGet, path, values, nil, &suggestions); err != nil {
		return nil, err
	}

	return suggestions, nil
}
