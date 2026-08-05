package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Flag names shared by more than one command.
//
// Constants rather than literals because --change-id alone appears on fifteen
// commands, and a flag whose name is spelled out fifteen times is a flag that
// will eventually be spelled two ways.
const (
	flagChangeID = "change-id"
	flagMessage  = "message"
	flagFile     = "file"
	flagQuery    = "query"
	flagLimit    = "limit"
	flagTopic    = "topic"
)

// Errors raised while reading a command line.
var (
	// ErrUsage reports a command line gerrit-cli could not accept.
	ErrUsage = errors.New("invalid arguments")
	// ErrUnexpectedArguments reports values left over after the flags.
	ErrUnexpectedArguments = errors.New("unexpected arguments")
)

// newFlagSet builds a FlagSet for one command.
//
// Always ContinueOnError, never ExitOnError: revive's deep-exit rule confines
// os.Exit to main, and a FlagSet that exits on a typo cannot be tested.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(ProgramName+" "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)

	return flags
}

// parse parses args and refuses anything left over.
//
// The standard flag package stops at the first non-flag token, so
// `query-changes foo --limit 5` would otherwise parse as a bare "foo" with
// --limit silently ignored, and answer a different question than the one
// asked. Every value gerrit-cli takes is a flag, so a leftover is a mistake.
func parse(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if flags.NArg() > 0 {
		return fmt.Errorf("%w: %s (every value is a flag, see `%s help`)",
			ErrUnexpectedArguments, strings.Join(flags.Args(), " "), ProgramName)
	}

	return nil
}

// emit writes a rendered body to stdout.
//
// Fprint rather than Fprintln: everything internal/render returns already ends
// with a newline, and the golden files pin that. Adding another would put a
// blank line under every answer this binary gives.
func emit(stdout io.Writer, body string) error {
	if _, err := io.WriteString(stdout, body); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

// changeIDFlag binds the --change-id flag shared by the commands acting on one
// change.
//
// One binder per shared flag, so the usage text cannot drift between the
// fifteen commands that take it.
func changeIDFlag(flags *flag.FlagSet) *string {
	return flags.String(flagChangeID, "",
		"change number (12345) or the triplet project~branch~Change-Id")
}

// changeCommand builds one of the commands whose only argument is a change id.
//
// Nine commands have exactly this shape. Writing the binding out nine times
// would be nine chances for the flag name or its usage text to drift.
func changeCommand(
	name, summary string,
	act func(ctx context.Context, deps Deps, changeID string) error,
) Command {
	return Command{
		Name:    name,
		Summary: summary,
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)

			if err := parse(flags, args); err != nil {
				return err
			}

			return act(ctx, deps, *changeID)
		},
	}
}
