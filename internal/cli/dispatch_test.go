package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// The tests here drive commands through Run rather than calling what they do
// directly. That is the difference between "the work is right" and "the
// command line reaches the work": a flag bound but never read, or a meta
// command wired to the wrong function, is invisible to a test that skips the
// dispatcher.

func TestNormalizeAcceptsTheSpellingsCallersUse(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"-h":               "help",
		"-help":            "help",
		"--help":           "help",
		"-v":               "version",
		"-version":         "version",
		"--version":        "version",
		"query_changes":    "query-changes",
		"query-changes":    "query-changes",
		"get_bugs_from_cl": "get-bugs-from-cl",
	}

	for token, want := range tests {
		if got := normalize(token); got != want {
			t.Errorf("normalize(%q) = %q, want %q", token, got, want)
		}
	}
}

func TestRunReachesEveryMetaCommand(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want string
		args []string
	}{
		"the command list":    {args: []string{"help"}, want: "query-changes"},
		"one command's flags": {args: []string{"help", "get-file-diff"}, want: "-change-id"},
		"the underscore form": {args: []string{"help", "get_file_diff"}, want: "-change-id"},
		"a meta command":      {args: []string{"help", "version"}, want: "query-changes"},
		"the build stamp":     {args: []string{"version"}, want: ProgramName},
		"the version flag":    {args: []string{"--version"}, want: ProgramName},
		"the configuration":   {args: []string{"config"}, want: config.EnvURL},
		"the short help flag": {args: []string{"-h"}, want: "Usage"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder

			err := Run(t.Context(), test.args, &Options{
				Lookup: lookupFrom(nil),
				Stdout: &stdout,
				Stderr: &stderr,
			})
			if err != nil {
				t.Fatalf("Run(%v) returned an unexpected error: %v", test.args, err)
			}

			if !strings.Contains(stdout.String(), test.want) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), test.want)
			}
		})
	}
}

func TestRunRejectsHelpForAnUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder

	err := Run(t.Context(), []string{"help", "list-everything"}, &Options{
		Lookup: lookupFrom(nil),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if !errors.Is(err, ErrUnknownCommand) {
		t.Errorf("error = %v, want it to match %v", err, ErrUnknownCommand)
	}
}

func TestMetaCommandsRejectUnexpectedArguments(t *testing.T) {
	t.Parallel()

	// help takes a command name, so it is not in this set. init is not either:
	// it refuses a non-terminal before it parses, which is its own test. The
	// rest take nothing, and a caller who passed something meant something by
	// it.
	for _, args := range [][]string{{"version", "extra"}, {"config", "extra"}, {"doctor", "extra"}} {
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder

			err := Run(t.Context(), args, &Options{
				Lookup: lookupFrom(nil),
				Stdout: &stdout,
				Stderr: &stderr,
			})
			if !errors.Is(err, ErrUnexpectedArguments) {
				t.Errorf("error = %v, want it to match %v", err, ErrUnexpectedArguments)
			}
		})
	}
}

// TestRunWithNoCommandReportsAnUnwritableStderr covers the one path that
// cannot fall back anywhere: with no command at all the list is written to
// stderr, and a closed stderr leaves nothing to say.
func TestRunWithNoCommandReportsAnUnwritableStderr(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := Run(t.Context(), nil, &Options{
		Lookup: lookupFrom(nil),
		Stdout: &stdout,
		Stderr: errWriter{},
	})
	if err == nil {
		t.Fatal("Run() succeeded writing the list to a closed stderr, want the failure reported")
	}

	if errors.Is(err, ErrNoCommand) {
		t.Errorf("error = %v, want the write failure rather than the missing command", err)
	}
}

// TestWriteSectionSkipsAnEmptyGroup covers the guard that keeps a heading with
// nothing under it out of the help.
func TestWriteSectionSkipsAnEmptyGroup(t *testing.T) {
	t.Parallel()

	var body strings.Builder

	writeSection(&body, "Nothing here", nil, 10)

	if body.Len() != 0 {
		t.Errorf("writeSection() wrote %q for an empty group, want nothing", body.String())
	}
}

// TestRunReportsAnUnwritableStdout pins that a closed pipe is reported rather
// than swallowed. Exiting 0 having written nothing would tell a caller the
// change has no files.
func TestRunReportsAnUnwritableStdout(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"a rendered answer": invocation("query-changes"),
		"the command list":  {"help"},
		"the build stamp":   {"version"},
		"the configuration": {"config"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := stubServer(t, serveJSON(t, "[]"))

			var stderr strings.Builder

			err := Run(t.Context(), args, &Options{
				Lookup: lookupFrom(map[string]string{
					config.EnvURL:   server,
					config.EnvUser:  testUser,
					config.EnvToken: testToken,
				}),
				Stdout: errWriter{},
				Stderr: &stderr,
			})
			if err == nil {
				t.Fatalf("Run(%v) succeeded writing to a closed stdout, want the failure reported", args)
			}

			if !strings.Contains(err.Error(), "writing") {
				t.Errorf("error = %v, want it to name the failed write", err)
			}
		})
	}
}
