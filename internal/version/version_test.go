package version

import (
	"strings"
	"testing"
)

// stamp sets the build variables for one test and restores them afterwards.
//
// They are package-level vars rather than constants because the release
// pipeline overwrites them with -ldflags, which is also the only way to test
// what a released binary reports. Tests that call this cannot be parallel.
func stamp(t *testing.T, version, commit, date string) {
	t.Helper()

	previous := [3]string{Version, Commit, Date}

	t.Cleanup(func() {
		Version, Commit, Date = previous[0], previous[1], previous[2]
	})

	Version, Commit, Date = version, commit, date
}

func TestStringReportsTheStampedBuild(t *testing.T) {
	stamp(t, "1.2.3", "abc1234", "2026-08-06T00:00:00Z")

	const want = "gerrit-mcp-server 1.2.3 (commit abc1234, built 2026-08-06T00:00:00Z)"

	if got := String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestStringKeepsTheStampsApart is the test that matters: all three values are
// strings, and a build reporting its commit where its version belongs would go
// unnoticed until someone tried to work out which release they were running.
func TestStringKeepsTheStampsApart(t *testing.T) {
	stamp(t, "the-version", "the-commit", "the-date")

	got := String()

	tests := map[string]struct {
		before string
		after  string
	}{
		"version": {before: "gerrit-mcp-server ", after: "the-version"},
		"commit":  {before: "(commit ", after: "the-commit"},
		"date":    {before: "built ", after: "the-date"},
	}

	for name, test := range tests {
		if !strings.Contains(got, test.before+test.after) {
			t.Errorf("String() = %q, want the %s to follow %q", got, name, test.before)
		}
	}
}

// TestStringForNamesTheProgram pins the half of the line String() cannot check:
// the same stamps are built into two binaries, and gerrit-cli reporting itself
// as gerrit-mcp-server would send anyone reading the output to the wrong
// program.
func TestStringForNamesTheProgram(t *testing.T) {
	stamp(t, "1.2.3", "abc1234", "2026-08-06T00:00:00Z")

	tests := map[string]string{
		"gerrit-mcp-server": "gerrit-mcp-server 1.2.3 (commit abc1234, built 2026-08-06T00:00:00Z)",
		"gerrit-cli":        "gerrit-cli 1.2.3 (commit abc1234, built 2026-08-06T00:00:00Z)",
	}

	for name, want := range tests {
		if got := StringFor(name); got != want {
			t.Errorf("StringFor(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestStringDescribesAnUnstampedBuild pins what a `go build` with no ldflags
// reports. These defaults are what a contributor running from source sees, and
// they have to read as "not a release" rather than as a real version.
func TestStringDescribesAnUnstampedBuild(t *testing.T) {
	const want = "gerrit-mcp-server dev (commit none, built unknown)"

	if got := String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
