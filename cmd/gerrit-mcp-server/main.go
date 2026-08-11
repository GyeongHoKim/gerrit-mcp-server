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

	dropUnsupportedTools(ctx, server, client, logger, cfg.AllowWrite)

	// Not mcp.Server.Run, which collapses connecting and serving into one
	// error. Failing to connect is this process failing to start; a session
	// ending is the client going away, and the two do not deserve the same
	// exit status.
	session, err := server.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("serving over stdio: %w", err)
	}

	wait(ctx, session, logger)

	return nil
}

// wait blocks until the client disconnects or the context is cancelled.
//
// How a session ended is logged rather than returned: on stdio the only way to
// end is the other end going away, which is a diagnostic and not an exit
// status. The line is written whether or not the disconnect failed a request
// still in flight, because a session that ends silently reads as a hang.
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

// dropUnsupportedTools asks Gerrit which release it is and removes the tools it
// is too old to serve.
//
// Called before Connect, so the first tools/list is already correct. Removing
// later would leave that to a tools/list_changed, which is sent once and only
// to a client the server has already seen subscribe.
//
// Only a server with write tools probes at all: every version-gated tool is a
// write tool. A version that cannot be determined offers everything.
func dropUnsupportedTools(
	ctx context.Context,
	server *mcp.Server,
	client *gerrit.Client,
	logger *slog.Logger,
	allowWrite bool,
) {
	if !allowWrite {
		return
	}

	running, err := client.GetServerVersion(ctx)
	if err != nil {
		// Not a failure to serve. A host that will not say which Gerrit it is
		// gets every tool, and one it does not have answers for itself.
		logger.Warn("could not determine the gerrit version; offering every tool", "error", err)

		return
	}

	logger.Info("gerrit version", "version", running.String())

	if unsupported := mcpserver.UnsupportedTools(running); len(unsupported) > 0 {
		server.RemoveTools(unsupported...)
		logger.Info("hiding tools this gerrit is too old to have", "tools", unsupported)
	}
}
