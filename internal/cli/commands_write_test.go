package cli

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// TestWriteCommandsNeverReachGerritWithoutTheOptIn is the test that costs
// something. The MCP server enforces this by not registering the tool at all;
// gerrit-cli cannot hide a command from a caller who can type any string, so
// the gate has to hold here instead -- and it has to hold before anything is
// sent, not after Gerrit has already been asked.
func TestWriteCommandsNeverReachGerritWithoutTheOptIn(t *testing.T) {
	t.Parallel()

	for _, command := range writeCommands() {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			got := runCLI(t, refuse(t), invocation(command.Name)...)

			if !errors.Is(got.err, ErrWriteNotAllowed) {
				t.Errorf("error = %v, want it to match %v", got.err, ErrWriteNotAllowed)
			}

			if got.requests != 0 {
				t.Errorf("gerrit saw %d requests, want none before the gate opens", got.requests)
			}

			// A refusal that does not say how to lift it is a dead end for
			// whoever hit it.
			if !strings.Contains(got.err.Error(), config.EnvAllowWrite) {
				t.Errorf("error = %v, want it to name %s", got.err, config.EnvAllowWrite)
			}

			// The status is what tells an agent to ask a human rather than
			// retry with different arguments.
			if ExitCode(got.err) != ExitNotPermitted {
				t.Errorf("ExitCode() = %d, want %d", ExitCode(got.err), ExitNotPermitted)
			}
		})
	}
}

func TestWriteCommandsRunWithTheOptIn(t *testing.T) {
	t.Parallel()

	for _, command := range writeCommands() {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			got := runCLIWith(t, serveJSON(t, "{}"), allowWrites(), invocation(command.Name)...)

			// Not merely "not refused": with the opt-in set the command has to
			// run all the way through, so a gate that opened onto a broken call
			// still fails here.
			if got.err != nil {
				t.Fatalf("%s returned an unexpected error: %v", command.Name, got.err)
			}

			if got.requests == 0 {
				t.Errorf("%s never reached gerrit, want the command to have run", command.Name)
			}

			// Every write command renders its result, and stdout is where a
			// caller pipes it from. What it says is internal/render's contract
			// and is goldened there; that it says anything at all is this one.
			if got.stdout == "" {
				t.Errorf("%s wrote nothing to stdout, want the rendered result", command.Name)
			}
		})
	}
}

// TestWriteCommandsAreAllMarked is the complement of the gate: a command that
// mutates Gerrit but is left out of writeCommands would be registered as a
// read, and would run without the opt-in.
// The commands that carry a minimum version, and the flags that reach them.
func versionGatedInvocations() map[string][]string {
	return map[string][]string{
		"set-work-in-progress": {"--change-id", "12345"},
		"set-ready-for-review": {"--change-id", "12345"},
		"revert-submission":    {"--change-id", "12345"},
	}
}

// End to end on the frontend a person types into: an old host answers 404,
// internal/gerrit turns that into a sentence naming both releases, and the
// exit code says to stop rather than to fix the arguments.
func TestVersionGatedCommandsReportAnOldServer(t *testing.T) {
	t.Parallel()

	for name, flags := range versionGatedInvocations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != "/a/config/server/version" {
					w.WriteHeader(http.StatusNotFound)

					return
				}

				if _, err := w.Write([]byte(")]}'\n\"2.14.22\"")); err != nil {
					t.Errorf("writing test response: %v", err)
				}
			}

			got := runCLIWith(t, handler, allowWrites(), append([]string{name}, flags...)...)

			if !errors.Is(got.err, gerrit.ErrUnsupportedByServer) {
				t.Fatalf("%s error = %v, want ErrUnsupportedByServer", name, got.err)
			}

			if code := ExitCode(got.err); code != ExitNotPermitted {
				t.Errorf("%s exit code = %d, want %d", name, code, ExitNotPermitted)
			}

			// Both releases: the one the command needs and the one the host
			// admits to. Either alone leaves the reader without the next step.
			for _, want := range []string{"2.14", "or newer"} {
				if !strings.Contains(got.err.Error(), want) {
					t.Errorf("%s error = %q, want it to mention %q", name, got.err, want)
				}
			}
		})
	}
}

// Every version-gated command is a write command. gerrit-cli does not depend
// on that the way the MCP server does, but the two frontends describing the
// same operations differently would be its own bug.
func TestVersionGatedCommandsAreWriteCommands(t *testing.T) {
	t.Parallel()

	gated := versionGatedInvocations()

	for _, command := range GerritCommands() {
		if _, want := gated[command.Name]; want != !command.MinVersion.IsZero() {
			t.Errorf("%s has a minimum version = %t, want %t", command.Name, !want, want)
		}

		if _, isGated := gated[command.Name]; isGated && !command.Write {
			t.Errorf("%s carries a minimum version but is not a write command", command.Name)
		}
	}
}

func TestWriteCommandsAreAllMarked(t *testing.T) {
	t.Parallel()

	for _, command := range writeCommands() {
		if !command.Write {
			t.Errorf("%s is listed as a write command but is not marked as one", command.Name)
		}
	}

	for _, command := range readCommands() {
		if command.Write {
			t.Errorf("%s is listed as a read command but is marked as a write", command.Name)
		}
	}
}
