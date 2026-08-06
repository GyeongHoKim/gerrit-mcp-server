package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/version"
)

// columnGap is the space between a command name and its summary.
const columnGap = 2

// helpCommand shows the command list, or the flags one command takes.
func helpCommand() Command {
	return Command{
		Name:    "help",
		Summary: "Show this list, or the flags one command takes.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			if len(args) == 0 {
				return writeHelp(deps.Options.Stdout)
			}

			return writeCommandHelp(ctx, deps, normalize(args[0]))
		},
	}
}

// versionCommand prints the build information.
func versionCommand() Command {
	const name = "version"

	return Command{
		Name:    name,
		Summary: "Print the build information.",
		Run: func(_ context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)

			if err := parse(flags, args); err != nil {
				return err
			}

			return emit(deps.Options.Stdout, version.StringFor(ProgramName)+"\n")
		},
	}
}

// writeCommandHelp prints one command's flags by handing that command -h.
//
// flag.Parse prints the usage and stops at -h before a command does anything
// else, which is why passing no Gerrit client here is safe -- and why the test
// that walks all of them through this doubles as the check that none reaches
// for the client before it parses.
//
// The meta commands take no flags, so asking about one prints the list.
func writeCommandHelp(ctx context.Context, deps Deps, name string) error {
	command, ok := find(GerritCommands(), name)
	if !ok {
		if _, isMeta := find(metaCommands(), name); isMeta {
			return writeHelp(deps.Options.Stdout)
		}

		return fmt.Errorf("%w: %s (try `%s help`)", ErrUnknownCommand, name, ProgramName)
	}

	// Usage asked for by name is the answer to the question, not a diagnostic,
	// so it goes to stdout.
	asked := *deps.Options
	asked.Stderr = deps.Options.Stdout

	err := command.Run(ctx, Deps{Options: &asked}, []string{"-h"})
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		return err
	}

	return nil
}

// writeHelp prints the command list.
func writeHelp(out io.Writer) error {
	commands := GerritCommands()
	// Every section is padded to one width, so it has to come from every
	// command listed rather than from the Gerrit ones alone: a longer meta
	// command would otherwise ask for negative padding and panic.
	width := nameWidth(append(commands, metaCommands()...))

	var body strings.Builder

	body.WriteString(ProgramName + " drives a Gerrit code review host from the command line.\n")
	body.WriteString("\nUsage:\n")
	body.WriteString("  " + ProgramName + " <command> [flags]\n")
	body.WriteString("  " + ProgramName + " help <command>   show the flags one command takes\n")
	body.WriteString("\nEvery value is a flag; there are no positional arguments.\n")

	writeSection(&body, "Read commands", pick(commands, false), width)

	// Write commands are listed even when the configuration refuses them.
	// internal/mcpserver hides its write tools because a model cannot call a
	// tool it cannot see; a caller here can type any string, so hiding them
	// would only teach a reader the feature does not exist when the answer is
	// one variable away.
	writeSection(&body, "Write commands (need "+config.EnvAllowWrite+"=true)", pick(commands, true), width)
	writeSection(&body, "Other commands", metaCommands(), width)

	body.WriteString("\nConfiguration comes from " + config.EnvURL + ", " + config.EnvUser + " and " +
		config.EnvToken + ", or from the file `" + ProgramName + " config` names.\n")

	if _, err := io.WriteString(out, body.String()); err != nil {
		return fmt.Errorf("writing help: %w", err)
	}

	return nil
}

// writeSection appends one titled block of commands, and nothing at all when
// there are none to list.
func writeSection(body *strings.Builder, title string, commands []Command, width int) {
	if len(commands) == 0 {
		return
	}

	body.WriteString("\n" + title + ":\n")

	for _, command := range commands {
		padding := width - len(command.Name) + columnGap
		body.WriteString("  " + command.Name + strings.Repeat(" ", padding) + command.Summary + "\n")
	}
}

// pick returns the commands whose Write flag matches want.
func pick(commands []Command, want bool) []Command {
	picked := make([]Command, 0, len(commands))

	for _, command := range commands {
		if command.Write == want {
			picked = append(picked, command)
		}
	}

	return picked
}

// nameWidth reports the longest command name, so the summaries line up.
func nameWidth(commands []Command) int {
	width := 0

	for _, command := range commands {
		width = max(width, len(command.Name))
	}

	return width
}
