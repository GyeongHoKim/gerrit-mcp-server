package gerrit

import (
	"context"
	"errors"
)

// errStubbed is returned by unimplemented endpoints.
var errStubbed = errors.New("not implemented")

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
//
//nolint:unused // the implementation commit routes through it
func revisionPath(changeID, suffix string) string {
	return changePath(changeID, "/revisions/current"+suffix)
}

// ListFiles lists the files touched by a change's current patch set, keyed by
// path.
//
// The result includes Gerrit's pseudo-entries such as /COMMIT_MSG, which are
// what a reviewer sees in the UI.
func (*Client) ListFiles(_ context.Context, _ string) (map[string]FileInfo, error) {
	return nil, errStubbed
}
