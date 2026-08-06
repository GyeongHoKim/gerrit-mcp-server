package cli

import (
	"errors"
	"flag"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// Exit statuses gerrit-cli reports.
//
// Each one names a different thing the caller should do about it, which is the
// criterion here rather than fidelity to HTTP. An agent driving this binary
// decides its next move from the status before it reads a word of the message,
// so two failures needing the same response share a status and two needing
// different responses never do.
const (
	// ExitOK reports success.
	ExitOK = 0
	// ExitFailure reports a failure with no more specific status.
	ExitFailure = 1
	// ExitUsage reports a command line that could not be accepted. Fix the
	// arguments.
	ExitUsage = 2
	// ExitNotConfigured reports missing or unusable configuration. Run
	// `gerrit-cli init`, or set GERRIT_URL, GERRIT_USER and GERRIT_TOKEN.
	ExitNotConfigured = 3
	// ExitNotPermitted reports that the account, or this process, may not do
	// this. Ask a human rather than retrying.
	ExitNotPermitted = 4
	// ExitNotFound reports that Gerrit has no such change, file or comment.
	ExitNotFound = 5
	// ExitConflict reports a change that is not in a state allowing this.
	ExitConflict = 6
)

// exitCodes maps the sentinels a caller acts on to a status.
//
// Scanned in order, so the specific entries precede the general ones. A slice
// rather than a switch because errors.Is is the only correct comparison for a
// wrapped error and a switch cannot express it.
var exitCodes = []struct {
	err  error
	code int
}{
	// Argument problems come first: internal/gerrit validates before it dials,
	// so these are command lines to fix rather than answers from a server.
	{ErrUsage, ExitUsage},
	{ErrUnexpectedArguments, ExitUsage},
	{ErrNoCommand, ExitUsage},
	{ErrUnknownCommand, ExitUsage},
	{gerrit.ErrEmptyChangeID, ExitUsage},
	{gerrit.ErrEmptyQuery, ExitUsage},
	{gerrit.ErrEmptyFilePath, ExitUsage},
	{gerrit.ErrEmptyMessage, ExitUsage},
	{gerrit.ErrEmptyDraftID, ExitUsage},
	{gerrit.ErrEmptyReviewer, ExitUsage},
	{gerrit.ErrIncompleteChange, ExitUsage},

	// init's own refusals are all things to do differently, not things to
	// retry: run it in a terminal, pass -force, answer all three questions.
	{ErrNotInteractive, ExitUsage},
	{ErrConfigExists, ExitUsage},
	{ErrIncompleteAnswers, ExitUsage},

	{ErrNoConfigPath, ExitNotConfigured},
	{ErrNotConfigured, ExitNotConfigured},
	{config.ErrMissing, ExitNotConfigured},
	{config.ErrInvalid, ExitNotConfigured},

	{ErrWriteNotAllowed, ExitNotPermitted},
	{gerrit.ErrUnauthorized, ExitNotPermitted},
	{gerrit.ErrForbidden, ExitNotPermitted},

	{gerrit.ErrNotFound, ExitNotFound},

	{gerrit.ErrConflict, ExitConflict},
	{gerrit.ErrConfirmationRequired, ExitConflict},
	{gerrit.ErrReviewerRejected, ExitConflict},
	{gerrit.ErrReviewRejected, ExitConflict},
}

// ExitCode reports the process status err should produce.
//
// This lives here rather than in main because the mapping is a contract with
// whatever drives the binary -- a skill tells an agent what each status means
// -- and a contract deserves a test.
func ExitCode(err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}

	for _, entry := range exitCodes {
		if errors.Is(err, entry.err) {
			return entry.code
		}
	}

	return ExitFailure
}
