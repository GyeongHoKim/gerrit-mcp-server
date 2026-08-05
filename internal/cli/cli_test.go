package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

const (
	testUser  = "alice"
	testToken = "s3cret"
)

// xssiPrefix is the guard Gerrit puts in front of every JSON body. The
// fixtures here carry it because handling it is part of what the client under
// test does.
const xssiPrefix = ")]}'"

// result is everything one gerrit-cli invocation produced.
type result struct {
	err      error
	stdout   string
	stderr   string
	requests int64
}

// lookupFrom adapts a map to the os.LookupEnv signature.
func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]

		return value, ok
	}
}

// runCLI runs one invocation against a stub Gerrit, with the environment the
// commands need to reach it, and counts what actually arrived there.
func runCLI(t *testing.T, handler http.HandlerFunc, args ...string) result {
	t.Helper()

	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	env := map[string]string{
		config.EnvURL:   server.URL,
		config.EnvUser:  testUser,
		config.EnvToken: testToken,
	}

	var stdout, stderr strings.Builder

	// No ConfigPath and no ReadFile: these tests configure through the
	// environment, which is the path that must work on a host with no
	// configuration file at all.
	err := Run(t.Context(), args, Options{
		Lookup: lookupFrom(env),
		Stdout: &stdout,
		Stderr: &stderr,
	})

	return result{
		err:      err,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		requests: requests.Load(),
	}
}

// refuse is a stub Gerrit that fails the test if anything reaches it.
func refuse(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("gerrit was called at %s, want the command to have stopped first", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// serveJSON replies with body behind Gerrit's XSSI guard.
func serveJSON(t *testing.T, body string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := io.WriteString(w, xssiPrefix+"\n"+body); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	}
}

func TestEveryCommandDescribesItself(t *testing.T) {
	t.Parallel()

	for _, command := range append(GerritCommands(), metaCommands()...) {
		if command.Summary == "" {
			t.Errorf("%s has no summary", command.Name)
		}

		if command.Run == nil {
			t.Errorf("%s has nothing to run", command.Name)
		}

		// The name is a contract with internal/mcpserver's parity test, and an
		// underscore there would mean a command an agent cannot reach by the
		// spelling the help prints.
		if strings.Contains(command.Name, "_") {
			t.Errorf("%s is spelled with an underscore, want dashes", command.Name)
		}
	}
}

// TestEveryCommandRejectsUnexpectedArguments pins the guard against the one
// mistake the standard flag package makes silently: it stops at the first
// non-flag token, so a stray value would leave every flag after it unparsed
// and the command would answer a different question than the one asked.
func TestEveryCommandRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()

	for _, command := range GerritCommands() {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			got := runCLI(t, refuse(t), command.Name, "12345")

			if !errors.Is(got.err, ErrUnexpectedArguments) {
				t.Errorf("error = %v, want it to match %v", got.err, ErrUnexpectedArguments)
			}

			if ExitCode(got.err) != ExitUsage {
				t.Errorf("ExitCode() = %d, want %d", ExitCode(got.err), ExitUsage)
			}

			if got.requests != 0 {
				t.Errorf("gerrit saw %d requests, want none", got.requests)
			}
		})
	}
}

func TestEveryCommandRejectsAnUnknownFlag(t *testing.T) {
	t.Parallel()

	for _, command := range GerritCommands() {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			got := runCLI(t, refuse(t), command.Name, "-nonexistent")

			if !errors.Is(got.err, ErrUsage) {
				t.Errorf("error = %v, want it to match %v", got.err, ErrUsage)
			}

			if got.stdout != "" {
				t.Errorf("stdout = %q, want the complaint on stderr", got.stdout)
			}

			if got.requests != 0 {
				t.Errorf("gerrit saw %d requests, want none", got.requests)
			}
		})
	}
}

// TestHelpDescribesEveryCommand doubles as the check that parsing is the first
// thing every command does: it drives each one with no Gerrit client at all,
// which only works because flag.Parse stops at -h before anything reaches for
// one.
func TestHelpDescribesEveryCommand(t *testing.T) {
	t.Parallel()

	for _, command := range GerritCommands() {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder

			opts := Options{Lookup: lookupFrom(nil), Stdout: &stdout, Stderr: &stderr}

			if err := writeCommandHelp(t.Context(), Deps{Options: opts}, command.Name); err != nil {
				t.Fatalf("writeCommandHelp(%s) returned an unexpected error: %v", command.Name, err)
			}

			if !strings.Contains(stdout.String(), command.Name) {
				t.Errorf("help for %s = %q, want it to name the command", command.Name, stdout.String())
			}

			// Usage asked for by name is the answer, not a diagnostic.
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want the usage on stdout", stderr.String())
			}
		})
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	if err := writeHelp(&stdout); err != nil {
		t.Fatalf("writeHelp() returned an unexpected error: %v", err)
	}

	for _, command := range append(GerritCommands(), metaCommands()...) {
		if !strings.Contains(stdout.String(), command.Name) {
			t.Errorf("help does not list %s", command.Name)
		}
	}
}

// TestQueryChangesRendersExactly is the test that catches an Fprintln where an
// Fprint belongs: everything internal/render returns already ends in a
// newline, so a command that adds one puts a blank line under every answer.
func TestQueryChangesRendersExactly(t *testing.T) {
	t.Parallel()

	got := runCLI(t, serveJSON(t, "[]"), "query-changes", "-query", "is:open")
	if got.err != nil {
		t.Fatalf("query-changes returned an unexpected error: %v", got.err)
	}

	const want = "No changes matched.\n"

	if got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}

	if got.stderr != "" {
		t.Errorf("stderr = %q, want nothing on a successful run", got.stderr)
	}
}

func TestReadCommandsRunWithoutTheWriteOptIn(t *testing.T) {
	t.Parallel()

	for _, command := range pick(GerritCommands(), false) {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			got := runCLI(t, serveJSON(t, "{}"), command.Name, "-"+flagChangeID, "12345")

			if errors.Is(got.err, ErrWriteNotAllowed) {
				t.Errorf("%s was refused as a write command", command.Name)
			}
		})
	}
}

func TestRunAcceptsTheUnderscoreSpelling(t *testing.T) {
	t.Parallel()

	// The MCP tool is spelled query_changes, and an agent that remembers it
	// should get the answer rather than a lecture about dashes.
	got := runCLI(t, serveJSON(t, "[]"), "query_changes", "-query", "is:open")
	if got.err != nil {
		t.Fatalf("query_changes returned an unexpected error: %v", got.err)
	}

	if got.stdout != "No changes matched.\n" {
		t.Errorf("stdout = %q, want the same answer the dashed spelling gives", got.stdout)
	}
}

func TestRunRejectsAnUnknownCommand(t *testing.T) {
	t.Parallel()

	got := runCLI(t, refuse(t), "list-everything")

	if !errors.Is(got.err, ErrUnknownCommand) {
		t.Errorf("error = %v, want it to match %v", got.err, ErrUnknownCommand)
	}

	if !strings.Contains(got.err.Error(), "help") {
		t.Errorf("error = %v, want it to point at the help", got.err)
	}
}

func TestRunWithNoArgumentsPrintsTheHelpAndFails(t *testing.T) {
	t.Parallel()

	got := runCLI(t, refuse(t))

	if !errors.Is(got.err, ErrNoCommand) {
		t.Errorf("error = %v, want it to match %v", got.err, ErrNoCommand)
	}

	if ExitCode(got.err) != ExitUsage {
		t.Errorf("ExitCode() = %d, want %d", ExitCode(got.err), ExitUsage)
	}

	// The list is a diagnostic here rather than an answer, so it goes to
	// stderr and leaves stdout clean for a caller that piped it.
	if !strings.Contains(got.stderr, "query-changes") {
		t.Errorf("stderr = %q, want the command list", got.stderr)
	}

	if got.stdout != "" {
		t.Errorf("stdout = %q, want nothing", got.stdout)
	}
}

func TestRunReportsAMissingConfiguration(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder

	err := Run(t.Context(), []string{"query-changes", "-query", "is:open"}, Options{
		Lookup: lookupFrom(nil),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err == nil {
		t.Fatal("Run() succeeded with nothing configured, want an error")
	}

	// Every missing variable at once, so a shell can be fixed in one pass.
	for _, want := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}

	// The status is what tells an agent to run init rather than retry.
	if ExitCode(err) != ExitNotConfigured {
		t.Errorf("ExitCode() = %d, want %d", ExitCode(err), ExitNotConfigured)
	}
}

func TestRunReportsAnUnreadableConfigurationFile(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder

	err := Run(t.Context(), []string{"query-changes"}, Options{
		Lookup:     lookupFrom(nil),
		ReadFile:   func(string) ([]byte, error) { return []byte(`{"tokn":"typo"}`), nil },
		Stdout:     &stdout,
		Stderr:     &stderr,
		ConfigPath: "/config/gerrit-cli/config.json",
	})
	if err == nil {
		t.Fatal("Run() succeeded on a malformed configuration file, want an error")
	}

	// A corrupt file and a missing one need the same thing done about them, so
	// they report the same status.
	if ExitCode(err) != ExitNotConfigured {
		t.Errorf("ExitCode() = %d, want %d", ExitCode(err), ExitNotConfigured)
	}
}

func TestVersionNamesTheProgram(t *testing.T) {
	t.Parallel()

	got := runCLI(t, refuse(t), "--version")
	if got.err != nil {
		t.Fatalf("--version returned an unexpected error: %v", got.err)
	}

	if !strings.HasPrefix(got.stdout, ProgramName+" ") {
		t.Errorf("stdout = %q, want it to name %s", got.stdout, ProgramName)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want int
	}{
		"success":                {err: nil, want: ExitOK},
		"help is not a failure":  {err: flag.ErrHelp, want: ExitOK},
		"a wrapped help request": {err: fmtErrorf(flag.ErrHelp), want: ExitOK},
		"unknown command":        {err: ErrUnknownCommand, want: ExitUsage},
		"leftover arguments":     {err: ErrUnexpectedArguments, want: ExitUsage},
		"an empty change id":     {err: gerrit.ErrEmptyChangeID, want: ExitUsage},
		"nothing configured":     {err: config.ErrMissing, want: ExitNotConfigured},
		"a bad variable":         {err: config.ErrInvalid, want: ExitNotConfigured},
		"writes not allowed":     {err: ErrWriteNotAllowed, want: ExitNotPermitted},
		"unauthorized":           {err: gerrit.ErrUnauthorized, want: ExitNotPermitted},
		"forbidden":              {err: gerrit.ErrForbidden, want: ExitNotPermitted},
		"no such change":         {err: gerrit.ErrNotFound, want: ExitNotFound},
		"a conflict":             {err: gerrit.ErrConflict, want: ExitConflict},
		"a rejected review":      {err: gerrit.ErrReviewRejected, want: ExitConflict},
		"anything else":          {err: errors.New("the network is gone"), want: ExitFailure},
		"a response too large":   {err: gerrit.ErrResponseTooLarge, want: ExitFailure},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := ExitCode(test.err); got != test.want {
				t.Errorf("ExitCode(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}

// fmtErrorf wraps err the way a command would, so the exit-code table is
// checked against a wrapped error rather than only a bare sentinel.
func fmtErrorf(err error) error {
	return errors.Join(errors.New("parsing flags"), err)
}

// TestRunKeepsDiagnosticsOffStdout pins the discipline that makes the output
// pipeable: only rendered answers land on stdout, so `gerrit-cli ... | grep`
// never picks up a usage message.
func TestRunKeepsDiagnosticsOffStdout(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"an unknown command":  {"list-everything"},
		"an unknown flag":     {"query-changes", "-nonexistent"},
		"leftover arguments":  {"query-changes", "leftover"},
		"no command at all":   {},
		"a misconfigured run": {"query-changes", "-query", "is:open"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder

			err := Run(t.Context(), args, Options{
				Lookup: lookupFrom(nil),
				Stdout: &stdout,
				Stderr: &stderr,
			})
			if err == nil {
				t.Fatalf("Run(%v) succeeded, want it to report the problem", args)
			}

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing but rendered output there", stdout.String())
			}
		})
	}
}

// TestRunPassesTheContextThrough checks that a cancelled context stops the
// command rather than being ignored on the way to the client.
func TestRunPassesTheContextThrough(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout, stderr strings.Builder

	server := httptest.NewServer(refuse(t))
	t.Cleanup(server.Close)

	err := Run(ctx, []string{"query-changes", "-query", "is:open"}, Options{
		Lookup: lookupFrom(map[string]string{
			config.EnvURL:   server.URL,
			config.EnvUser:  testUser,
			config.EnvToken: testToken,
		}),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to carry the cancellation", err)
	}
}
