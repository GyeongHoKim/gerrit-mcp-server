package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
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
