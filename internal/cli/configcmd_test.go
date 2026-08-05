package cli

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// reportOptions returns options for a config-report test.
func reportOptions(t *testing.T, env map[string]string, file string) (*Options, *strings.Builder) {
	t.Helper()

	var stdout, stderr strings.Builder

	opts := &Options{
		Lookup:     lookupFrom(env),
		Stdout:     &stdout,
		Stderr:     &stderr,
		ConfigPath: testConfigPath,
	}

	if file != "" {
		opts.ReadFile = func(string) ([]byte, error) { return []byte(file), nil }
		opts.Stat = func(string) (fs.FileInfo, error) { return nil, nil } //nolint:nilnil // only the error is read
	} else {
		opts.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrNotExist }
		opts.Stat = func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
	}

	return opts, &stdout
}

// TestConfigCommandNeverPrintsTheToken is the one this command must never get
// wrong. Its whole purpose is to be run and pasted somewhere when something is
// broken, which is precisely the wrong place for a credential.
func TestConfigCommandNeverPrintsTheToken(t *testing.T) {
	t.Parallel()

	const token = "hunter2"

	tests := map[string]struct {
		env  map[string]string
		file string
	}{
		"from the environment": {
			env: map[string]string{config.EnvToken: token},
		},
		"from the file": {
			file: `{"url":"https://gerrit.example.com","user":"alice","token":"` + token + `"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, stdout := reportOptions(t, test.env, test.file)

			if err := writeConfigReport(opts); err != nil {
				t.Fatalf("writeConfigReport() returned an unexpected error: %v", err)
			}

			if strings.Contains(stdout.String(), token) {
				t.Errorf("the report prints the token:\n%s", stdout.String())
			}

			if !strings.Contains(stdout.String(), tokenMask) {
				t.Errorf("the report does not say a token is set:\n%s", stdout.String())
			}
		})
	}
}

func TestConfigCommandReportsWhereEachValueCameFrom(t *testing.T) {
	t.Parallel()

	file := `{"url":"https://file.example.com","user":"from-file","token":"t"}`
	env := map[string]string{config.EnvUser: "from-env"}

	opts, stdout := reportOptions(t, env, file)

	if err := writeConfigReport(opts); err != nil {
		t.Fatalf("writeConfigReport() returned an unexpected error: %v", err)
	}

	got := stdout.String()

	tests := map[string]string{
		config.EnvURL:        "(file)",
		config.EnvUser:       "(environment)",
		config.EnvTimeout:    "(default)",
		config.EnvAllowWrite: "(default)",
	}

	for key, want := range tests {
		line := lineFor(t, got, key)
		if !strings.Contains(line, want) {
			t.Errorf("%s reported as %q, want %s", key, line, want)
		}
	}

	// The environment winning is the rule the whole layering exists to
	// implement; saying so is what makes the report worth running.
	if !strings.Contains(lineFor(t, got, config.EnvUser), "from-env") {
		t.Errorf("%s does not show the environment's value:\n%s", config.EnvUser, got)
	}

	if !strings.Contains(got, testConfigPath) {
		t.Errorf("the report does not name the configuration file:\n%s", got)
	}
}

// TestConfigCommandReportsAnUnconfiguredHost covers the case an agent runs
// this in: nothing is set, and the answer has to name the fix rather than
// merely list six blanks.
func TestConfigCommandReportsAnUnconfiguredHost(t *testing.T) {
	t.Parallel()

	opts, stdout := reportOptions(t, nil, "")

	if err := writeConfigReport(opts); err != nil {
		t.Fatalf("writeConfigReport() returned an unexpected error: %v", err)
	}

	got := stdout.String()

	for _, want := range []string{config.EnvURL, config.EnvUser, config.EnvToken, ProgramName + " init"} {
		if !strings.Contains(got, want) {
			t.Errorf("the report does not mention %s:\n%s", want, got)
		}
	}

	if !strings.Contains(got, "not created yet") {
		t.Errorf("the report does not say the file is absent:\n%s", got)
	}
}

// TestConfigCommandSurvivesAnUnreadableFile pins that the report still answers
// when the configuration is the thing that is broken. Refusing here would be
// silent in the one case anyone runs it for.
func TestConfigCommandSurvivesAnUnreadableFile(t *testing.T) {
	t.Parallel()

	opts, stdout := reportOptions(t, map[string]string{config.EnvURL: "https://gerrit.example.com"},
		`{"tokn":"typo"}`)

	if err := writeConfigReport(opts); err != nil {
		t.Fatalf("writeConfigReport() returned an unexpected error: %v", err)
	}

	got := stdout.String()

	if !strings.Contains(got, "unreadable") {
		t.Errorf("the report does not say the file could not be read:\n%s", got)
	}

	// What the environment does supply is still reported, because that is what
	// tells the reader how much of the problem the file is.
	if !strings.Contains(lineFor(t, got, config.EnvURL), "gerrit.example.com") {
		t.Errorf("the report drops the environment's value:\n%s", got)
	}
}

// lineFor returns the reported line for one variable.
func lineFor(t *testing.T, report, key string) string {
	t.Helper()

	for line := range strings.SplitSeq(report, "\n") {
		if strings.Contains(line, key) {
			return line
		}
	}

	t.Fatalf("the report has no line for %s:\n%s", key, report)

	return ""
}
