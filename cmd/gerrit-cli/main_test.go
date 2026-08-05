package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/cli"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// lookupFrom returns an os.LookupEnv stand-in reading from env.
func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]

		return value, ok
	}
}

// blankConfiguration empties the variables a developer's own shell is likely
// to have set, so that a test of the unconfigured path does not accidentally
// find a real Gerrit.
func blankConfiguration(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		config.EnvURL, config.EnvUser, config.EnvToken, config.EnvConfigPath,
	} {
		t.Setenv(key, "")
	}
}

func TestRunPrintsTheVersionOnStdout(t *testing.T) {
	var stdout, stderr strings.Builder

	blankConfiguration(t)

	if err := run(t.Context(), []string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() returned an unexpected error: %v", err)
	}

	// The stamps are shared with gerrit-mcp-server, so the name is the only
	// thing distinguishing what this binary reports from what that one does.
	if !strings.HasPrefix(stdout.String(), cli.ProgramName+" ") {
		t.Errorf("stdout = %q, want it to name %s", stdout.String(), cli.ProgramName)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want the version on stdout alone", stderr.String())
	}
}

func TestRunPrintsTheHelpOnStdout(t *testing.T) {
	var stdout, stderr strings.Builder

	blankConfiguration(t)

	if err := run(t.Context(), []string{"-h"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() returned an unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "query-changes") {
		t.Errorf("stdout = %q, want the command list", stdout.String())
	}
}

func TestRunReportsAnUnconfiguredEnvironment(t *testing.T) {
	var stdout, stderr strings.Builder

	blankConfiguration(t)

	err := run(t.Context(), []string{"query-changes", "-query", "is:open"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() succeeded with no configuration, want it to report that")
	}

	// Every missing variable is named at once, so a shell can be fixed in one
	// pass rather than one variable per attempt.
	for _, want := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}

	if cli.ExitCode(err) != cli.ExitNotConfigured {
		t.Errorf("ExitCode() = %d, want %d", cli.ExitCode(err), cli.ExitNotConfigured)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing but rendered output there", stdout.String())
	}
}

func TestRunReportsAnUnknownCommand(t *testing.T) {
	var stdout, stderr strings.Builder

	blankConfiguration(t)

	err := run(t.Context(), []string{"list-everything"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() succeeded on an unknown command, want it reported")
	}

	if !errors.Is(err, cli.ErrUnknownCommand) {
		t.Errorf("error = %v, want it to match %v", err, cli.ErrUnknownCommand)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

func TestConfigPathPrefersTheOverride(t *testing.T) {
	t.Parallel()

	env := map[string]string{config.EnvConfigPath: "  /custom/gerrit.json  "}

	got := configPath(lookupFrom(env), func() (string, error) { return "/unused", nil })
	if want := "/custom/gerrit.json"; got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestConfigPathFallsBackToTheConfigDirectory(t *testing.T) {
	t.Parallel()

	got := configPath(lookupFrom(nil), func() (string, error) {
		return filepath.Join("root", "cfg"), nil
	})

	want := filepath.Join("root", "cfg", config.DirName, config.FileName)
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

// TestConfigPathToleratesAnUnresolvableHome covers the host agents actually
// run on: a container with no HOME and no %AppData%. There is no file to read
// there, which is not a reason to refuse to run -- the environment alone is a
// complete configuration, and every command works from it.
func TestConfigPathToleratesAnUnresolvableHome(t *testing.T) {
	t.Parallel()

	broken := errors.New("neither $XDG_CONFIG_HOME nor $HOME are defined")

	if got := configPath(lookupFrom(nil), func() (string, error) { return "", broken }); got != "" {
		t.Errorf("configPath() = %q, want an empty path rather than a guess", got)
	}
}

// TestConfigPathIgnoresABlankOverride keeps `GERRIT_CONFIG=` from pointing the
// binary at a file named the empty string.
func TestConfigPathIgnoresABlankOverride(t *testing.T) {
	t.Parallel()

	env := map[string]string{config.EnvConfigPath: "   "}

	got := configPath(lookupFrom(env), func() (string, error) { return "cfg", nil })
	if want := filepath.Join("cfg", config.DirName, config.FileName); got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}
