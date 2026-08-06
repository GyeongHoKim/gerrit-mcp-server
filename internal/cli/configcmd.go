package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// tokenMask is what the report says instead of the token.
//
// Reporting the token would put it in whatever scrollback, transcript or bug
// report the output is pasted into, and knowing that one is set is the only
// thing anyone diagnosing a configuration actually needs.
const tokenMask = "set"

// configCommand reports the configuration and where each value came from.
func configCommand() Command {
	const name = "config"

	return Command{
		Name:    name,
		Summary: "Report the configuration and where each value came from.",
		Run: func(_ context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)

			if err := parse(flags, args); err != nil {
				return err
			}

			return writeConfigReport(deps.Options)
		},
	}
}

// writeConfigReport prints the resolved configuration.
//
// It deliberately does not fail on a configuration Load would refuse. This is
// the command an agent runs when something is wrong, and answering "there is
// an error" to "what is set?" would waste the turn that was meant to fix it.
func writeConfigReport(opts *Options) error {
	file, readErr := readConfigFile(opts)
	values, sources := config.Values(opts.Lookup, &file)

	var body strings.Builder

	writeConfigLocation(&body, opts, readErr)

	width := keyWidth()

	for _, key := range config.Keys() {
		value, ok := values[key]
		if !ok {
			value = defaultFor(key)
		}

		if key == config.EnvToken && ok {
			value = tokenMask
		}

		if value == "" {
			value = "not set"
		}

		padding := width - len(key) + columnGap
		body.WriteString("  " + key + strings.Repeat(" ", padding) +
			pad(value) + "(" + sources.Of(key).String() + ")\n")
	}

	writeConfigAdvice(&body, values)

	return emit(opts.Stdout, body.String())
}

// writeConfigLocation names the configuration file and says whether it is
// there, which is half of what anyone runs this command to find out.
func writeConfigLocation(body *strings.Builder, opts *Options, readErr error) {
	switch {
	case opts.ConfigPath == "":
		body.WriteString("configuration file: none (no OS configuration directory on this host)\n\n")
	case readErr != nil:
		body.WriteString("configuration file: " + opts.ConfigPath + " (unreadable: " + readErr.Error() + ")\n\n")
	case fileExists(opts):
		body.WriteString("configuration file: " + opts.ConfigPath + "\n\n")
	default:
		body.WriteString("configuration file: " + opts.ConfigPath + " (not created yet)\n\n")
	}
}

// writeConfigAdvice names the fix when something required is missing, so that
// the report is actionable rather than merely accurate.
func writeConfigAdvice(body *strings.Builder, values map[string]string) {
	var missing []string

	for _, key := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		if values[key] == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) == 0 {
		return
	}

	body.WriteString("\n" + strings.Join(missing, ", ") + " must be set.\n")
	body.WriteString("Run `" + ProgramName + " init` in a terminal, or export them into this shell.\n")
}

// readConfigFile reads the configuration file, or reports an empty one when
// there is no path to read or nothing at it.
func readConfigFile(opts *Options) (config.File, error) {
	if opts.ConfigPath == "" || opts.ReadFile == nil {
		return config.File{}, nil
	}

	file, err := config.ReadFile(opts.ReadFile, opts.ConfigPath)
	if err != nil {
		return config.File{}, fmt.Errorf("%w: %w", ErrNotConfigured, err)
	}

	return file, nil
}

// fileExists reports whether the configuration file is on disk.
func fileExists(opts *Options) bool {
	if opts.Stat == nil {
		return false
	}

	_, err := opts.Stat(opts.ConfigPath)

	return !errors.Is(err, fs.ErrNotExist)
}

// defaultFor is what a variable nothing supplied resolves to.
func defaultFor(key string) string {
	switch key {
	case config.EnvAllowWrite:
		return strconv.FormatBool(false)
	case config.EnvTimeout:
		return config.DefaultTimeout.String()
	case config.EnvLogLevel:
		return strings.ToLower(config.DefaultLogLevel.String())
	default:
		return ""
	}
}

// keyWidth reports the longest variable name, so the report lines up.
func keyWidth() int {
	width := 0

	for _, key := range config.Keys() {
		width = max(width, len(key))
	}

	return width
}

// pad right-fills a reported value so that the sources line up beside it.
func pad(value string) string {
	const valueColumn = 30

	if len(value) >= valueColumn {
		return value + " "
	}

	return value + strings.Repeat(" ", valueColumn-len(value))
}
