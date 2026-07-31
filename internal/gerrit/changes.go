package gerrit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// timestampLayout is Gerrit's timestamp format. It is deliberately not RFC
// 3339: Gerrit sends "yyyy-mm-dd hh:mm:ss.fffffffff", always in UTC.
const timestampLayout = "2006-01-02 15:04:05.000000000"

// Timestamp is a point in time as Gerrit reports it.
type Timestamp struct {
	time.Time
}

// UnmarshalJSON decodes Gerrit's timestamp format. An empty string decodes to
// the zero time, which is how optional timestamps arrive.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decoding gerrit timestamp: %w", err)
	}

	if raw == "" {
		t.Time = time.Time{}

		return nil
	}

	parsed, err := time.Parse(timestampLayout, raw)
	if err != nil {
		return fmt.Errorf("parsing gerrit timestamp %q: %w", raw, err)
	}

	t.Time = parsed

	return nil
}

// AccountInfo identifies a Gerrit user.
type AccountInfo struct {
	// Name is the user's full name.
	Name string `json:"name,omitempty"`
	// Email is the user's registered address.
	Email string `json:"email,omitempty"`
	// Username is the login name.
	Username string `json:"username,omitempty"`
	// AccountID is the numeric account id.
	AccountID int `json:"_account_id"`
}

// Display returns the most human-friendly identifier available.
func (a AccountInfo) Display() string {
	switch {
	case a.Name != "":
		return a.Name
	case a.Username != "":
		return a.Username
	case a.Email != "":
		return a.Email
	default:
		return strconv.Itoa(a.AccountID)
	}
}

// ChangeInfo is the part of Gerrit's ChangeInfo the tools actually render.
//
// Gerrit's own struct has upwards of forty fields; carrying only what is shown
// keeps decoding cheap and makes it obvious what the model gets to see.
type ChangeInfo struct {
	// ID is Gerrit's own id for the change: "project~<number>", with the
	// project already URL encoded. The triplet project~branch~Change-Id is a
	// separate field, triplet_id.
	ID string `json:"id"`
	// Project is the repository the change targets.
	Project string `json:"project"`
	// Branch is the target branch, without refs/heads/.
	Branch string `json:"branch"`
	// Topic groups related changes, and is often empty.
	Topic string `json:"topic,omitempty"`
	// Subject is the first line of the commit message.
	Subject string `json:"subject"`
	// Status is NEW, MERGED or ABANDONED.
	Status string `json:"status"`
	// Updated is when the change last changed.
	Updated Timestamp `json:"updated"`
	// Owner is the account that created the change.
	Owner AccountInfo `json:"owner"`
	// Number is the change number shown in the UI.
	Number int `json:"_number"`
	// Insertions is the number of added lines.
	Insertions int `json:"insertions"`
	// Deletions is the number of removed lines.
	Deletions int `json:"deletions"`
	// WorkInProgress reports a change its owner is not asking review on.
	WorkInProgress bool `json:"work_in_progress"`
	// IsPrivate reports a change visible only to its owner and reviewers.
	IsPrivate bool `json:"is_private"`
	// MoreChanges is set by Gerrit on the final entry of a result set it cut
	// short. Without it a truncated answer is indistinguishable from a
	// complete one.
	MoreChanges bool `json:"_more_changes,omitempty"`
}

// ErrEmptyQuery reports a search with nothing to search for.
var ErrEmptyQuery = errors.New("query must not be empty")

// QueryChanges searches for changes matching a Gerrit query such as
// "status:open owner:self".
//
// A limit of zero leaves the server's default in place.
func (c *Client) QueryChanges(ctx context.Context, query string, limit int) ([]ChangeInfo, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}

	values := url.Values{"q": {query}}
	if limit > 0 {
		values.Set("n", strconv.Itoa(limit))
	}

	var changes []ChangeInfo
	if err := c.do(ctx, http.MethodGet, "/changes/", values, nil, &changes); err != nil {
		return nil, err
	}

	return changes, nil
}

// ApprovalInfo is one account's vote on a label.
type ApprovalInfo struct {
	AccountInfo

	// Value is the vote, for example 2 for a Code-Review +2.
	Value int `json:"value,omitempty"`
}

// LabelInfo is the vote state of one label on a change.
type LabelInfo struct {
	// Approved is a user who gave the label its maximum value.
	Approved *AccountInfo `json:"approved,omitempty"`
	// Rejected is a user who gave the label its minimum value.
	Rejected *AccountInfo `json:"rejected,omitempty"`
	// Recommended is a user who voted positively but not the maximum.
	Recommended *AccountInfo `json:"recommended,omitempty"`
	// Disliked is a user who voted negatively but not the minimum.
	Disliked *AccountInfo `json:"disliked,omitempty"`
	// All is every vote cast on this label.
	All []ApprovalInfo `json:"all,omitempty"`
	// Optional reports a label that does not block submission.
	Optional bool `json:"optional,omitempty"`
	// Blocking reports a label whose current state blocks submission.
	Blocking bool `json:"blocking,omitempty"`
}

// ChangeDetail is a change with the review state Gerrit only returns from the
// detail endpoint.
type ChangeDetail struct {
	// Labels is the vote state, keyed by label name.
	Labels map[string]LabelInfo `json:"labels,omitempty"`
	// Reviewers is keyed by state: REVIEWER, CC or REMOVED.
	Reviewers map[string][]AccountInfo `json:"reviewers,omitempty"`
	// ChangeID is the Change-Id footer from the commit message.
	ChangeID string `json:"change_id"`
	// Embedded after the pointer-bearing fields so the struct's scannable
	// prefix stays short. Field order has no bearing on the JSON, which is
	// keyed by name.
	ChangeInfo

	// TotalCommentCount counts every comment on the change.
	TotalCommentCount int `json:"total_comment_count"`
	// UnresolvedCommentCount counts the comments still marked unresolved.
	UnresolvedCommentCount int `json:"unresolved_comment_count"`
}

// ErrEmptyChangeID reports a call with no change to act on.
var ErrEmptyChangeID = errors.New("change id must not be empty")

// GetChange retrieves one change with its labels, reviewers and comment
// counts.
func (c *Client) GetChange(ctx context.Context, changeID string) (*ChangeDetail, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	var detail ChangeDetail
	if err := c.do(ctx, http.MethodGet, changePath(changeID, "/detail"), nil, nil, &detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

// CommitMessageInfo is a change's commit message with its footers parsed out.
type CommitMessageInfo struct {
	// Footers maps footer keys such as Bug or Change-Id to their values.
	Footers map[string]string `json:"footers,omitempty"`
	// Subject is the first line of the message.
	Subject string `json:"subject"`
	// FullMessage is the complete commit message, subject included.
	FullMessage string `json:"full_message"`
}

// GetCommitMessage retrieves the commit message of a change's current patch
// set.
func (c *Client) GetCommitMessage(ctx context.Context, changeID string) (*CommitMessageInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	var message CommitMessageInfo
	if err := c.do(ctx, http.MethodGet, changePath(changeID, "/message"), nil, nil, &message); err != nil {
		return nil, err
	}

	return &message, nil
}
