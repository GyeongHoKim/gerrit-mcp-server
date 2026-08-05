// Command gerrit-cli drives a Gerrit code review host from the command line.
//
// It is the second frontend to the client gerrit-mcp-server exposes over the
// Model Context Protocol, for callers who would rather spend a process than a
// context window: an MCP server's tool schemas sit in a model's context for
// the whole session, whereas an agent skill teaching an agent to run this
// binary costs one line until it is used.
//
// Rendered output goes to stdout and everything else -- usage, diagnostics,
// errors -- goes to stderr, so a caller can pipe the answer.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/cli"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		log.New(os.Stderr, cli.ProgramName+": ", 0).Println(err)
		os.Exit(cli.ExitCode(err))
	}
}

// run builds the injected outside world and dispatches one invocation,
// returning any error rather than exiting so that it stays testable.
//
// This is the only place that reaches for the real environment and the real
// filesystem; everything below takes them as values.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return cli.Run(ctx, args, &cli.Options{
		Lookup:      os.LookupEnv,
		ReadFile:    os.ReadFile,
		WriteFile:   os.WriteFile,
		MkdirAll:    os.MkdirAll,
		Rename:      os.Rename,
		Stat:        os.Stat,
		Stdin:       os.Stdin,
		Stdout:      stdout,
		Stderr:      stderr,
		ConfigPath:  configPath(os.LookupEnv, os.UserConfigDir),
		Interactive: interactive(os.Stdin),
	})
}

// interactive reports a stdin attached to a terminal.
//
// A character device is a terminal; a pipe, a file and /dev/null are not.
// That is the whole test, and the standard library can answer it --
// golang.org/x/term would say the same thing and cost a dependency this
// project does not take.
//
// init is the only command that cares, and it cares a great deal: reading a
// pipe nobody is typing into blocks forever, which for an agent that ran the
// command speculatively means a session that never comes back. A pipe is what
// this has to catch, and it does.
//
// Windows reports NUL as a character device, so `gerrit-cli init < NUL` looks
// like a terminal here. That is harmless -- the reads return EOF at once and
// init refuses for having been told nothing, rather than blocking -- and
// telling NUL from a console needs a syscall this does not make.
func interactive(stdin *os.File) bool {
	info, err := stdin.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

// configPath resolves where the configuration file should be.
//
// A host that cannot resolve a configuration directory -- a bare container, or
// an agent sandbox with no HOME -- yields an empty path rather than an error.
// Every command works from the environment alone, and refusing to start here
// would break exactly the hosts agents run on.
func configPath(lookup func(string) (string, bool), userConfigDir func() (string, error)) string {
	if override, ok := lookup(config.EnvConfigPath); ok && strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}

	path, err := config.DefaultPath(userConfigDir)
	if err != nil {
		return ""
	}

	return path
}
