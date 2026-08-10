package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// stdout is the JSON-RPC channel. Anything written there that is not protocol
// traffic corrupts the stream and the client disconnects, so what lands on
// which writer is the one thing this command cannot get wrong.

// lookupFrom returns an [os.LookupEnv] stand-in reading from env.
//
// config.Load takes the lookup as a parameter precisely so that this is
// possible; nothing here has to mutate the process environment.
func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]

		return value, ok
	}
}

// validEnv is the smallest environment serve accepts. The host is never
// dialled: listing tools does not reach Gerrit, and a server without
// GERRIT_ALLOW_WRITE does not probe for the version either, because every
// version-gated tool is a write tool.
//
// A test that turns writes on does dial this host. It does not resolve, so the
// probe fails and every tool stays offered, which is the fail-open path.
func validEnv() map[string]string {
	return map[string]string{
		config.EnvURL:   "https://gerrit.example.com",
		config.EnvUser:  "alice",
		config.EnvToken: "s3cret",
	}
}

// envAgainst is validEnv pointed at a stub Gerrit that reports version, with
// writes enabled so that the version-gated tools are registered to begin with.
func envAgainst(t *testing.T, version string) map[string]string {
	t.Helper()

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/a/config/server/version" {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		if _, err := w.Write([]byte(")]}'\n\"" + version + "\"")); err != nil {
			t.Errorf("writing stub response: %v", err)
		}
	}))
	t.Cleanup(stub.Close)

	env := validEnv()
	env[config.EnvURL] = stub.URL
	env[config.EnvAllowWrite] = "true"

	return env
}

// servedSession is a running serve call with a client attached to it.
type servedSession struct {
	session *mcp.ClientSession
	done    <-chan error
}

// startServe runs serve over an in-memory transport and connects a client to
// it.
func startServe(t *testing.T, env map[string]string, stderr io.Writer) servedSession {
	t.Helper()

	return startServeWith(t, env, stderr, nil)
}

// startServeWith is [startServe] for a test that needs the client to react to
// something the server sends it.
func startServeWith(
	t *testing.T,
	env map[string]string,
	stderr io.Writer,
	opts *mcp.ClientOptions,
) servedSession {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	done := make(chan error, 1)

	go func() {
		done <- serve(t.Context(), lookupFrom(env), stderr, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, opts)

	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting to the served session: %v", err)
	}

	return servedSession{session: session, done: done}
}

// A client subscribed to tools/list_changed leaves a subscriptions/listen
// request in flight for the life of the subscription, so disconnecting always
// fails it with the SDK's "server is closing". That is an ordinary disconnect
// and must not be reported as a failure to serve.
//
// The subscription needs nothing from Gerrit: the SDK advertises
// tools.listChanged whenever a server has any tools, so this reproduces
// against a host that is never dialled.
func TestServeEndsCleanlyForASubscribedClient(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	served := startServeWith(t, validEnv(), &stderr, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {},
	})

	// Listed first so the subscription is certainly established before the
	// client goes away.
	if _, err := served.session.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	if err := served.stop(t); err != nil {
		t.Errorf("serve() = %v, want a clean return", err)
	}

	// Discarded, not hidden: how the pipe broke is still worth a line.
	if !strings.Contains(stderr.String(), "the client disconnected") {
		t.Errorf("stderr does not record how the session ended:\n%s", stderr.String())
	}
}

// stop disconnects the client and returns what serve returned.
//
// Reading the channel is also what orders the caller against anything serve
// wrote to stderr on its own goroutine.
func (s servedSession) stop(t *testing.T) error {
	t.Helper()

	if err := s.session.Close(); err != nil {
		t.Fatalf("closing the session: %v", err)
	}

	return <-s.done
}

// failingTransport refuses to connect, standing in for a stdio pipe that is
// already gone.
type failingTransport struct{ err error }

func (f failingTransport) Connect(context.Context) (mcp.Connection, error) {
	return nil, f.err
}

// errWriter fails every write, standing in for a closed stdout.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("stream closed")
}

func TestRunPrintsTheVersionOnStdout(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder

	if err := run(t.Context(), []string{"-version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() returned an unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "gerrit-mcp-server") {
		t.Errorf("stdout = %q, want it to name the binary", stdout.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want the version on stdout alone", stderr.String())
	}
}

func TestRunReportsAnUnwritableStdout(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder

	err := run(t.Context(), []string{"-version"}, errWriter{}, &stderr)
	if err == nil {
		t.Fatal("run() succeeded writing the version to a closed stdout, want the failure reported")
	}

	// Swallowing this would exit 0 having printed nothing, which reads as a
	// version of the binary that does not know its own version.
	if !strings.Contains(err.Error(), "writing version") {
		t.Errorf("error = %v, want it to name the failed write", err)
	}
}

func TestRunKeepsDiagnosticsOffStdout(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		wantErr error
		args    []string
	}{
		// -h is not a failure; main returns without a message for it.
		"usage text":  {args: []string{"-h"}, wantErr: flag.ErrHelp},
		"parse error": {args: []string{"-nonexistent"}, wantErr: nil},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder

			err := run(t.Context(), test.args, &stdout, &stderr)
			if err == nil {
				t.Fatalf("run(%v) succeeded, want it to report the flag problem", test.args)
			}

			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Errorf("run(%v) error = %v, want it to match %v", test.args, err, test.wantErr)
			}

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing but JSON-RPC on stdout", stdout.String())
			}

			if stderr.Len() == 0 {
				t.Errorf("stderr is empty, want the flag package's message on it")
			}
		})
	}
}

// TestRunServesWhenGivenNoFlags pins that an argument-less invocation -- the
// only way an MCP client ever starts this binary -- reaches serve.
//
// The environment is blanked rather than left to the developer's shell, so the
// configuration step fails before anything opens the real stdio transport.
func TestRunServesWhenGivenNoFlags(t *testing.T) {
	for _, key := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		t.Setenv(key, "")
	}

	var stdout, stderr strings.Builder

	err := run(t.Context(), nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() succeeded with no configuration, want it to report that")
	}

	if !strings.Contains(err.Error(), "reading configuration") {
		t.Errorf("error = %v, want it to come from serve", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing but JSON-RPC on stdout", stdout.String())
	}
}

func TestServeRunsUntilTheClientDisconnects(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	served := startServe(t, validEnv(), &stderr)

	if _, err := served.session.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("listing tools on the served session: %v", err)
	}

	// A client going away is the normal end of this process, not a failure.
	if err := served.stop(t); err != nil {
		t.Errorf("serve() = %v, want a clean return once the client disconnected", err)
	}

	if got := stderr.String(); !strings.Contains(got, "gerrit.example.com") {
		t.Errorf("stderr = %q, want the startup line naming the host", got)
	}

	// The token is the one thing that must never reach a log a colleague reads.
	if strings.Contains(stderr.String(), "s3cret") {
		t.Errorf("stderr leaks the auth token:\n%s", stderr.String())
	}
}

// TestServeRegistersWriteToolsOnlyWhenAllowed pins the wiring behind
// GERRIT_ALLOW_WRITE. internal/mcpserver enforces the split; this is the test
// that the flag actually travels from the environment to that decision.
func TestServeRegistersWriteToolsOnlyWhenAllowed(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		allowWrite string
		want       bool
	}{
		"unset":   {allowWrite: "", want: false},
		"enabled": {allowWrite: "true", want: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := validEnv()
			if test.allowWrite != "" {
				env[config.EnvAllowWrite] = test.allowWrite
			}

			served := startServe(t, env, io.Discard)

			tools, err := served.session.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatalf("listing tools: %v", err)
			}

			got := slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool {
				return tool.Name == "post_review_comment"
			})
			if got != test.want {
				t.Errorf("post_review_comment registered = %t, want %t", got, test.want)
			}

			if stopErr := served.stop(t); stopErr != nil {
				t.Errorf("serve() = %v, want a clean return", stopErr)
			}
		})
	}
}

// toolNames lists what the client can currently see.
func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}

	return names
}

// The prune happens after Connect, so the client learns about it through
// notifications/tools/list_changed. Waiting on the notification rather than
// polling is deliberate: that it fires at all is the load-bearing claim behind
// pruning post-connect rather than delaying startup to probe first.
func TestServeHidesToolsAnOldGerritCannotServe(t *testing.T) {
	t.Parallel()

	changed := make(chan struct{}, 1)

	served := startServeWith(t, envAgainst(t, "2.14.22"), io.Discard, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			select {
			case changed <- struct{}{}:
			default:
			}
		},
	})

	select {
	case <-changed:
	case <-t.Context().Done():
		t.Fatal("the server never announced a change to its tool list")
	}

	got := toolNames(t, served.session)

	for _, gone := range []string{"revert_submission", "set_work_in_progress", "set_ready_for_review"} {
		if slices.Contains(got, gone) {
			t.Errorf("%s is still offered by a 2.14 server", gone)
		}
	}

	// The rest of the write set is untouched. A prune that took the whole
	// group would be indistinguishable from one that worked, from the outside.
	if !slices.Contains(got, "abandon_change") {
		t.Errorf("abandon_change was pruned; it works on every supported version")
	}

	if stopErr := served.stop(t); stopErr != nil {
		t.Errorf("serve() = %v, want a clean return", stopErr)
	}
}

// A current Gerrit gets everything, and says nothing about it. The assertion
// that matters is the tool list; the absence of a notification is not worth
// waiting on a timeout to prove.
func TestServeKeepsEveryToolOnACurrentGerrit(t *testing.T) {
	t.Parallel()

	served := startServe(t, envAgainst(t, "3.14.1"), io.Discard)

	if got := toolNames(t, served.session); !slices.Contains(got, "revert_submission") {
		t.Errorf("revert_submission is missing from a 3.14 server: %v", got)
	}

	if stopErr := served.stop(t); stopErr != nil {
		t.Errorf("serve() = %v, want a clean return", stopErr)
	}
}

// A host that will not say which Gerrit it is keeps every tool. Hiding one on
// a failed probe would take a working operation away from a host behind a
// proxy that swallows the version endpoint.
func TestServeOffersEveryToolWhenTheVersionIsUnknown(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	// The stub answers 404 on every path but the version one, and this asks it
	// for a version it will not produce.
	served := startServe(t, envAgainst(t, ""), &stderr)

	if got := toolNames(t, served.session); !slices.Contains(got, "revert_submission") {
		t.Errorf("revert_submission was hidden on an undetermined version: %v", got)
	}

	if stopErr := served.stop(t); stopErr != nil {
		t.Errorf("serve() = %v, want a clean return", stopErr)
	}

	// stop reads the done channel, which orders this against the probe's own
	// goroutine.
	if !strings.Contains(stderr.String(), "offering every tool") {
		t.Errorf("stderr does not report the failed probe:\n%s", stderr.String())
	}
}

func TestServeRejectsAMisconfiguredEnvironment(t *testing.T) {
	t.Parallel()

	transport := failingTransport{err: errors.New("the transport should not have been reached")}

	err := serve(t.Context(), lookupFrom(nil), io.Discard, transport)
	if err == nil {
		t.Fatal("serve() succeeded with an empty environment, want it refused")
	}

	if !strings.Contains(err.Error(), "reading configuration") {
		t.Errorf("error = %v, want it to name the configuration as the problem", err)
	}

	// Every missing variable is reported at once, so a client config can be
	// fixed in one pass rather than one variable per restart.
	for _, want := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

func TestServeReportsATransportFailure(t *testing.T) {
	t.Parallel()

	broken := errors.New("stdin is closed")

	err := serve(t.Context(), lookupFrom(validEnv()), io.Discard, failingTransport{err: broken})
	if err == nil {
		t.Fatal("serve() succeeded on a transport that cannot connect, want the failure reported")
	}

	if !errors.Is(err, broken) {
		t.Errorf("error = %v, want it to wrap the transport's own failure", err)
	}

	if !strings.Contains(err.Error(), "serving over stdio") {
		t.Errorf("error = %v, want it to say which stage failed", err)
	}
}
