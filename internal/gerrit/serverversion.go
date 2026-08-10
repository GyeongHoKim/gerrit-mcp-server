package gerrit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrUnknownServerVersion reports a version string this cannot read.
//
// It is not a failure to report to a caller so much as a reason to stop
// deciding anything from the version: everything that consults one treats an
// unknown as "offer it and let Gerrit answer".
var ErrUnknownServerVersion = errors.New("unrecognised gerrit version")

// ErrUnsupportedByServer reports an endpoint this Gerrit is too old to have.
//
// Declared here rather than with the transport sentinels in client.go: it is a
// conclusion drawn from a version, not a status Gerrit answered with. What the
// server actually sent was a 404.
var ErrUnsupportedByServer = errors.New("not supported by this gerrit")

// probeTimeout bounds the version probe.
//
// Deliberately not the configured request timeout. Someone raises that to
// minutes for a generated file's diff, and waiting minutes to decide whether
// to offer a tool would be absurd. It is not configurable either: every
// setting is a permanent compatibility surface, and this one has no answer
// worth arguing about.
const probeTimeout = 5 * time.Second

// ServerVersion is a Gerrit release.
//
// Only the major and minor are kept. Gerrit opens a feature in a release and
// never in a patch, so a patch level would be a field nothing could ever
// read.
type ServerVersion struct {
	// Major is the leading number, 2 in 2.14.
	Major int
	// Minor is the second number, 14 in 2.14.
	Minor int
}

// IsZero reports a version nothing determined.
//
// Callers treat it as "offer everything": hiding a working operation is worse
// than offering one that fails with an explanation, and an enterprise fork
// that backported an endpoint is common enough to design for.
func (v ServerVersion) IsZero() bool {
	return v.Major == 0 && v.Minor == 0
}

// Before reports whether this release predates other.
func (v ServerVersion) Before(other ServerVersion) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}

	return v.Minor < other.Minor
}

// String renders the release as Gerrit names it, for example "2.14".
func (v ServerVersion) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

// ParseServerVersion reads the release out of what Gerrit reports.
//
// Only the leading major.minor is taken; a patch level, a build suffix such as
// "-1234-gabcdef", or a vendor's own "-acme4" are all discarded. A string that
// does not begin with a number is an error rather than a zero value: a Gerrit
// built without version information answers "(unknown version)", and reading
// that as 0.0 would make every host look ancient and hide tools that work.
func ParseServerVersion(raw string) (ServerVersion, error) {
	text := strings.TrimPrefix(strings.TrimSpace(raw), "v")

	majorText, rest, found := strings.Cut(text, ".")
	if !found {
		return ServerVersion{}, fmt.Errorf("%w: %q", ErrUnknownServerVersion, raw)
	}

	major, majorOK := parseDigits(majorText)
	minor, minorOK := parseDigits(leadingDigits(rest))

	if !majorOK || !minorOK {
		return ServerVersion{}, fmt.Errorf("%w: %q", ErrUnknownServerVersion, raw)
	}

	return ServerVersion{Major: major, Minor: minor}, nil
}

// parseDigits reads a run of ASCII digits, refusing anything else -- including
// the leading sign that strconv.Atoi would otherwise accept.
func parseDigits(s string) (int, bool) {
	if s == "" || leadingDigits(s) != s {
		return 0, false
	}

	parsed, err := strconv.Atoi(s)

	return parsed, err == nil
}

// leadingDigits returns the run of ASCII digits at the start of s.
func leadingDigits(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}

	return s
}

// GetServerVersion reports which Gerrit release the host is running.
//
// It is asked once per client and remembered, failures included: a host that
// keeps this endpoint behind a proxy should be asked once rather than once per
// call that wonders. The mutex is held across the request on purpose, so that
// concurrent tool handlers share one probe instead of racing to make their
// own; sync.Once would be wrong here, because it would capture whichever
// caller arrived first along with its context.
func (c *Client) GetServerVersion(ctx context.Context) (ServerVersion, error) {
	c.versionMu.Lock()
	defer c.versionMu.Unlock()

	if !c.versionDone {
		c.versionDone = true
		c.serverVersion, c.versionErr = c.fetchServerVersion(ctx)
	}

	return c.serverVersion, c.versionErr
}

// unsupportedIfOlder rewrites a failure that turned out to mean the host is
// too old for the endpoint, and leaves every other failure alone.
//
// The status cannot say so by itself: POST /changes/999/wip answers 404 both
// for a change that does not exist and for a Gerrit that does not have the
// endpoint. Guessing from the 404 would send someone who mistyped a change
// number off to read release notes, and would report "ask a human" for
// something "fix the number" describes exactly. So the version is asked for,
// and only a host that really is older gets the clearer error.
//
// Everything else falls through unchanged, which is what makes this fail open
// rather than by policy: a version that could not be determined, or a fork
// that backported the endpoint and answers 200, is never refused. The cost is
// one request, and only on a call that has already failed.
func (c *Client) unsupportedIfOlder(ctx context.Context, err error, needs ServerVersion) error {
	if err == nil || !errors.Is(err, ErrNotFound) {
		return err
	}

	running, versionErr := c.GetServerVersion(ctx)
	if versionErr != nil || !running.Before(needs) {
		return err
	}

	// The 404 is deliberately not wrapped. Nothing is missing; the endpoint
	// was never there, and leaving ErrNotFound in the chain would have a
	// caller report it as a change that does not exist.
	return fmt.Errorf("%w: needs gerrit %s or newer, and this host reports %s",
		ErrUnsupportedByServer, needs, running)
}

// fetchServerVersion performs the probe.
//
// The endpoint answers a bare JSON string rather than an object, which is why
// this decodes into one.
func (c *Client) fetchServerVersion(ctx context.Context) (ServerVersion, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var raw string
	if err := c.do(ctx, http.MethodGet, "/config/server/version", nil, nil, &raw); err != nil {
		return ServerVersion{}, fmt.Errorf("asking gerrit its version: %w", err)
	}

	return ParseServerVersion(raw)
}
