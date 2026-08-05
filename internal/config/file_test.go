package config_test

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// readFrom adapts a map of file contents to the os.ReadFile signature.
func readFrom(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		body, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}

		return []byte(body), nil
	}
}

// requiredFile is the minimum file for a successful load.
func requiredFile() config.File {
	return config.File{
		URL:   "https://gerrit.example.com",
		User:  "alice",
		Token: "s3cret",
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	const path = "/config/gerrit-cli/config.json"

	tests := map[string]struct {
		body string
		want config.File
	}{
		"every field": {
			body: `{"url":"https://gerrit.example.com","user":"alice","token":"s3cret",
			        "timeout":"45s","log_level":"debug","allow_write":true}`,
			want: config.File{
				URL:        "https://gerrit.example.com",
				User:       "alice",
				Token:      "s3cret",
				Timeout:    "45s",
				LogLevel:   "debug",
				AllowWrite: pointerTo(t, true),
			},
		},
		"only the required fields": {
			body: `{"url":"https://gerrit.example.com","user":"alice","token":"s3cret"}`,
			want: requiredFile(),
		},
		// An absent key and an explicit false have to stay distinguishable, or
		// a file that says nothing about writes would veto an environment that
		// enables them.
		"allow_write written false": {
			body: `{"allow_write":false}`,
			want: config.File{AllowWrite: pointerTo(t, false)},
		},
		"an empty object": {
			body: `{}`,
			want: config.File{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.ReadFile(readFrom(map[string]string{path: test.body}), path)
			if err != nil {
				t.Fatalf("ReadFile() returned an unexpected error: %v", err)
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("ReadFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReadFileTreatsAnAbsentFileAsEmpty pins the case gerrit-mcp-server's whole
// design depends on staying quiet: the environment alone is a complete
// configuration, so a file that is not there is not a problem to report.
func TestReadFileTreatsAnAbsentFileAsEmpty(t *testing.T) {
	t.Parallel()

	got, err := config.ReadFile(readFrom(nil), "/config/gerrit-cli/config.json")
	if err != nil {
		t.Fatalf("ReadFile() on an absent file returned an error: %v", err)
	}

	if diff := cmp.Diff(config.File{}, got); diff != "" {
		t.Errorf("ReadFile() mismatch (-want +got):\n%s", diff)
	}
}

func TestReadFileRejects(t *testing.T) {
	t.Parallel()

	const path = "/config/gerrit-cli/config.json"

	tests := map[string]struct {
		body string
		want string
	}{
		"malformed json": {body: `{"url": `, want: "parsing"},
		"a json array":   {body: `["url"]`, want: "parsing"},
		"a wrongly typed field": {
			body: `{"allow_write":"true"}`,
			want: "parsing",
		},
		// The failure this exists for: a typo drops the key in silence and the
		// error arrives as "GERRIT_TOKEN: not set", which sends the reader to
		// the environment to fix a mistake that is in a file.
		"a misspelled key": {body: `{"tokn":"s3cret"}`, want: "parsing"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.ReadFile(readFrom(map[string]string{path: test.body}), path)
			if err == nil {
				t.Fatal("ReadFile() succeeded, want an error")
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("ReadFile() error = %v, want it to mention %q", err, test.want)
			}

			// Naming the file is the whole point: the reader has to know which
			// one to open.
			if !strings.Contains(err.Error(), path) {
				t.Errorf("ReadFile() error = %v, want it to name %s", err, path)
			}
		})
	}
}

func TestReadFileReportsOtherReadFailures(t *testing.T) {
	t.Parallel()

	broken := errors.New("permission denied")

	_, err := config.ReadFile(func(string) ([]byte, error) { return nil, broken }, "/config.json")
	if err == nil {
		t.Fatal("ReadFile() succeeded on an unreadable file, want the failure reported")
	}

	if !errors.Is(err, broken) {
		t.Errorf("ReadFile() error = %v, want it to wrap the reader's own failure", err)
	}
}

func TestLoadWithFileFillsFromFile(t *testing.T) {
	t.Parallel()

	got, sources, err := config.LoadWithFile(lookupFrom(nil), &config.File{
		URL:        "https://gerrit.example.com",
		User:       "alice",
		Token:      "s3cret",
		Timeout:    "45s",
		LogLevel:   "debug",
		AllowWrite: pointerTo(t, true),
	})
	if err != nil {
		t.Fatalf("LoadWithFile() returned an unexpected error: %v", err)
	}

	want := config.Config{
		BaseURL:    mustParse(t, "https://gerrit.example.com"),
		User:       "alice",
		Token:      "s3cret",
		AllowWrite: true,
		Timeout:    45 * time.Second,
		LogLevel:   slog.LevelDebug,
	}

	if diff := cmp.Diff(want, got, urlComparer); diff != "" {
		t.Errorf("LoadWithFile() mismatch (-want +got):\n%s", diff)
	}

	for _, key := range []string{
		config.EnvURL, config.EnvUser, config.EnvToken,
		config.EnvTimeout, config.EnvLogLevel, config.EnvAllowWrite,
	} {
		if source := sources.Of(key); source != config.SourceFile {
			t.Errorf("sources.Of(%s) = %v, want %v", key, source, config.SourceFile)
		}
	}
}

func TestLoadWithFileLetsTheEnvironmentWin(t *testing.T) {
	t.Parallel()

	file := config.File{
		URL:        "https://file.example.com",
		User:       "from-file",
		Token:      "file-token",
		Timeout:    "90s",
		LogLevel:   "error",
		AllowWrite: pointerTo(t, true),
	}

	tests := map[string]struct {
		want func(config.Config) string
		key  string
		env  string
	}{
		"url": {
			key:  config.EnvURL,
			env:  "https://env.example.com",
			want: func(c config.Config) string { return c.BaseURL.String() },
		},
		"user": {
			key:  config.EnvUser,
			env:  "from-env",
			want: func(c config.Config) string { return c.User },
		},
		"token": {
			key:  config.EnvToken,
			env:  "env-token",
			want: func(c config.Config) string { return c.Token },
		},
		"timeout": {
			key:  config.EnvTimeout,
			env:  "15s",
			want: func(c config.Config) string { return c.Timeout.String() },
		},
		"log level": {
			key:  config.EnvLogLevel,
			env:  "warn",
			want: func(c config.Config) string { return c.LogLevel.String() },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, sources, err := config.LoadWithFile(
				lookupFrom(map[string]string{test.key: test.env}), &file,
			)
			if err != nil {
				t.Fatalf("LoadWithFile() returned an unexpected error: %v", err)
			}

			// The log level prints as WARN and the timeout as 15s, so compare
			// case-insensitively against what was asked for.
			if value := test.want(got); !strings.EqualFold(value, test.env) {
				t.Errorf("%s = %q, want the environment's %q", name, value, test.env)
			}

			if source := sources.Of(test.key); source != config.SourceEnvironment {
				t.Errorf("sources.Of(%s) = %v, want %v", test.key, source, config.SourceEnvironment)
			}
		})
	}
}

// TestLoadWithFileTreatsABlankVariableAsAbsent is the subtle one. Load already
// reads a variable set to the empty string as unset; the layered lookup has to
// agree, or exporting GERRIT_URL= would shadow a perfectly good file with
// nothing and report the variable as missing.
func TestLoadWithFileTreatsABlankVariableAsAbsent(t *testing.T) {
	t.Parallel()

	blank := map[string]string{
		config.EnvURL:   "",
		config.EnvUser:  "   ",
		config.EnvToken: "",
	}

	file := requiredFile()

	got, sources, err := config.LoadWithFile(lookupFrom(blank), &file)
	if err != nil {
		t.Fatalf("LoadWithFile() returned an unexpected error: %v", err)
	}

	if got.User != "alice" {
		t.Errorf("User = %q, want the file's %q", got.User, "alice")
	}

	if source := sources.Of(config.EnvURL); source != config.SourceFile {
		t.Errorf("sources.Of(%s) = %v, want %v", config.EnvURL, source, config.SourceFile)
	}
}

// TestLoadWithFileReportsFileValuesLikeVariables pins the point of layering at
// the lookup: a bad value gets the same sentinel and the same message wherever
// it came from, because there is one parser rather than two.
func TestLoadWithFileReportsFileValuesLikeVariables(t *testing.T) {
	t.Parallel()

	tests := map[string]config.File{
		"a bad timeout":   {URL: "https://gerrit.example.com", User: "a", Token: "b", Timeout: "soon"},
		"a bad log level": {URL: "https://gerrit.example.com", User: "a", Token: "b", LogLevel: "chatty"},
		"a bad url":       {URL: "gopher://gerrit.example.com", User: "a", Token: "b"},
	}

	for name, file := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := config.LoadWithFile(lookupFrom(nil), &file)
			if err == nil {
				t.Fatal("LoadWithFile() succeeded, want an error")
			}

			if !errors.Is(err, config.ErrInvalid) {
				t.Errorf("LoadWithFile() error = %v, want it to match %v", err, config.ErrInvalid)
			}
		})
	}
}

func TestLoadWithFileReportsEverythingMissingAtOnce(t *testing.T) {
	t.Parallel()

	_, sources, err := config.LoadWithFile(lookupFrom(nil), nil)
	if err == nil {
		t.Fatal("LoadWithFile() succeeded with nothing configured, want an error")
	}

	for _, want := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadWithFile() error = %v, want it to name %s", err, want)
		}
	}

	// The sources come back even from a refused load: reporting where each
	// value came from is most useful when one is missing, which is the whole
	// reason `gerrit-cli config` exists.
	if source := sources.Of(config.EnvURL); source != config.SourceDefault {
		t.Errorf("sources.Of(%s) = %v, want %v", config.EnvURL, source, config.SourceDefault)
	}
}

// TestLoadIgnoresAnyFile pins the guarantee gerrit-mcp-server relies on: Load
// is the path the MCP server takes and it never reads a disk, so a file sitting
// on the host cannot change what the server is configured with.
func TestLoadIgnoresAnyFile(t *testing.T) {
	t.Parallel()

	_, err := config.Load(lookupFrom(nil))
	if err == nil {
		t.Fatal("Load() succeeded with an empty environment, want an error")
	}

	for _, want := range []string{config.EnvURL, config.EnvUser, config.EnvToken} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load() error = %v, want it to name %s", err, want)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	got, err := config.DefaultPath(func() (string, error) { return filepath.Join("root", "cfg"), nil })
	if err != nil {
		t.Fatalf("DefaultPath() returned an unexpected error: %v", err)
	}

	want := filepath.Join("root", "cfg", config.DirName, config.FileName)
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

// TestDefaultPathReportsAnUnresolvableHome covers the host agents actually run
// on: a container with no HOME, no XDG_CONFIG_HOME and no %AppData%. The
// failure has to be reported rather than guessed at, because every command but
// init works from the environment alone and must not be stopped by it.
func TestDefaultPathReportsAnUnresolvableHome(t *testing.T) {
	t.Parallel()

	broken := errors.New("neither $XDG_CONFIG_HOME nor $HOME are defined")

	_, err := config.DefaultPath(func() (string, error) { return "", broken })
	if err == nil {
		t.Fatal("DefaultPath() succeeded with no config directory, want the failure reported")
	}

	if !errors.Is(err, broken) {
		t.Errorf("DefaultPath() error = %v, want it to wrap the resolver's own failure", err)
	}
}

func TestSourceString(t *testing.T) {
	t.Parallel()

	tests := map[config.Source]string{
		config.SourceDefault:     "default",
		config.SourceFile:        "file",
		config.SourceEnvironment: "environment",
		config.Source(99):        "unknown",
	}

	for source, want := range tests {
		if got := source.String(); got != want {
			t.Errorf("Source(%d).String() = %q, want %q", source, got, want)
		}
	}
}

// TestReadFileReadsARealFile checks the injected reader against the one it
// stands in for. Everything else here fakes os.ReadFile, and a signature that
// only ever meets a fake is a signature nothing has confirmed.
func TestReadFileReadsARealFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), config.FileName)

	body := []byte(`{"url":"https://gerrit.example.com","user":"alice","token":"s3cret"}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing the test configuration: %v", err)
	}

	got, err := config.ReadFile(os.ReadFile, path)
	if err != nil {
		t.Fatalf("ReadFile() returned an unexpected error: %v", err)
	}

	if diff := cmp.Diff(requiredFile(), got); diff != "" {
		t.Errorf("ReadFile() mismatch (-want +got):\n%s", diff)
	}
}

// pointerTo returns a pointer to value, for the fields that need to tell an
// absent key from an explicit one.
func pointerTo[T any](t *testing.T, value T) *T {
	t.Helper()

	return &value
}
