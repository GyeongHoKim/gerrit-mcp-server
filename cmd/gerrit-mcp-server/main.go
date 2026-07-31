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
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		log.New(os.Stderr, "gerrit-mcp-server: ", 0).Println(err)
		os.Exit(1)
	}
}

// run parses arguments and serves the MCP server, returning any error rather
// than exiting so that it stays testable.
func run(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("gerrit-mcp-server", flag.ContinueOnError)
	flags.SetOutput(stdout)
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

	return serve(ctx)
}

// serve builds the server from the environment and runs it over stdio until
// the client disconnects.
func serve(ctx context.Context) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("reading configuration: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

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

	server := mcpserver.New(gerrit.New(cfg), cfg.AllowWrite)

	if err = server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serving over stdio: %w", err)
	}

	return nil
}
