package gerrit

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// ErrEmptyFilePath reports a call with no file to act on.
var ErrEmptyFilePath = errors.New("file path must not be empty")

// FileInfo describes one file in a patch set.
type FileInfo struct {
	// Status is A, D, R, C or W. Gerrit omits it for a plain modification.
	Status string `json:"status,omitempty"`
	// OldPath is the previous path of a renamed or copied file.
	OldPath string `json:"old_path,omitempty"`
	// LinesInserted counts added lines.
	LinesInserted int `json:"lines_inserted,omitempty"`
	// LinesDeleted counts removed lines.
	LinesDeleted int `json:"lines_deleted,omitempty"`
	// SizeDelta is the change in file size, in bytes.
	SizeDelta int `json:"size_delta"`
	// Size is the resulting file size, in bytes.
	Size int `json:"size"`
	// Binary reports a file Gerrit will not diff as text.
	Binary bool `json:"binary,omitempty"`
}

// revisionPath returns a path under the current revision of a change.
//
// Everything uses the literal revision "current" rather than resolving a patch
// set first: Gerrit accepts it, and it saves a round trip on every call.
func revisionPath(changeID, suffix string) string {
	return changePath(changeID, "/revisions/current"+suffix)
}

// ListFiles lists the files touched by a change's current patch set, keyed by
// path.
//
// The result includes Gerrit's pseudo-entries such as /COMMIT_MSG, which are
// what a reviewer sees in the UI.
func (c *Client) ListFiles(ctx context.Context, changeID string) (map[string]FileInfo, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return nil, ErrEmptyChangeID
	}

	files := map[string]FileInfo{}
	if err := c.do(ctx, http.MethodGet, revisionPath(changeID, "/files/"), nil, nil, &files); err != nil {
		return nil, err
	}

	return files, nil
}

// errStubbed is returned by unimplemented endpoints.
var errStubbed = errors.New("not implemented")

// DiffFileMeta describes one side of a diff.
type DiffFileMeta struct {
	// Name is the file path on this side.
	Name string `json:"name"`
	// ContentType is the detected MIME type.
	ContentType string `json:"content_type"`
	// Lines is the number of lines on this side.
	Lines int `json:"lines"`
}

// DiffContent is one region of a diff.
//
// Exactly one shape is populated per region: AB for context, A and B for a
// replacement, or Skip for a stretch Gerrit chose not to send.
type DiffContent struct {
	// A holds lines present only on the old side.
	A []string `json:"a,omitempty"`
	// B holds lines present only on the new side.
	B []string `json:"b,omitempty"`
	// AB holds lines common to both sides.
	AB []string `json:"ab,omitempty"`
	// Skip counts lines omitted from both sides.
	Skip int `json:"skip,omitempty"`
	// Common reports a region that is identical on both sides.
	Common bool `json:"common,omitempty"`
}

// DiffInfo is the diff of one file in a patch set.
type DiffInfo struct {
	// MetaA describes the old side, absent when the file was added.
	MetaA *DiffFileMeta `json:"meta_a,omitempty"`
	// MetaB describes the new side, absent when the file was deleted.
	MetaB *DiffFileMeta `json:"meta_b,omitempty"`
	// ChangeType is ADDED, MODIFIED, DELETED, RENAMED, COPIED or REWRITE.
	ChangeType string `json:"change_type"`
	// DiffHeader is the raw patch header Gerrit produced.
	DiffHeader []string `json:"diff_header,omitempty"`
	// Content is the diff itself, in order.
	Content []DiffContent `json:"content"`
	// Binary reports a file Gerrit will not diff as text.
	Binary bool `json:"binary,omitempty"`
}

// GetFileDiff retrieves the diff of one file in a change's current patch set.
func (*Client) GetFileDiff(_ context.Context, _, _ string) (*DiffInfo, error) {
	return nil, errStubbed
}
