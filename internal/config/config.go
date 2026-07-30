// Package config loads the server's configuration from the environment.
//
// Everything is configured with environment variables so that it lives in the
// MCP client's config file and nowhere else -- there is no config file of our
// own for a token to leak into.
package config

import (
	"errors"
	"log/slog"
	"net/url"
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

// errNotImplemented is returned by the stub until [Load] is written.
var errNotImplemented = errors.New("not implemented")

// Load reads and validates the configuration.
//
// lookup has the signature of [os.LookupEnv]; passing it explicitly keeps this
// function pure so tests never have to mutate the process environment.
//
// Every problem found is reported, not just the first, so that a misconfigured
// client can be fixed in one pass rather than one variable per attempt.
func Load(lookup func(string) (string, bool)) (Config, error) {
	_ = lookup

	return Config{}, errNotImplemented
}
