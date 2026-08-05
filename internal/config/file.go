package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// EnvConfigPath overrides where gerrit-cli looks for its configuration file.
//
// It is read from the environment and only from the environment: a file cannot
// say where it is.
const EnvConfigPath = "GERRIT_CONFIG"

// Where gerrit-cli keeps its configuration under the OS config directory.
const (
	// DirName is the directory holding gerrit-cli's configuration.
	DirName = "gerrit-cli"
	// FileName is gerrit-cli's configuration file.
	FileName = "config.json"
)

// Source reports where a configuration value came from.
type Source int

// The places a value can come from, in increasing order of precedence.
const (
	// SourceDefault is a value nothing supplied, left at its default.
	SourceDefault Source = iota
	// SourceFile is a value read from the configuration file.
	SourceFile
	// SourceEnvironment is a value read from the environment.
	SourceEnvironment
)

// String names the source the way `gerrit-cli config` reports it.
func (s Source) String() string {
	switch s {
	case SourceEnvironment:
		return "environment"
	case SourceFile:
		return "file"
	case SourceDefault:
		return "default"
	default:
		return "unknown"
	}
}

// Sources reports where each value came from, keyed by environment variable
// name.
//
// Returned alongside the Config rather than folded into it because it is what
// `gerrit-cli config` exists to print, and it is most useful in exactly the
// case [Load] refuses: a configuration that is missing something.
type Sources map[string]Source

// Of reports where the value for key came from.
//
// A key nothing answered reads as [SourceDefault], which is what it is.
func (s Sources) Of(key string) Source {
	return s[key]
}

// File is gerrit-cli's configuration file.
//
// Every field mirrors one environment variable, and every field but one is a
// string for the same reason the environment is: the value goes through the
// same parser, so a bad duration written here fails exactly as a bad
// GERRIT_TIMEOUT does, with the same message.
type File struct {
	// AllowWrite reports whether commands that modify Gerrit may run.
	//
	// A pointer so that an absent key and an explicit false are
	// distinguishable: absent must decline to answer rather than assert false,
	// or the file would override an environment that says otherwise.
	//
	// `gerrit-cli init` never writes this key. Persisting it arms every future
	// invocation on the machine, which is a decision worth making by hand.
	AllowWrite *bool `json:"allow_write,omitempty"`
	// URL is the Gerrit host.
	URL string `json:"url,omitempty"`
	// User is the Gerrit account name.
	User string `json:"user,omitempty"`
	// Token is the auth token from the account's HTTP Credentials page.
	Token string `json:"token,omitempty"`
	// Timeout bounds a single HTTP request, as a Go duration.
	Timeout string `json:"timeout,omitempty"`
	// LogLevel is the minimum level written to stderr.
	LogLevel string `json:"log_level,omitempty"`
}

// values maps the file onto the environment variable names [Load] reads.
//
// A field the file leaves blank is absent from the result rather than present
// and empty, so the layered lookup can tell "the file does not answer this"
// from "the file answers with nothing".
//
// A nil receiver is a file that was never read, and answers nothing.
func (f *File) values() map[string]string {
	values := make(map[string]string)

	if f == nil {
		return values
	}

	set := func(key, raw string) {
		if strings.TrimSpace(raw) != "" {
			values[key] = raw
		}
	}

	set(EnvURL, f.URL)
	set(EnvUser, f.User)
	set(EnvToken, f.Token)
	set(EnvTimeout, f.Timeout)
	set(EnvLogLevel, f.LogLevel)

	if f.AllowWrite != nil {
		values[EnvAllowWrite] = strconv.FormatBool(*f.AllowWrite)
	}

	return values
}

// ReadFile parses gerrit-cli's configuration file.
//
// read has the signature of [os.ReadFile]; passing it explicitly keeps this
// package free of the filesystem, exactly as [Load] is free of the environment.
//
// A file that is not there is not an error. The environment alone is a
// complete configuration, and it is the only one gerrit-mcp-server has.
func ReadFile(read func(string) ([]byte, error), path string) (File, error) {
	raw, err := read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}

	if err != nil {
		return File{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var file File

	decoder := json.NewDecoder(bytes.NewReader(raw))
	// A misspelled key is what this catches. "tokn" would otherwise be dropped
	// in silence and surface later as "GERRIT_TOKEN: not set", sending the
	// reader to the environment to fix a typo that is in a file.
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	return file, nil
}

// LoadWithFile reads and validates the configuration, filling anything the
// environment does not set from file.
//
// The environment always wins. Layering happens at the lookup rather than by
// merging two Configs, so every parser, every default and every error message
// applies identically to a value that came from a file -- there is one
// validator here, not two.
//
// The sources are returned even when the configuration is invalid: reporting
// where each value came from is most useful precisely when one is missing.
//
// A nil file is a host with no configuration file, which is not an error --
// the environment alone is a complete configuration.
func LoadWithFile(lookup func(string) (string, bool), file *File) (Config, Sources, error) {
	sources := make(Sources)

	cfg, err := Load(withFallback(lookup, file.values(), sources))

	return cfg, sources, err
}

// withFallback returns a lookup that answers from fallback whenever the
// environment does not, recording in sources which of the two answered.
//
// A variable set to the empty string counts as unset, matching what [value]
// already does with one: a blank GERRIT_URL in the environment has to fall
// through to the file rather than shadow it with nothing.
func withFallback(
	lookup func(string) (string, bool),
	fallback map[string]string,
	sources Sources,
) func(string) (string, bool) {
	return func(key string) (string, bool) {
		if raw, ok := lookup(key); ok && strings.TrimSpace(raw) != "" {
			sources[key] = SourceEnvironment

			return raw, true
		}

		raw, ok := fallback[key]
		if !ok {
			return "", false
		}

		sources[key] = SourceFile

		return raw, true
	}
}

// DefaultPath returns gerrit-cli's configuration file path under the OS
// configuration directory.
//
// userConfigDir has the signature of [os.UserConfigDir], which resolves to
// %AppData% on Windows, ~/Library/Application Support on macOS and
// $XDG_CONFIG_HOME or ~/.config elsewhere. Hardcoding any one of those would be
// wrong on the other two.
//
// It fails on a host with none of them set -- a bare container, or an agent
// sandbox -- so callers must treat that as fatal only where a path is actually
// required. Every command but init works from the environment alone.
func DefaultPath(userConfigDir func() (string, error)) (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating the user configuration directory: %w", err)
	}

	return filepath.Join(dir, DirName, FileName), nil
}
