package config_test

import (
	"errors"
	"log/slog"
	"maps"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// urlComparer compares URLs by their string form. url.URL contains a *Userinfo
// with unexported fields, which cmp refuses to walk.
var urlComparer = cmp.Comparer(func(a, b *url.URL) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.String() == b.String()
})

// lookupFrom adapts a map to the os.LookupEnv signature.
func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]

		return value, ok
	}
}

// required is the minimum environment for a successful load.
func required() map[string]string {
	return map[string]string{
		config.EnvURL:   "https://gerrit.example.com",
		config.EnvUser:  "alice",
		config.EnvToken: "s3cret",
	}
}

// with returns the required environment plus the given overrides.
func with(overrides map[string]string) map[string]string {
	env := required()
	maps.Copy(env, overrides)

	return env
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	got, err := config.Load(lookupFrom(required()))
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	want := config.Config{
		BaseURL:    mustParse(t, "https://gerrit.example.com"),
		User:       "alice",
		Token:      "s3cret",
		AllowWrite: false,
		Timeout:    config.DefaultTimeout,
		LogLevel:   config.DefaultLogLevel,
	}

	if diff := cmp.Diff(want, got, urlComparer); diff != "" {
		t.Errorf("Load() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadValid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env  map[string]string
		want config.Config
	}{
		"trailing slash is trimmed": {
			env: with(map[string]string{config.EnvURL: "https://gerrit.example.com/"}),
			want: config.Config{
				BaseURL:  mustParse(t, "https://gerrit.example.com"),
				User:     "alice",
				Token:    "s3cret",
				Timeout:  config.DefaultTimeout,
				LogLevel: config.DefaultLogLevel,
			},
		},
		"sub path is preserved": {
			env: with(map[string]string{config.EnvURL: "https://example.com/gerrit/"}),
			want: config.Config{
				BaseURL:  mustParse(t, "https://example.com/gerrit"),
				User:     "alice",
				Token:    "s3cret",
				Timeout:  config.DefaultTimeout,
				LogLevel: config.DefaultLogLevel,
			},
		},
		"allow write true": {
			env: with(map[string]string{config.EnvAllowWrite: "true"}),
			want: config.Config{
				BaseURL:    mustParse(t, "https://gerrit.example.com"),
				User:       "alice",
				Token:      "s3cret",
				AllowWrite: true,
				Timeout:    config.DefaultTimeout,
				LogLevel:   config.DefaultLogLevel,
			},
		},
		"allow write accepts 1": {
			env: with(map[string]string{config.EnvAllowWrite: "1"}),
			want: config.Config{
				BaseURL:    mustParse(t, "https://gerrit.example.com"),
				User:       "alice",
				Token:      "s3cret",
				AllowWrite: true,
				Timeout:    config.DefaultTimeout,
				LogLevel:   config.DefaultLogLevel,
			},
		},
		"allow write false": {
			env: with(map[string]string{config.EnvAllowWrite: "false"}),
			want: config.Config{
				BaseURL:  mustParse(t, "https://gerrit.example.com"),
				User:     "alice",
				Token:    "s3cret",
				Timeout:  config.DefaultTimeout,
				LogLevel: config.DefaultLogLevel,
			},
		},
		"custom timeout": {
			env: with(map[string]string{config.EnvTimeout: "5s"}),
			want: config.Config{
				BaseURL:  mustParse(t, "https://gerrit.example.com"),
				User:     "alice",
				Token:    "s3cret",
				Timeout:  5 * time.Second,
				LogLevel: config.DefaultLogLevel,
			},
		},
		"log level debug": {
			env: with(map[string]string{config.EnvLogLevel: "debug"}),
			want: config.Config{
				BaseURL:  mustParse(t, "https://gerrit.example.com"),
				User:     "alice",
				Token:    "s3cret",
				Timeout:  config.DefaultTimeout,
				LogLevel: slog.LevelDebug,
			},
		},
		"log level is case insensitive": {
			env: with(map[string]string{config.EnvLogLevel: "WARN"}),
			want: config.Config{
				BaseURL:  mustParse(t, "https://gerrit.example.com"),
				User:     "alice",
				Token:    "s3cret",
				Timeout:  config.DefaultTimeout,
				LogLevel: slog.LevelWarn,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.Load(lookupFrom(test.env))
			if err != nil {
				t.Fatalf("Load() returned an unexpected error: %v", err)
			}

			if diff := cmp.Diff(test.want, got, urlComparer); diff != "" {
				t.Errorf("Load() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env map[string]string
		// wantCause is the sentinel the error must match. ErrMissing and
		// ErrInvalid are exported precisely so a caller can tell "you did not
		// set this" from "what you set is unusable", and the chain that
		// carries them -- two %w verbs in one Errorf, under an errors.Join --
		// is exactly the kind that breaks without anything failing.
		wantCause error
		// wantMentions are substrings the error must contain, so that the
		// message actually tells the user which variable to fix.
		wantMentions []string
	}{
		"missing url": {
			env:          map[string]string{config.EnvUser: "alice", config.EnvToken: "s3cret"},
			wantCause:    config.ErrMissing,
			wantMentions: []string{config.EnvURL},
		},
		"missing user": {
			env: map[string]string{
				config.EnvURL:   "https://gerrit.example.com",
				config.EnvToken: "s3cret",
			},
			wantCause:    config.ErrMissing,
			wantMentions: []string{config.EnvUser},
		},
		"missing token": {
			env: map[string]string{
				config.EnvURL:  "https://gerrit.example.com",
				config.EnvUser: "alice",
			},
			wantCause:    config.ErrMissing,
			wantMentions: []string{config.EnvToken},
		},
		"every missing variable is reported at once": {
			env:          map[string]string{},
			wantCause:    config.ErrMissing,
			wantMentions: []string{config.EnvURL, config.EnvUser, config.EnvToken},
		},
		"empty value counts as missing": {
			env:          with(map[string]string{config.EnvUser: ""}),
			wantCause:    config.ErrMissing,
			wantMentions: []string{config.EnvUser},
		},
		"url without a scheme": {
			env:          with(map[string]string{config.EnvURL: "gerrit.example.com"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvURL},
		},
		"url with a non http scheme": {
			env:          with(map[string]string{config.EnvURL: "ftp://gerrit.example.com"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvURL},
		},
		"url carrying a fragment": {
			env:          with(map[string]string{config.EnvURL: "https://gerrit.example.com/#/q/status:open"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvURL},
		},
		"url carrying a query string": {
			env:          with(map[string]string{config.EnvURL: "https://gerrit.example.com/?foo=1"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvURL},
		},
		"url carrying credentials": {
			//nolint:gosec // G101: the fake credential is the point of this case
			env:          with(map[string]string{config.EnvURL: "https://user:hunter2@gerrit.example.com"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvURL},
		},
		"unparsable url": {
			env:          with(map[string]string{config.EnvURL: "https://gerrit example.com"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvURL},
		},
		"unparsable timeout": {
			env:          with(map[string]string{config.EnvTimeout: "soon"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvTimeout},
		},
		"zero timeout": {
			env:          with(map[string]string{config.EnvTimeout: "0s"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvTimeout},
		},
		"negative timeout": {
			env:          with(map[string]string{config.EnvTimeout: "-1s"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvTimeout},
		},
		"unknown allow write value": {
			env:          with(map[string]string{config.EnvAllowWrite: "yes please"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvAllowWrite},
		},
		"unknown log level": {
			env:          with(map[string]string{config.EnvLogLevel: "chatty"}),
			wantCause:    config.ErrInvalid,
			wantMentions: []string{config.EnvLogLevel},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load(lookupFrom(test.env))
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}

			if !errors.Is(err, test.wantCause) {
				t.Errorf("Load() error = %v, want it to match %v", err, test.wantCause)
			}

			for _, mention := range test.wantMentions {
				if !strings.Contains(err.Error(), mention) {
					t.Errorf("Load() error %q does not mention %q", err, mention)
				}
			}

			// A URL may carry a password. Naming the variable is the whole
			// point of these errors; echoing its value is not.
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("Load() error leaks a credential: %q", err)
			}
		})
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}

	return parsed
}
