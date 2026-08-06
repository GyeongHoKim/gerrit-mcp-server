package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
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

	return runCLIWith(t, handler, nil, args...)
}

// runCLIWith is [runCLI] with extra environment layered on top, for the tests
// that turn writes on.
func runCLIWith(
	t *testing.T,
	handler http.HandlerFunc,
	extra map[string]string,
	args ...string,
) result {
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
	maps.Copy(env, extra)

	var stdout, stderr strings.Builder

	// No ConfigPath and no ReadFile: these tests configure through the
	// environment, which is the path that must work on a host with no
	// configuration file at all.
	err := Run(t.Context(), args, &Options{
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

// allowWrites is the environment that lifts the write gate.
func allowWrites() map[string]string {
	return map[string]string{config.EnvAllowWrite: "true"}
}

// stubServer starts a stub Gerrit and returns its URL.
func stubServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server.URL
}

// errWriter fails every write, standing in for a closed stdout.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("stream closed")
}

// commandArgs is the smallest invocation of each command that gets past the
// argument validation in internal/gerrit and actually dials the host.
//
// Spelled out per command so that adding a twenty-third without deciding how
// to call it fails TestEveryCommandIsCovered, rather than quietly going
// untested on the paths that matter most -- the write gate, and what each
// command says when Gerrit refuses.
func commandArgs() map[string][]string {
	change := []string{"-" + flagChangeID, "12345"}

	with := func(extra ...string) []string {
		return append(slices.Clone(change), extra...)
	}

	return map[string][]string{
		"query-changes":              {"-" + flagQuery, "is:open"},
		"get-change-details":         change,
		"get-commit-message":         change,
		"list-change-files":          change,
		"get-file-diff":              with("-"+flagFile, "src/widget.go"),
		"list-change-comments":       change,
		"list-draft-comments":        change,
		"changes-submitted-together": change,
		"suggest-reviewers":          change,
		"get-bugs-from-cl":           change,

		"post-review-comment":   with("-"+flagFile, "src/widget.go", "-"+flagMessage, "nit"),
		"publish-drafts":        change,
		"delete-draft-comment":  with("-draft-id", "abc123"),
		"delete-draft-comments": change,
		"add-reviewer":          with("-reviewer", "bob"),
		"set-topic":             with("-"+flagTopic, "cleanup"),
		"set-ready-for-review":  change,
		"set-work-in-progress":  change,
		"abandon-change":        change,
		"revert-change":         change,
		"revert-submission":     change,
		"create-change":         {"-project", "platform/base", "-branch", "main", "-subject", "wire it up"},
	}
}

// invocation is the full argument list for one command.
func invocation(name string) []string {
	return append([]string{name}, commandArgs()[name]...)
}

func TestEveryCommandIsCovered(t *testing.T) {
	t.Parallel()

	args := commandArgs()

	for _, command := range GerritCommands() {
		if _, ok := args[command.Name]; !ok {
			t.Errorf("%s has no entry in commandArgs; add one so its failure path is tested", command.Name)
		}
	}

	if len(args) != len(GerritCommands()) {
		t.Errorf("commandArgs has %d entries for %d commands", len(args), len(GerritCommands()))
	}
}

// TestEveryCommandReportsAGerritFailure pins what each command says when
// Gerrit refuses, which is the half of every command that the happy path never
// reaches.
//
// The wanted text is copied from internal/mcpserver's equivalent test on
// purpose. Both frontends wrap the same call with the same words, so a failure
// reads identically whichever one reported it, and this is what holds them
// there.
func TestEveryCommandReportsAGerritFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want   string
		status int
	}{
		"query-changes":              {status: http.StatusBadRequest, want: "querying changes"},
		"get-change-details":         {status: http.StatusNotFound, want: "fetching change 12345"},
		"get-commit-message":         {status: http.StatusNotFound, want: "fetching commit message for 12345"},
		"list-change-files":          {status: http.StatusNotFound, want: "listing files for 12345"},
		"get-file-diff":              {status: http.StatusNotFound, want: "fetching diff of src/widget.go in 12345"},
		"list-change-comments":       {status: http.StatusForbidden, want: "listing comments on 12345"},
		"list-draft-comments":        {status: http.StatusNotFound, want: "listing draft comments on 12345"},
		"changes-submitted-together": {status: http.StatusNotFound, want: "computing changes submitted with 12345"},
		"suggest-reviewers":          {status: http.StatusForbidden, want: "suggesting reviewers for 12345"},
		"get-bugs-from-cl":           {status: http.StatusNotFound, want: "fetching commit message for 12345"},

		"post-review-comment":   {status: http.StatusNotFound, want: "staging a draft comment on 12345"},
		"publish-drafts":        {status: http.StatusConflict, want: "publishing drafts on 12345"},
		"delete-draft-comment":  {status: http.StatusNotFound, want: "discarding draft abc123 on 12345"},
		"delete-draft-comments": {status: http.StatusNotFound, want: "discarding drafts on 12345 after removing 0"},
		"add-reviewer":          {status: http.StatusForbidden, want: "adding bob to 12345"},
		"set-topic":             {status: http.StatusForbidden, want: "setting the topic on 12345"},
		"set-ready-for-review":  {status: http.StatusConflict, want: "marking 12345 ready for review"},
		"set-work-in-progress":  {status: http.StatusConflict, want: "marking 12345 work-in-progress"},
		"abandon-change":        {status: http.StatusConflict, want: "abandoning 12345"},
		"revert-change":         {status: http.StatusConflict, want: "reverting 12345"},
		"revert-submission":     {status: http.StatusConflict, want: "reverting the submission containing 12345"},
		"create-change":         {status: http.StatusForbidden, want: "creating a change on platform/base"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			refuse := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(test.status) }

			got := runCLIWith(t, refuse, allowWrites(), invocation(name)...)
			if got.err == nil {
				t.Fatalf("%s succeeded against a %d, want the failure reported", name, test.status)
			}

			// The wrapping is what turns "not found" into something the reader
			// can act on: which change, which file, which operation.
			if !strings.Contains(got.err.Error(), test.want) {
				t.Errorf("error = %v, want it to say %q", got.err, test.want)
			}

			// Nothing partial reaches stdout on a failure, so a caller piping
			// the output never sees half an answer.
			if got.stdout != "" {
				t.Errorf("stdout = %q, want nothing written on a failure", got.stdout)
			}
		})
	}
}

// TestGerritFailuresCarryTheirStatus pins the other half of a failure: the
// exit code, which is what an agent reads before deciding whether to fix its
// arguments, ask a human, or give up.
func TestGerritFailuresCarryTheirStatus(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status int
		want   int
	}{
		"a missing change":   {status: http.StatusNotFound, want: ExitNotFound},
		"a bad token":        {status: http.StatusUnauthorized, want: ExitNotPermitted},
		"no permission":      {status: http.StatusForbidden, want: ExitNotPermitted},
		"a wrong state":      {status: http.StatusConflict, want: ExitConflict},
		"a broken host":      {status: http.StatusInternalServerError, want: ExitFailure},
		"a rejected request": {status: http.StatusBadRequest, want: ExitFailure},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			refuse := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(test.status) }

			got := runCLI(t, refuse, invocation("get-change-details")...)
			if got.err == nil {
				t.Fatalf("succeeded against a %d, want the failure reported", test.status)
			}

			if code := ExitCode(got.err); code != test.want {
				t.Errorf("ExitCode() = %d for a %d, want %d", code, test.status, test.want)
			}
		})
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

			// Writes are allowed here so that the gate is not what stops the
			// command: this is a test of the argument handling behind it, and
			// the gate has its own.
			got := runCLIWith(t, refuse(t), allowWrites(), command.Name, "12345")

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

			got := runCLIWith(t, refuse(t), allowWrites(), command.Name, "-nonexistent")

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

			opts := &Options{Lookup: lookupFrom(nil), Stdout: &stdout, Stderr: &stderr}

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

// readFixtures is a body each read command can actually decode.
//
// The shapes differ -- a list, a map keyed by file, a single object -- and
// handing every command an empty object would leave each of them failing to
// decode, which reads as a passing test right up until you notice none of them
// ever reached its renderer.
func readFixtures() map[string]string {
	return map[string]string{
		"query-changes":              `[]`,
		"get-change-details":         `{"_number":12345,"project":"platform/base","subject":"wire it up"}`,
		"get-commit-message":         `{"message":"wire it up\n\nBug: 42\n"}`,
		"list-change-files":          `{"src/widget.go":{"lines_inserted":3}}`,
		"get-file-diff":              `{"content":[{"ab":["unchanged"]}]}`,
		"list-change-comments":       `{"src/widget.go":[{"id":"abc123","message":"nit"}]}`,
		"list-draft-comments":        `{"src/widget.go":[{"id":"abc123","message":"nit"}]}`,
		"changes-submitted-together": `[]`,
		"suggest-reviewers":          `[]`,
		"get-bugs-from-cl":           `{"message":"wire it up\n\nBug: 42\n"}`,
	}
}

// TestReadCommandsRunWithoutTheWriteOptIn is also the only test that walks each
// read command all the way to its renderer, so the fixtures have to be
// decodable rather than merely well-formed.
func TestReadCommandsRunWithoutTheWriteOptIn(t *testing.T) {
	t.Parallel()

	fixtures := readFixtures()

	for _, command := range pick(GerritCommands(), false) {
		t.Run(command.Name, func(t *testing.T) {
			t.Parallel()

			body, ok := fixtures[command.Name]
			if !ok {
				t.Fatalf("%s has no fixture in readFixtures", command.Name)
			}

			got := runCLI(t, serveJSON(t, body), invocation(command.Name)...)
			if got.err != nil {
				t.Fatalf("%s returned an unexpected error: %v", command.Name, got.err)
			}

			if errors.Is(got.err, ErrWriteNotAllowed) {
				t.Errorf("%s was refused as a write command", command.Name)
			}

			// Reaching the renderer is the point. A command that answered
			// nothing has not been tested by getting here.
			if got.stdout == "" {
				t.Errorf("%s wrote nothing to stdout", command.Name)
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

	err := Run(t.Context(), []string{"query-changes", "-query", "is:open"}, &Options{
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

	err := Run(t.Context(), []string{"query-changes"}, &Options{
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
		"success":                       {err: nil, want: ExitOK},
		"help is not a failure":         {err: flag.ErrHelp, want: ExitOK},
		"a wrapped help request":        {err: fmtErrorf(flag.ErrHelp), want: ExitOK},
		"unknown command":               {err: ErrUnknownCommand, want: ExitUsage},
		"leftover arguments":            {err: ErrUnexpectedArguments, want: ExitUsage},
		"an empty change id":            {err: gerrit.ErrEmptyChangeID, want: ExitUsage},
		"nothing configured":            {err: config.ErrMissing, want: ExitNotConfigured},
		"a bad variable":                {err: config.ErrInvalid, want: ExitNotConfigured},
		"writes not allowed":            {err: ErrWriteNotAllowed, want: ExitNotPermitted},
		"unauthorized":                  {err: gerrit.ErrUnauthorized, want: ExitNotPermitted},
		"forbidden":                     {err: gerrit.ErrForbidden, want: ExitNotPermitted},
		"no such change":                {err: gerrit.ErrNotFound, want: ExitNotFound},
		"init without a terminal":       {err: ErrNotInteractive, want: ExitUsage},
		"a configuration already there": {err: ErrConfigExists, want: ExitUsage},
		"init told nothing":             {err: ErrIncompleteAnswers, want: ExitUsage},
		"nowhere to put the file":       {err: ErrNoConfigPath, want: ExitNotConfigured},
		"a conflict":                    {err: gerrit.ErrConflict, want: ExitConflict},
		"a rejected review":             {err: gerrit.ErrReviewRejected, want: ExitConflict},
		"anything else":                 {err: errors.New("the network is gone"), want: ExitFailure},
		"a response too large":          {err: gerrit.ErrResponseTooLarge, want: ExitFailure},
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

			err := Run(t.Context(), args, &Options{
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

	err := Run(ctx, []string{"query-changes", "-query", "is:open"}, &Options{
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
