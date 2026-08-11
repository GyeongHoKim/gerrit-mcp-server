package gerrit

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The cases below are the contract for what a Gerrit 3.x server would have
// parsed for us. Anything this gets wrong shows up as a missing or invented
// issue reference, which is why the table is long for fifty lines of code.
func TestParseFooters(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want    map[string]string
		message string
	}{
		"a message with no trailers": {
			message: "Fix the thing\n\nIt was broken.\n",
			want:    nil,
		},
		"the ordinary case": {
			message: "Fix the thing\n\nIt was broken.\n\nBug: 42\nChange-Id: I1039447\n",
			want:    map[string]string{"Bug": "42", "Change-Id": "I1039447"},
		},
		// A one-line commit message is a subject. Reporting it as an issue
		// reference would be worse than missing it, so a message with only one
		// paragraph yields nothing at all.
		"a single paragraph is a subject, not a trailer": {
			message: "Bug: 42\n",
			want:    nil,
		},
		"a subject that looks like a trailer keeps its body's trailers": {
			message: "Bug: 42\n\nChange-Id: I1\n",
			want:    map[string]string{"Change-Id": "I1"},
		},
		// The middle of a commit message is prose. "Note: this is slow" is a
		// sentence, not a footer.
		"a colon line in the body is not a trailer": {
			message: "Fix it\n\nNote: this used to be slow.\n\nChange-Id: I1\n",
			want:    map[string]string{"Change-Id": "I1"},
		},
		"the value keeps everything past the first colon": {
			message: "Fix it\n\nbody\n\nBug: http://tracker.example.com/1\n",
			want:    map[string]string{"Bug": "http://tracker.example.com/1"},
		},
		// Present in essentially every Gerrit repository. Strict git would
		// disqualify the whole paragraph over it; we skip the line instead.
		"a cherry pick line does not disqualify the block": {
			message: "Fix it\n\nbody\n\nBug: 42\n(cherry picked from commit abc123)\nChange-Id: I1\n",
			want:    map[string]string{"Bug": "42", "Change-Id": "I1"},
		},
		"an indented line continues the trailer above it": {
			message: "Fix it\n\nbody\n\nBug: 42,\n  43\nChange-Id: I1\n",
			want:    map[string]string{"Bug": "42, 43", "Change-Id": "I1"},
		},
		// Gerrit's own /message endpoint keeps only the last of these. Joining
		// is strictly more useful, and Bugs() already splits on the comma.
		"a repeated key is joined rather than replaced": {
			message: "Fix it\n\nbody\n\nBug: 42\nBug: 43\n",
			want:    map[string]string{"Bug": "42, 43"},
		},
		"keys are canonicalised": {
			message: "Fix it\n\nbody\n\nbug: 42\nCHANGE-ID: I1\nrelated-bug: 43\n",
			want:    map[string]string{"Bug": "42", "Change-Id": "I1", "Related-Bug": "43"},
		},
		"a key outside the trailer charset is not a trailer": {
			message: "Fix it\n\nbody\n\nSee also: nothing\nBug: 42\n",
			want:    map[string]string{"Bug": "42"},
		},
		"an empty value is kept": {
			message: "Fix it\n\nbody\n\nBug:\nChange-Id: I1\n",
			want:    map[string]string{"Bug": "", "Change-Id": "I1"},
		},
		"crlf line endings are handled": {
			message: "Fix it\r\n\r\nbody\r\n\r\nBug: 42\r\nChange-Id: I1\r\n",
			want:    map[string]string{"Bug": "42", "Change-Id": "I1"},
		},
		"trailing blank lines do not hide the block": {
			message: "Fix it\n\nbody\n\nBug: 42\n\n\n",
			want:    map[string]string{"Bug": "42"},
		},
		"a trailer paragraph with nothing parseable yields nothing": {
			message: "Fix it\n\nbody\n\n(cherry picked from commit abc123)\n",
			want:    nil,
		},
		"an empty message": {
			message: "",
			want:    nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(test.want, parseFooters(test.message)); diff != "" {
				t.Errorf("parseFooters() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCanonicalFooterKey(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"Bug":         "Bug",
		"bug":         "Bug",
		"BUG":         "Bug",
		"change-id":   "Change-Id",
		"CHANGE-ID":   "Change-Id",
		"Change-ID":   "Change-Id",
		"related-bug": "Related-Bug",
		"partial-bug": "Partial-Bug",
		"a":           "A",
		"a--b":        "A--B",
	}

	for key, want := range tests {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			if got := canonicalFooterKey(key); got != want {
				t.Errorf("canonicalFooterKey(%q) = %q, want %q", key, got, want)
			}
		})
	}
}

// The parsed footers have to reach Bugs() in the spelling it looks up, so the
// two are checked together rather than trusting the canonicalisation in
// isolation.
func TestParseFootersFeedsBugs(t *testing.T) {
	t.Parallel()

	message := "Fix it\n\nbody\n\nbug: 42\nrelated-bug: b/43\nChange-Id: I1\n"
	info := CommitMessageInfo{Footers: parseFooters(message)}

	if diff := cmp.Diff([]string{"42", "b/43"}, info.Bugs()); diff != "" {
		t.Errorf("Bugs() mismatch (-want +got):\n%s", diff)
	}
}
