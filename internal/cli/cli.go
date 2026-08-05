// Package cli implements gerrit-cli, the command-line frontend to the same
// Gerrit client the MCP server drives.
//
// Commands here are as thin as the MCP tool handlers they mirror, and
// deliberately so: bind flags, call one method on the Gerrit client, hand the
// result to the renderer. Both frontends therefore answer the same question
// with the same bytes, and internal/render remains the only thing deciding
// what a caller ever sees.
//
// Unlike gerrit-mcp-server, stdout here is not a protocol channel. Rendered
// output goes to stdout and everything else -- usage, diagnostics, errors --
// goes to stderr, so that the answer can be piped and the noise cannot corrupt
// it.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// ProgramName is how gerrit-cli refers to itself in usage and errors.
const ProgramName = "gerrit-cli"

// Errors the dispatcher raises before a command ever runs.
var (
	// ErrNoCommand reports an invocation with no subcommand at all.
	ErrNoCommand = errors.New("no command given")
	// ErrUnknownCommand reports a subcommand gerrit-cli does not have.
	ErrUnknownCommand = errors.New("unknown command")
	// ErrWriteNotAllowed reports a mutating command run without the opt-in.
	ErrWriteNotAllowed = errors.New("this command modifies gerrit")
	// ErrNotConfigured reports a configuration gerrit-cli could not use.
	ErrNotConfigured = errors.New("gerrit-cli is not configured")
)

// Options is everything gerrit-cli reaches for outside its own logic, so that
// a test can drive [Run] without touching the real environment, the real
// filesystem or the real terminal.
type Options struct {
	// Lookup has the signature of os.LookupEnv.
	Lookup func(string) (string, bool)
	// ReadFile has the signature of os.ReadFile.
	ReadFile func(string) ([]byte, error)
	// Stdout receives rendered output and nothing else.
	Stdout io.Writer
	// Stderr receives usage, diagnostics and errors.
	Stderr io.Writer
	// ConfigPath is where the configuration file lives.
	//
	// Empty means the OS configuration directory could not be resolved, which
	// is a bare container or an agent sandbox. That is fatal only where a path
	// is actually required: every command here works from the environment
	// alone, and refusing to start would break exactly the hosts agents run on.
	ConfigPath string
}

// Deps is what one command is handed to do its work.
type Deps struct {
	// Gerrit is nil for the commands that run before the configuration is
	// known -- help and version. The dispatcher guarantees it is set for
	// everything in [GerritCommands].
	Gerrit *gerrit.Client
	// Options is the injected outside world.
	Options Options
}

// Command is one gerrit-cli subcommand.
type Command struct {
	// Run binds this command's flags to a fresh FlagSet, parses args, and does
	// the work. Parsing is always the first thing it does; `gerrit-cli help
	// <command>` relies on that to print a command's flags without a client.
	Run func(ctx context.Context, deps Deps, args []string) error
	// Name is the subcommand. It is the MCP tool name with underscores written
	// as dashes, and internal/mcpserver's parity test holds it there.
	Name string
	// Summary is the single line `gerrit-cli help` prints.
	Summary string
	// Write reports a command that modifies Gerrit. The dispatcher refuses
	// these unless the configuration allows writes.
	Write bool
}

// readCommands never modify Gerrit.
//
// A function rather than a package-level slice, for the same reason
// mcpserver.readTools is one: the read/write split is the boundary
// GERRIT_ALLOW_WRITE exists to enforce, and a var could be appended to from
// anywhere in the package, moving a destructive command into the set that runs
// without asking.
func readCommands() []Command {
	return []Command{
		queryChanges(),
		getChangeDetails(),
		getCommitMessage(),
		listChangeFiles(),
		getFileDiff(),
		listChangeComments(),
		listDraftComments(),
		changesSubmittedTogether(),
		suggestReviewers(),
		getBugsFromCL(),
	}
}

// GerritCommands returns every command that talks to Gerrit.
//
// The meta commands are deliberately not here. They are gerrit-cli's own and
// have no MCP tool to correspond to, which is exactly what the parity test in
// internal/mcpserver asserts about the ones that are.
func GerritCommands() []Command {
	return append(readCommands(), writeCommands()...)
}

// metaCommands are gerrit-cli's own, and run without a Gerrit client.
func metaCommands() []Command {
	return []Command{
		helpCommand(),
		versionCommand(),
	}
}

// Run executes one gerrit-cli invocation.
//
// The error is returned rather than printed so that main decides both the
// message and the status; see [ExitCode].
func Run(ctx context.Context, args []string, opts Options) error {
	if len(args) == 0 {
		if err := writeHelp(opts.Stderr); err != nil {
			return err
		}

		return fmt.Errorf("%w (try `%s help`)", ErrNoCommand, ProgramName)
	}

	name, rest := normalize(args[0]), args[1:]

	if command, ok := find(metaCommands(), name); ok {
		return command.Run(ctx, Deps{Options: opts}, rest)
	}

	command, ok := find(GerritCommands(), name)
	if !ok {
		return fmt.Errorf("%w: %s (try `%s help`)", ErrUnknownCommand, name, ProgramName)
	}

	return runGerritCommand(ctx, command, rest, opts)
}

// runGerritCommand loads the configuration, enforces the write gate and builds
// the client.
//
// The gate is checked before the client exists and long before anything is
// sent, so a mutating command run without the opt-in cannot reach Gerrit at
// all. That is the CLI's counterpart of never registering the tool.
//
// It is also checked before the command parses its own flags, which means a
// typo in a write command's arguments is reported as a refusal rather than as
// a bad flag. That is the right way round: without the opt-in the command was
// never going to run, and the refusal is the one message worth acting on.
func runGerritCommand(ctx context.Context, command Command, args []string, opts Options) error {
	cfg, _, err := loadConfig(opts)
	if err != nil {
		return err
	}

	if command.Write && !cfg.AllowWrite {
		return fmt.Errorf("%w: set %s=true to allow it", ErrWriteNotAllowed, config.EnvAllowWrite)
	}

	client := gerrit.New(gerrit.Options{
		BaseURL: cfg.BaseURL,
		User:    cfg.User,
		Token:   cfg.Token,
		Timeout: cfg.Timeout,
	})

	return command.Run(ctx, Deps{Gerrit: client, Options: opts}, args)
}

// loadConfig reads the configuration from the environment and, where there is
// one to read, the configuration file.
func loadConfig(opts Options) (config.Config, config.Sources, error) {
	var file config.File

	if opts.ConfigPath != "" && opts.ReadFile != nil {
		read, err := config.ReadFile(opts.ReadFile, opts.ConfigPath)
		if err != nil {
			return config.Config{}, nil, fmt.Errorf("%w: %w", ErrNotConfigured, err)
		}

		file = read
	}

	cfg, sources, err := config.LoadWithFile(opts.Lookup, &file)
	if err != nil {
		return config.Config{}, sources, fmt.Errorf("reading configuration: %w", err)
	}

	return cfg, sources, nil
}

// normalize maps the tokens a caller might reasonably type onto a command name.
//
// The underscore spelling is accepted because the MCP tool each command mirrors
// is spelled that way: an agent that remembers query_changes should get the
// answer, not a lecture about dashes.
func normalize(token string) string {
	switch token {
	case "-h", "-help", "--help":
		return "help"
	case "-v", "-version", "--version":
		return "version"
	default:
		return strings.ReplaceAll(token, "_", "-")
	}
}

// find returns the named command.
func find(commands []Command, name string) (Command, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}

	return Command{}, false
}
