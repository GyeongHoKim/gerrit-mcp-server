// Command gerrit-mcp-server exposes a Gerrit code review host over the Model
// Context Protocol on stdio, so that any MCP-capable agent can query changes
// and review code without leaving its session.
//
// The process speaks JSON-RPC on stdout. Nothing else may ever be written
// there -- diagnostics go to stderr.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/mcpserver"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/version"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		log.New(os.Stderr, "gerrit-mcp-server: ", 0).Println(err)
		os.Exit(1)
	}
}

// run parses arguments and serves the MCP server, returning any error rather
// than exiting so that it stays testable.
//
// Both writers are injected because which one a message lands on is the single
// invariant this binary cannot get wrong: stdout is the JSON-RPC channel.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("gerrit-mcp-server", flag.ContinueOnError)
	// Usage text and parse errors are diagnostics. -version is the one thing
	// here that belongs on stdout, and it writes there explicitly below.
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print version information and exit")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *showVersion {
		if _, err := fmt.Fprintln(stdout, version.String()); err != nil {
			return fmt.Errorf("writing version: %w", err)
		}

		return nil
	}

	return serve(ctx, os.LookupEnv, stderr, &mcp.StdioTransport{})
}

// serve builds the server from the environment and runs it over transport
// until the client disconnects.
//
// The environment, the diagnostic sink and the transport are all injected so
// that a test can drive the whole wiring over an in-memory transport; main
// supplies the real ones. Nothing here may reach for os.Stdout: that is the
// transport's, and only the transport's.
func serve(
	ctx context.Context,
	lookup func(string) (string, bool),
	stderr io.Writer,
	transport mcp.Transport,
) error {
	cfg, err := config.Load(lookup)
	if err != nil {
		return fmt.Errorf("reading configuration: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	// Deliberately not the token. Everything here is safe in a shared log.
	logger.Info("serving gerrit over stdio",
		"url", cfg.BaseURL.String(),
		"user", cfg.User,
		"writes_allowed", cfg.AllowWrite,
		"version", version.Version,
	)

	if !cfg.AllowWrite {
		logger.Info("write tools are not registered; set " + config.EnvAllowWrite + "=true to enable them")
	}

	// Ctrl-C and SIGTERM unwind the same path a disconnecting client does.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := gerrit.New(gerrit.Options{
		BaseURL: cfg.BaseURL,
		User:    cfg.User,
		Token:   cfg.Token,
		Timeout: cfg.Timeout,
	})

	server := mcpserver.New(client, cfg.AllowWrite)

	// Not mcp.Server.Run, which collapses connecting and serving into one
	// error. Failing to connect is this process failing to start; a session
	// ending is the client going away, and the two do not deserve the same
	// exit status.
	session, err := server.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("serving over stdio: %w", err)
	}

	// Probed only once the transport is up. Registering tools must not make a
	// network call, and asking Gerrit before Connect would delay the client's
	// initialize by a round trip to a host that may not answer at all.
	stopProbe := pruneUnsupportedTools(ctx, server, client, logger, cfg.AllowWrite)
	defer stopProbe()

	wait(ctx, session, logger)

	return nil
}

// wait blocks until the client disconnects or the context is cancelled.
//
// How a session ended is logged rather than returned. On stdio there is one
// client and one way for a session to end -- the other end went away -- so a
// broken pipe is a diagnostic and not an exit status. A client subscribed to
// tools/list_changed usually ends this way: its subscription is a
// subscriptions/listen request the server holds open for the subscription's
// lifetime, and disconnecting fails that request unless the cancellation the
// client sends first happens to arrive before the connection closes.
//
// Which of those two the client wins is a race inside the SDK and not
// something an operator can act on, so the line is written either way and the
// reason is what varies. A session that ends silently is the one shape of
// this a reader cannot tell from a hang.
//
// A cancelled context closes the session rather than abandoning it, and the
// goroutine waiting on it is joined either way.
func wait(ctx context.Context, session *mcp.ServerSession, logger *slog.Logger) {
	closed := make(chan error, 1)

	go func() { closed <- session.Wait() }()

	select {
	case err := <-closed:
		if err != nil {
			logger.Info("the client disconnected", "reason", err)
		} else {
			logger.Info("the client disconnected")
		}
	case <-ctx.Done():
		// The error is the session's own; a close that fails on the way out
		// tells nobody anything they can act on.
		_ = session.Close() //nolint:errcheck // see above
		<-closed
	}
}

// pruneUnsupportedTools asks Gerrit which release it is and removes the tools
// it is too old to serve, returning a function that joins the probe.
//
// It runs in the background because the answer is not worth delaying the
// session for. RemoveTools notifies the client, so an answer that arrives
// after the first tools/list is a tools/list_changed rather than a list that
// was wrong.
//
// Only a server that registered write tools probes at all: every tool with a
// minimum version is a write tool, so a read-only server has nothing to prune
// and makes no network call.
//
// The returned function cancels before it waits. The probe is blocked on an
// HTTP request that only unwinds on cancellation, so waiting first would hold
// shutdown open for the whole probe timeout.
func pruneUnsupportedTools(
	ctx context.Context,
	server *mcp.Server,
	client *gerrit.Client,
	logger *slog.Logger,
	allowWrite bool,
) func() {
	if !allowWrite {
		return func() {}
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		running, err := client.GetServerVersion(ctx)
		if err != nil {
			// Not a failure to serve. A host that will not say which Gerrit it
			// is gets every tool, and one it does not have answers for itself.
			logger.Warn("could not determine the gerrit version; offering every tool", "error", err)

			return
		}

		logger.Info("gerrit version", "version", running.String())

		if unsupported := mcpserver.UnsupportedTools(running); len(unsupported) > 0 {
			server.RemoveTools(unsupported...)
			logger.Info("hiding tools this gerrit is too old to have", "tools", unsupported)
		}
	}()

	return func() {
		cancel()
		<-done
	}
}
