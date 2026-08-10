package mcpserver

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

func TestUnsupportedTools(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want    []string
		running gerrit.ServerVersion
	}{
		// The oldest supported release: no work-in-progress state, and no
		// reverting a whole submission.
		"2.14": {
			running: gerrit.ServerVersion{Major: 2, Minor: 14},
			want:    []string{"revert_submission", "set_ready_for_review", "set_work_in_progress"},
		},
		"2.15 gained work in progress": {
			running: gerrit.ServerVersion{Major: 2, Minor: 15},
			want:    []string{"revert_submission"},
		},
		"2.16": {
			running: gerrit.ServerVersion{Major: 2, Minor: 16},
			want:    []string{"revert_submission"},
		},
		// A major bump is not enough on its own; revert_submission is 3.2.
		"3.1": {
			running: gerrit.ServerVersion{Major: 3, Minor: 1},
			want:    []string{"revert_submission"},
		},
		"3.2 gained revert submission": {
			running: gerrit.ServerVersion{Major: 3, Minor: 2},
			want:    nil,
		},
		"3.14": {
			running: gerrit.ServerVersion{Major: 3, Minor: 14},
			want:    nil,
		},
		// Nothing was determined. Hiding a tool on a guess is the one outcome
		// worse than offering one that fails with an explanation.
		"an undetermined version hides nothing": {
			running: gerrit.ServerVersion{},
			want:    nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(test.want, UnsupportedTools(test.running)); diff != "" {
				t.Errorf("UnsupportedTools(%v) mismatch (-want +got):\n%s", test.running, diff)
			}
		})
	}
}

// Every gated tool is a write tool, which is what lets the binary skip the
// version probe entirely on a read-only server. If a read tool ever acquires a
// minimum version, that shortcut becomes a bug and this is where it is caught.
func TestEveryVersionGatedToolIsAWriteTool(t *testing.T) {
	t.Parallel()

	writes := make(map[string]bool, len(wantWriteTools))
	for _, tool := range wantWriteTools {
		writes[tool] = true
	}

	for tool := range minVersions() {
		if !writes[tool] {
			t.Errorf("%s has a minimum version but is not a write tool", tool)
		}
	}
}

// A tool named in the table but never registered would be silently ignored by
// RemoveTools, and the gate would do nothing at all.
func TestEveryVersionGatedToolExists(t *testing.T) {
	t.Parallel()

	known := make(map[string]bool, len(wantReadTools)+len(wantWriteTools))
	for _, tool := range append(append([]string{}, wantReadTools...), wantWriteTools...) {
		known[tool] = true
	}

	for tool := range minVersions() {
		if !known[tool] {
			t.Errorf("%s has a minimum version but is not a tool", tool)
		}
	}
}
