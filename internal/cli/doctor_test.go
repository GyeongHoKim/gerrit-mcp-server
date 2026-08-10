package cli

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// serveVersion answers the version endpoint with version, and 404 elsewhere.
// An empty version leaves it answering 404 too, which is the host that keeps
// the endpoint behind a proxy.
func serveVersion(t *testing.T, version string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/a/config/server/version" || version == "" {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		if _, err := w.Write([]byte(")]}'\n\"" + version + "\"")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	}
}

func TestDoctorReportsAnOldRelease(t *testing.T) {
	t.Parallel()

	got := runCLI(t, serveVersion(t, "2.14.22"), "doctor")
	if got.err != nil {
		t.Fatalf("doctor returned an unexpected error: %v", got.err)
	}

	for _, want := range []string{
		"2.14",
		"set-ready-for-review",
		"set-work-in-progress",
		"revert-submission",
		"needs Gerrit 2.15+",
		"needs Gerrit 3.2+",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout does not mention %q:\n%s", want, got.stdout)
		}
	}
}

func TestDoctorReportsACurrentRelease(t *testing.T) {
	t.Parallel()

	got := runCLI(t, serveVersion(t, "3.14.1"), "doctor")
	if got.err != nil {
		t.Fatalf("doctor returned an unexpected error: %v", got.err)
	}

	if !strings.Contains(got.stdout, "Every command works") {
		t.Errorf("stdout does not report full support:\n%s", got.stdout)
	}

	// Naming a command here would mean naming it as unsupported.
	if strings.Contains(got.stdout, "revert-submission") {
		t.Errorf("stdout lists a command a 3.14 host can run:\n%s", got.stdout)
	}
}

// The command someone runs because something is already wrong has to answer
// the question rather than add an error to it.
func TestDoctorSucceedsWhenTheVersionIsUnknown(t *testing.T) {
	t.Parallel()

	got := runCLI(t, serveVersion(t, ""), "doctor")
	if got.err != nil {
		t.Fatalf("doctor returned an error for an undetermined version: %v", got.err)
	}

	if !strings.Contains(got.stdout, "unknown") {
		t.Errorf("stdout does not say the version is unknown:\n%s", got.stdout)
	}

	if !strings.Contains(got.stdout, "Every command is offered") {
		t.Errorf("stdout does not say what happens next:\n%s", got.stdout)
	}
}

// Credentials the host refuses are the one failure worth returning: it is not
// about versions, and it is the same answer every other command would give.
func TestDoctorReportsRejectedCredentials(t *testing.T) {
	t.Parallel()

	got := runCLI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}, "doctor")

	if got.err == nil {
		t.Fatal("doctor succeeded against a host that rejected the token")
	}

	if code := ExitCode(got.err); code != ExitNotPermitted {
		t.Errorf("exit code = %d, want %d", code, ExitNotPermitted)
	}
}

// Exactly one request, and it is the version endpoint. Probing each operation
// to see which ones answer would mean calling twelve that modify Gerrit, so
// this is the guard against doctor being "improved" into a sweep later.
func TestDoctorMakesOneRequest(t *testing.T) {
	t.Parallel()

	var (
		requests atomic.Int64
		paths    []string
	)

	got := runCLI(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		paths = append(paths, r.URL.EscapedPath())

		serveVersion(t, "3.14.1")(w, r)
	}, "doctor")
	if got.err != nil {
		t.Fatalf("doctor returned an unexpected error: %v", got.err)
	}

	if n := requests.Load(); n != 1 {
		t.Fatalf("requests = %d (%v), want exactly 1", n, paths)
	}

	if want := "/a/config/server/version"; paths[0] != want {
		t.Errorf("path = %q, want %q", paths[0], want)
	}
}

// Arguments are parsed before the configuration is read, so a bad command line
// is exit 2 rather than exit 3 on a host that was never going to be asked.
func TestDoctorRejectsUnexpectedArgumentsWithoutAConfiguration(t *testing.T) {
	t.Parallel()

	got := runCLIWith(t, refuse(t), unconfigured(), "doctor", "extra")

	if code := ExitCode(got.err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d (%v)", code, ExitUsage, got.err)
	}

	if got.requests != 0 {
		t.Errorf("requests = %d, want none", got.requests)
	}
}

// unconfigured blanks the settings runCLIWith always supplies. config treats a
// whitespace-only value as absent, so this is the environment of a host that
// has never been set up.
func unconfigured() map[string]string {
	return map[string]string{
		config.EnvURL:   "",
		config.EnvUser:  "",
		config.EnvToken: "",
	}
}

// Without a configuration there is no host to ask about, and saying so is more
// use than a report with nothing in it.
func TestDoctorReportsAMissingConfiguration(t *testing.T) {
	t.Parallel()

	got := runCLIWith(t, refuse(t), unconfigured(), "doctor")

	if code := ExitCode(got.err); code != ExitNotConfigured {
		t.Errorf("exit code = %d, want %d (%v)", code, ExitNotConfigured, got.err)
	}

	if got.requests != 0 {
		t.Errorf("requests = %d, want none", got.requests)
	}
}
