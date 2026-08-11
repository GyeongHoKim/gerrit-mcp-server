package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// doctorCommand reports which Gerrit the configured host is and what that
// means for the commands that need a particular release.
func doctorCommand() Command {
	const name = "doctor"

	return Command{
		Name:    name,
		Summary: "Report the host's Gerrit release and which commands it supports.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)

			// Parsed before the configuration is read, so that `doctor extra`
			// is a bad command line rather than a missing configuration. Every
			// other command does it in this order for the same reason.
			if err := parse(flags, args); err != nil {
				return err
			}

			cfg, err := loadConfig(deps.Options)
			if err != nil {
				return err
			}

			return writeDoctorReport(ctx, deps.Options, cfg)
		},
	}
}

// writeDoctorReport asks the host which Gerrit it is and prints the answer.
//
// Exactly one request is made. Calling each endpoint to see which ones answer
// would be a sweep across twelve operations that modify Gerrit, and abandoning
// a change to find out whether abandoning changes works is not a diagnostic.
// The support matrix is computed from the minimum versions the commands
// already carry.
//
// The request doubles as an authentication check: it goes to /a/ like every
// other call, so a rejected token is reported here rather than on the next
// command someone tries.
//
// A report is a success, including one that could not determine the version.
// This is the command someone runs because something is already wrong, and
// answering "there was an error" to "what is this host?" wastes the turn that
// was meant to fix it. Only a configuration that cannot be loaded and
// credentials the host refuses are returned as failures, because those are the
// two answers that are actionable and are not about versions.
func writeDoctorReport(ctx context.Context, opts *Options, cfg config.Config) error {
	running, versionErr := newGerritClient(cfg).GetServerVersion(ctx)

	if errors.Is(versionErr, gerrit.ErrUnauthorized) || errors.Is(versionErr, gerrit.ErrForbidden) {
		return fmt.Errorf("asking %s which gerrit it is: %w", cfg.BaseURL, versionErr)
	}

	var body strings.Builder

	body.WriteString("Host:    " + cfg.BaseURL.String() + "\n")
	body.WriteString("Account: " + cfg.User + "\n")

	writeDoctorVersion(&body, running, versionErr)
	writeDoctorSupport(&body, running, cfg.AllowWrite)

	return emit(opts.Stdout, body.String())
}

// writeDoctorVersion reports the release, or why it could not be determined.
func writeDoctorVersion(body *strings.Builder, running gerrit.ServerVersion, versionErr error) {
	if versionErr != nil {
		body.WriteString("Version: unknown (" + versionErr.Error() + ")\n")

		return
	}

	body.WriteString("Version: " + running.String() + "\n")
}

// writeDoctorSupport lists what this host can and cannot run.
func writeDoctorSupport(body *strings.Builder, running gerrit.ServerVersion, allowWrite bool) {
	if !allowWrite {
		body.WriteString("Writes:  refused; set " + config.EnvAllowWrite + "=true to allow them\n")
	}

	var unsupported []Command

	for _, command := range GerritCommands() {
		if !command.MinVersion.IsZero() && !running.IsZero() && running.Before(command.MinVersion) {
			unsupported = append(unsupported, command)
		}
	}

	if running.IsZero() {
		body.WriteString("\nEvery command is offered. One this host does not have fails with exit 4\n" +
			"and says which release it needs.\n")

		return
	}

	if len(unsupported) == 0 {
		body.WriteString("\nEvery command works on this release.\n")

		return
	}

	body.WriteString("\nNot available on this release:\n")

	width := nameWidth(unsupported)

	for _, command := range unsupported {
		padding := width - len(command.Name) + columnGap
		body.WriteString("  " + command.Name + strings.Repeat(" ", padding) +
			"needs Gerrit " + command.MinVersion.String() + "+\n")
	}

	body.WriteString("\nEverything else works. These fail with exit 4 rather than doing nothing.\n")
}
