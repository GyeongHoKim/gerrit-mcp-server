package gerrit

import (
	"context"
	"errors"
	"fmt"
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

// Reviewer states for [ReviewerInput.State].
const (
	// StateReviewer adds someone who is expected to vote.
	StateReviewer = "REVIEWER"
	// StateCC adds someone who is only kept informed.
	StateCC = "CC"
)

// ReviewerInput adds one reviewer or CC to a change.
type ReviewerInput struct {
	// Reviewer is an account id, username, email or group name.
	Reviewer string `json:"reviewer"`
	// State is REVIEWER or CC. Empty means REVIEWER.
	State string `json:"state,omitempty"`
	// Confirmed acknowledges adding a group large enough that Gerrit asks
	// first.
	Confirmed bool `json:"confirmed,omitempty"`
}

// ReviewerResult is Gerrit's answer to adding a reviewer.
type ReviewerResult struct {
	// Error explains a reviewer Gerrit declined to add.
	Error string `json:"error,omitempty"`
	// Reviewers are the accounts added as reviewers.
	Reviewers []AccountInfo `json:"reviewers,omitempty"`
	// CCs are the accounts added as CC.
	CCs []AccountInfo `json:"ccs,omitempty"`
	// Confirm reports a group large enough that Gerrit wants confirmation
	// before adding everyone in it.
	Confirm bool `json:"confirm,omitempty"`
}

// ErrReviewerRejected reports a reviewer Gerrit declined to add.
var ErrReviewerRejected = errors.New("gerrit rejected the reviewer")

// ErrConfirmationRequired reports a group Gerrit will only add once the
// caller says it means to add everyone in it.
var ErrConfirmationRequired = errors.New("adding this group needs confirmation")

// ErrEmptyReviewer reports a call with nobody to add.
var ErrEmptyReviewer = errors.New("reviewer must not be empty")

// AddReviewer adds one reviewer or CC to a change.
//
// in is taken by value: it is three small fields, and a pointer here would buy
// nothing but a nil to dereference.
func (c *Client) AddReviewer(
	ctx context.Context,
	changeID string,
	in ReviewerInput,
) (*ReviewerResult, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	if strings.TrimSpace(in.Reviewer) == "" {
		return nil, ErrEmptyReviewer
	}

	var result ReviewerResult
	if err := c.do(ctx, http.MethodPost, changePath(changeID, "/reviewers"), nil, in, &result); err != nil {
		return nil, err
	}

	// Gerrit reports both outcomes inside a 200, and they are not the same
	// thing: a confirmation can be retried, a refusal cannot.
	if result.Confirm {
		return nil, fmt.Errorf("%w: %s", ErrConfirmationRequired, result.Error)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("%w: %s", ErrReviewerRejected, result.Error)
	}

	return &result, nil
}
