package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// Permissions for what init creates.
//
// On Linux and macOS these do what they say. On Windows os.Chmod only toggles
// the read-only attribute, so the file inherits the ACL of its parent under
// %AppData% instead -- already restricted to the account plus SYSTEM and
// Administrators. Setting a tighter DACL would need golang.org/x/sys, which is
// a dependency this project does not take; the README says so rather than this
// code pretending otherwise.
const (
	// configDirPerm keeps the configuration directory to its owner.
	configDirPerm fs.FileMode = 0o700
	// configFilePerm keeps a file holding an auth token to its owner.
	configFilePerm fs.FileMode = 0o600
)

// Errors raised by init.
var (
	// ErrNotInteractive reports an init that would have blocked reading a
	// stdin nobody is typing into.
	ErrNotInteractive = errors.New("init reads its answers from a terminal")
	// ErrConfigExists reports an init that would have replaced a
	// configuration file.
	ErrConfigExists = errors.New("a configuration file is already there")
	// ErrNoConfigPath reports a host with nowhere to put the file.
	ErrNoConfigPath = errors.New("no configuration file location could be resolved")
	// ErrIncompleteAnswers reports an init that was not told enough.
	ErrIncompleteAnswers = errors.New("init needs a url, a user and a token")
)

// initCommand writes the configuration file.
func initCommand() Command {
	const name = "init"

	return Command{
		Name:    name,
		Summary: "Write the configuration file. Run this yourself; it asks for a token.",
		Run: func(_ context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			force := flags.Bool("force", false, "replace an existing configuration file")
			nonInteractive := flags.Bool("non-interactive", false,
				"read the answers from a stdin that is not a terminal, in the order "+
					"url, user, token; without this a non-terminal stdin is refused")

			if err := parse(flags, args); err != nil {
				return err
			}

			return runInit(deps.Options, *force, *nonInteractive)
		},
	}
}

// runInit asks for the configuration and writes it.
func runInit(opts *Options, force, nonInteractive bool) error {
	if err := checkInitCanRun(opts, force, nonInteractive); err != nil {
		return err
	}

	file, err := askForConfig(opts)
	if err != nil {
		return err
	}

	// Validating here means a typo in the host is reported now rather than on
	// the first real command, and it costs nothing: it is the same validator
	// every other invocation runs.
	if _, _, err = config.LoadWithFile(noEnvironment, &file); err != nil {
		return fmt.Errorf("%w: %w", ErrNotConfigured, err)
	}

	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the configuration: %w", err)
	}

	if writeErr := writeConfigFile(opts, opts.ConfigPath, append(body, '\n')); writeErr != nil {
		return writeErr
	}

	// The token is not repeated back. It went in; saying so is enough.
	return report(opts.Stderr, "wrote "+opts.ConfigPath+"\n"+
		"  url   "+file.URL+"\n"+
		"  user  "+file.User+"\n"+
		"  token stored\n")
}

// checkInitCanRun refuses before anything is read, so that the two ways this
// goes wrong cost nothing.
//
// The stdin check is the one that matters. init is a command a person runs;
// an agent that runs it from a pipe nobody is typing into would otherwise
// block on the first read and hang until the session is killed. Refusing is
// recoverable, and the message says who should run it.
func checkInitCanRun(opts *Options, force, nonInteractive bool) error {
	if opts.ConfigPath == "" {
		return fmt.Errorf("%w: set %s to say where it should go", ErrNoConfigPath, config.EnvConfigPath)
	}

	if !opts.Interactive && !nonInteractive {
		return fmt.Errorf("%w: run `%s init` yourself in a terminal, "+
			"or pass -non-interactive to pipe the url, user and token into it",
			ErrNotInteractive, ProgramName)
	}

	if !force && fileExists(opts) {
		return fmt.Errorf("%w at %s: pass -force to replace it", ErrConfigExists, opts.ConfigPath)
	}

	return nil
}

// askForConfig reads the three values init writes.
//
// Everything comes from stdin, including the token. There is no -token flag on
// purpose: a flag would put the credential in the process arguments and in
// shell history, and "the token never appears in `ps`" is a claim this project
// makes and intends to keep.
//
// The token does echo as it is typed. Turning terminal echo off needs
// golang.org/x/term; the README says so rather than this pretending it is
// hidden.
func askForConfig(opts *Options) (config.File, error) {
	var file config.File

	// A slice rather than three copies of the same four lines, and in the
	// order a person expects to be asked.
	questions := []question{
		{into: &file.URL, ask: "Gerrit URL (https://gerrit.example.com): "},
		{into: &file.User, ask: "Gerrit username: "},
		{into: &file.Token, ask: "Auth token (Settings -> HTTP Credentials): "},
	}

	reader := bufio.NewReader(opts.Stdin)

	for _, q := range questions {
		value, err := askFor(opts, reader, q.ask)
		if err != nil {
			return config.File{}, err
		}

		*q.into = value
	}

	if err := checkAnswers(&file); err != nil {
		return config.File{}, err
	}

	return file, nil
}

// question is one value init asks for and where the answer goes.
type question struct {
	into *string
	ask  string
}

// askFor reads one line, prompting first when there is a person to prompt.
func askFor(opts *Options, reader *bufio.Reader, question string) (string, error) {
	if opts.Interactive {
		if _, err := io.WriteString(opts.Stderr, question); err != nil {
			return "", fmt.Errorf("writing the prompt: %w", err)
		}
	}

	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading the answer: %w", err)
	}

	return strings.TrimSpace(line), nil
}

// checkAnswers reports everything missing at once, the way config.Load reports
// variables, so that a second attempt can supply all of it.
func checkAnswers(file *config.File) error {
	var missing []string

	if file.URL == "" {
		missing = append(missing, "url")
	}

	if file.User == "" {
		missing = append(missing, "user")
	}

	if file.Token == "" {
		missing = append(missing, "token")
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: nothing was given for %s", ErrIncompleteAnswers, strings.Join(missing, ", "))
}

// writeConfigFile writes the file through a sibling temp file and a rename.
//
// os.WriteFile honours its permission argument only when it creates the file,
// so -force over one somebody had left world-readable would leave it that way.
// A fresh inode cannot inherit that, and a write that fails halfway cannot
// leave a truncated configuration behind either.
func writeConfigFile(opts *Options, path string, body []byte) error {
	directory := filepath.Dir(path)

	if err := opts.MkdirAll(directory, configDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", directory, err)
	}

	temporary := path + ".tmp"

	if err := opts.WriteFile(temporary, body, configFilePerm); err != nil {
		return fmt.Errorf("writing %s: %w", temporary, err)
	}

	if err := opts.Rename(temporary, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", temporary, path, err)
	}

	return nil
}

// report writes a note to stderr. init produces no rendered output, so stdout
// stays empty for it.
func report(stderr io.Writer, body string) error {
	if _, err := io.WriteString(stderr, body); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

// noEnvironment is a lookup that answers nothing, so that init validates what
// it is about to write rather than what the current shell happens to hold.
func noEnvironment(string) (string, bool) {
	return "", false
}
