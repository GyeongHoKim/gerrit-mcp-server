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
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

// serve runs the MCP server over stdio until the client disconnects.
func serve(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gerrit",
		Version: version.Version,
	}, nil)

	// Tools are registered here once internal/mcpserver lands.

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serving over stdio: %w", err)
	}

	return nil
}
