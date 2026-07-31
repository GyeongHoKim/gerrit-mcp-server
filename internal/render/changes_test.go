package render_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

// update rewrites the golden files instead of comparing against them.
//
//	go test ./internal/render -update
var update = flag.Bool("update", false, "update golden files")

// golden compares got against testdata/<name>.golden.
func golden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name+".golden")

	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("creating testdata: %v", err)
		}

		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}

		return
	}

	raw, err := os.ReadFile(path) //nolint:gosec // the path is built from a test-supplied name
	if err != nil {
		t.Fatalf("reading %s (run with -update to create it): %v", path, err)
	}

	// .gitattributes pins these files to LF, but a checkout predating it -- or
	// one made with core.autocrlf=true and no attributes in play -- hands us
	// CRLF. render only ever emits LF, so the difference is never meaningful.
	want := strings.ReplaceAll(string(raw), "\r\n", "\n")

	if got != want {
		// %q, because a line ending is the one difference that a plain diff
		// prints as two identical blocks.
		t.Errorf("output mismatch for %s\n--- got ---\n%q\n--- want ---\n%q", name, got, want)
	}
}

func at(t *testing.T, value string) gerrit.Timestamp {
	t.Helper()

	parsed, err := time.Parse("2006-01-02 15:04:05.000000000", value)
	if err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}

	return gerrit.Timestamp{Time: parsed}
}

func TestChanges(t *testing.T) {
	t.Parallel()

	tests := map[string][]gerrit.ChangeInfo{
		"changes_empty": {},
		"changes_single": {{
			Number:     12345,
			Project:    "platform/base",
			Branch:     "main",
			Subject:    "fix the widget alignment",
			Status:     "NEW",
			Updated:    at(t, "2026-07-31 06:04:05.000000000"),
			Owner:      gerrit.AccountInfo{Name: "Alice Adams", Username: "alice", AccountID: 1000096},
			Insertions: 42,
			Deletions:  3,
		}},
		"changes_multiple": {
			{
				Number:         12345,
				Project:        "platform/base",
				Branch:         "main",
				Subject:        "fix the widget alignment",
				Status:         "NEW",
				Updated:        at(t, "2026-07-31 06:04:05.000000000"),
				Owner:          gerrit.AccountInfo{Name: "Alice Adams", AccountID: 1000096},
				Insertions:     42,
				Deletions:      3,
				WorkInProgress: true,
			},
			{
				Number:    12346,
				Project:   "tools",
				Branch:    "release-3.14",
				Topic:     "cleanup",
				Subject:   "drop the unused helper",
				Status:    "MERGED",
				Updated:   at(t, "2026-07-30 09:00:00.000000000"),
				Owner:     gerrit.AccountInfo{Username: "bob", AccountID: 1000042},
				Deletions: 18,
			},
		},
		// Everything optional is absent: no owner name, no topic, no counts.
		"changes_minimal": {{
			Number:  1,
			Project: "p",
			Branch:  "main",
			Subject: "initial commit",
			Status:  "MERGED",
			Owner:   gerrit.AccountInfo{AccountID: 7},
		}},
		// The last entry carrying _more_changes means Gerrit cut the result
		// set short; saying "2 changes" would misreport the project.
		"changes_truncated": {
			{
				Number: 1, Project: "p", Branch: "main", Subject: "one", Status: "NEW",
				Owner: gerrit.AccountInfo{Username: "alice", AccountID: 1},
			},
			{
				Number: 2, Project: "p", Branch: "main", Subject: "two", Status: "NEW",
				Owner: gerrit.AccountInfo{Username: "alice", AccountID: 1}, MoreChanges: true,
			},
		},
		"changes_private": {{
			Number:    999,
			Project:   "secret",
			Branch:    "main",
			Subject:   "not for everyone",
			Status:    "NEW",
			Updated:   at(t, "2026-07-31 06:04:05.000000000"),
			Owner:     gerrit.AccountInfo{Username: "carol", AccountID: 5},
			IsPrivate: true,
		}},
	}

	for name, changes := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.Changes(changes))
		})
	}
}

func TestChangesStaysCompact(t *testing.T) {
	t.Parallel()

	// A hundred changes is a plausible query result. The rendering has to stay
	// proportional -- this is the guard against someone reintroducing raw JSON.
	changes := make([]gerrit.ChangeInfo, 0, 100)
	for i := range 100 {
		changes = append(changes, gerrit.ChangeInfo{
			Number:  i,
			Project: "platform/base",
			Branch:  "main",
			Subject: "a change with a reasonably typical subject line",
			Status:  "NEW",
			Updated: at(t, "2026-07-31 06:04:05.000000000"),
			Owner:   gerrit.AccountInfo{Name: "Alice Adams", AccountID: 1},
		})
	}

	got := render.Changes(changes)

	const maxBytesPerChange = 200
	if len(got) > len(changes)*maxBytesPerChange {
		t.Errorf("rendered %d bytes for %d changes, want under %d per change",
			len(got), len(changes), maxBytesPerChange)
	}
}

func TestSubmittedTogether(t *testing.T) {
	t.Parallel()

	tests := map[string][]gerrit.ChangeInfo{
		// An empty list means the change submits alone, which must not read as
		// "nothing found".
		"submitted_alone": {},
		"submitted_with_others": {
			{
				Number: 12345, Project: "p", Branch: "main", Subject: "first", Status: "NEW",
				Owner: gerrit.AccountInfo{Username: "alice", AccountID: 1},
			},
			{
				Number: 12346, Project: "p", Branch: "main", Subject: "second", Status: "NEW",
				Owner: gerrit.AccountInfo{Username: "bob", AccountID: 2},
			},
		},
	}

	for name, changes := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			golden(t, name, render.SubmittedTogether(changes))
		})
	}
}
