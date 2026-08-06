// Package config loads the configuration for both frontends.
//
// gerrit-mcp-server is configured with environment variables alone, so its
// credentials live in the MCP client's config and nowhere else: [Load] never
// touches a disk. gerrit-cli has no client config to inherit from, so it also
// reads a JSON file under the OS configuration directory -- see [LoadWithFile].
//
// The environment always wins over the file, and it wins by layering at the
// lookup rather than by merging two Configs, so a value read from a file goes
// through the same parser, the same validation and the same error message an
// environment variable does. There is one parser here, not two.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Environment variable names read by [Load].
const (
	EnvURL        = "GERRIT_URL"
	EnvUser       = "GERRIT_USER"
	EnvToken      = "GERRIT_TOKEN"
	EnvAllowWrite = "GERRIT_ALLOW_WRITE"
	EnvTimeout    = "GERRIT_TIMEOUT"
	EnvLogLevel   = "GERRIT_LOG_LEVEL"
)

// Defaults applied when the corresponding variable is unset.
const (
	DefaultTimeout  = 30 * time.Second
	DefaultLogLevel = slog.LevelInfo
)

// Sentinel causes, so callers can tell a missing variable from a bad one.
var (
	// ErrMissing reports a required variable that is unset or blank.
	ErrMissing = errors.New("not set")
	// ErrInvalid reports a variable that holds an unusable value.
	ErrInvalid = errors.New("invalid value")
)

// Config is the validated configuration for one server process.
type Config struct {
	// BaseURL is the Gerrit host, without a trailing slash.
	BaseURL *url.URL
	// User is the Gerrit account name used for HTTP Basic auth.
	User string
	// Token is the auth token from the account's HTTP Credentials page.
	Token string
	// AllowWrite reports whether tools that modify Gerrit may be registered.
	AllowWrite bool
	// Timeout bounds a single HTTP request to Gerrit.
	Timeout time.Duration
	// LogLevel is the minimum level written to stderr.
	LogLevel slog.Level
}

// Load reads and validates the configuration.
//
// lookup has the signature of [os.LookupEnv]; passing it explicitly keeps this
// function pure so tests never have to mutate the process environment.
//
// Every problem found is reported, not just the first, so that a misconfigured
// client can be fixed in one pass rather than one variable per attempt.
func Load(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{
		User:     value(lookup, EnvUser),
		Token:    value(lookup, EnvToken),
		Timeout:  DefaultTimeout,
		LogLevel: DefaultLogLevel,
	}

	var errs []error

	if cfg.User == "" {
		errs = append(errs, missing(EnvUser))
	}

	if cfg.Token == "" {
		errs = append(errs, missing(EnvToken))
	}

	baseURL, err := parseBaseURL(value(lookup, EnvURL))
	if err != nil {
		errs = append(errs, err)
	}

	cfg.BaseURL = baseURL

	errs = append(errs, loadOptions(lookup, &cfg)...)

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

	return cfg, nil
}

// loadOptions applies the variables that have a usable default, leaving cfg
// untouched for any that fails to parse.
func loadOptions(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error

	if raw := value(lookup, EnvAllowWrite); raw != "" {
		allowWrite, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, invalid(EnvAllowWrite, raw, err))
		} else {
			cfg.AllowWrite = allowWrite
		}
	}

	if raw := value(lookup, EnvTimeout); raw != "" {
		timeout, err := parseTimeout(raw)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.Timeout = timeout
		}
	}

	if raw := value(lookup, EnvLogLevel); raw != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(raw)); err != nil {
			errs = append(errs, invalid(EnvLogLevel, raw, err))
		} else {
			cfg.LogLevel = level
		}
	}

	return errs
}

// value returns the trimmed value of key. An unset variable and one holding
// only whitespace are the same thing: absent.
func value(lookup func(string) (string, bool), key string) string {
	raw, ok := lookup(key)
	if !ok {
		return ""
	}

	return strings.TrimSpace(raw)
}

// parseBaseURL validates the Gerrit host and strips any trailing slash so that
// callers can join paths onto it without doubling the separator.
func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, missing(EnvURL)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, invalid(EnvURL, raw, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, invalidf(EnvURL, raw, "scheme must be http or https")
	}

	if parsed.Host == "" {
		return nil, invalidf(EnvURL, raw, "no host")
	}

	// A URL pasted out of a browser carries the UI fragment or a query string.
	// endpoint() concatenates onto this value, so anything after the path
	// would land in the middle of every request it builds.
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, invalidf(EnvURL, raw, "must be a bare host and path, with no query or fragment")
	}

	// Reported without the value: userinfo is where a password would be.
	if parsed.User != nil {
		return nil, fmt.Errorf("%s: %w: must not carry credentials, use %s and %s",
			EnvURL, ErrInvalid, EnvUser, EnvToken)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")

	return parsed, nil
}

// parseTimeout rejects durations that would make every request fail.
func parseTimeout(raw string) (time.Duration, error) {
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, invalid(EnvTimeout, raw, err)
	}

	if timeout <= 0 {
		return 0, invalidf(EnvTimeout, raw, "must be positive")
	}

	return timeout, nil
}

// missing names the variable the user has to set.
func missing(key string) error {
	return fmt.Errorf("%s: %w", key, ErrMissing)
}

// invalid quotes the offending value alongside the parser's own complaint.
//
// Only ever called for variables whose value is safe to echo. GERRIT_TOKEN is
// deliberately not one of them -- it can only ever be reported as missing.
func invalid(key, raw string, cause error) error {
	return fmt.Errorf("%s=%q: %w: %w", key, raw, ErrInvalid, cause)
}

// invalidf is [invalid] for the cases where we are the one objecting.
func invalidf(key, raw, reason string) error {
	return fmt.Errorf("%s=%q: %w: %s", key, raw, ErrInvalid, reason)
}
