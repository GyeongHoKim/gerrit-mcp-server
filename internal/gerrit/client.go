// Package gerrit is a thin client for the Gerrit REST API.
//
// All HTTP in this server happens here. The package deliberately covers only
// the endpoints the MCP tools need rather than the whole API surface.
package gerrit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/version"
)

// maxResponseBytes caps a single response body. A diff on a generated file can
// be enormous, and an unbounded read is a memory bug waiting for the right
// change to come along.
const maxResponseBytes = 32 << 20 // 32 MiB

// maxErrorBodyBytes caps how much of a failure response we keep. It only has
// to be enough to explain the refusal.
const maxErrorBodyBytes = 4 << 10 // 4 KiB

// xssiPrefix guards Gerrit's JSON responses against cross-site script
// inclusion. It must be removed before the body can be parsed.
const xssiPrefix = ")]}'"

// Sentinel errors for the Gerrit statuses callers act on.
var (
	// ErrNotFound reports a 404: the change, file or comment does not exist.
	ErrNotFound = errors.New("not found")
	// ErrUnauthorized reports a 401: the credentials were rejected.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden reports a 403: the account may not do this.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict reports a 409: the change is not in a state that allows it.
	ErrConflict = errors.New("conflict")
	// ErrResponseTooLarge reports a body past the size cap.
	ErrResponseTooLarge = errors.New("response too large")
)

// statusFor reports the status a sentinel stands for.
//
// A switch rather than a map: errors.Is calls Is(target) whatever the target
// is, and hashing an uncomparable error value panics.
func statusFor(target error) (int, bool) {
	switch {
	case errors.Is(target, ErrNotFound):
		return http.StatusNotFound, true
	case errors.Is(target, ErrUnauthorized):
		return http.StatusUnauthorized, true
	case errors.Is(target, ErrForbidden):
		return http.StatusForbidden, true
	case errors.Is(target, ErrConflict):
		return http.StatusConflict, true
	default:
		return 0, false
	}
}

// userAgent identifies this client to Gerrit, which makes a misbehaving
// version easy to find in a server log.
var userAgent = "gerrit-mcp-server/" + version.Version

// APIError is a non-2xx response from Gerrit.
type APIError struct {
	// Method is the HTTP method of the failed request.
	Method string
	// Path is the API path of the failed request, without the host.
	Path string
	// Body is Gerrit's response body, which usually explains the refusal.
	Body string
	// StatusCode is the HTTP status Gerrit returned.
	StatusCode int
}

// Error implements the error interface.
func (e *APIError) Error() string {
	message := fmt.Sprintf("gerrit: %s %s: %d %s",
		e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))

	if body := strings.TrimSpace(e.Body); body != "" {
		message += ": " + body
	}

	return message
}

// Is maps the HTTP status onto the package's sentinel errors so that callers
// can write errors.Is(err, gerrit.ErrNotFound) instead of comparing numbers.
func (e *APIError) Is(target error) bool {
	status, ok := statusFor(target)

	return ok && e.StatusCode == status
}

// Client talks to one Gerrit host as one account.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	user       string
	token      string
}

// Options describes the host a [Client] talks to and the account it talks as.
//
// A struct rather than four parameters: User and Token are both strings, and
// nothing in the type system would catch them being handed over the wrong way
// round.
type Options struct {
	// BaseURL is the Gerrit host, without a trailing slash. It must be
	// absolute and carry no query or fragment -- endpoint concatenates onto it.
	BaseURL *url.URL
	// User is the Gerrit account name used for HTTP Basic auth.
	User string
	// Token is the auth token from that account's HTTP Credentials page.
	Token string
	// Timeout bounds a single request. Zero leaves the request unbounded,
	// which only makes sense in a test.
	Timeout time.Duration
}

// New returns a client for the host in opts.
//
// The caller is responsible for the validation: this package takes the host
// and credentials as given rather than knowing where they came from.
func New(opts Options) *Client {
	return &Client{
		baseURL:    opts.BaseURL,
		httpClient: &http.Client{Timeout: opts.Timeout},
		user:       opts.User,
		token:      opts.Token,
	}
}

// changePath returns the API path for a change, escaping the id.
//
// Escaping is not optional: the triplet form of a change id is
// project~branch~Ihash and Gerrit projects routinely contain a slash.
func changePath(id, suffix string) string {
	// Gerrit's own ChangeInfo.id arrives percent-encoded ("<project>~<number>"
	// with project encoded), while a user is just as likely to type the raw
	// triplet. Decoding first makes the escaping idempotent instead of turning
	// an id that was already encoded into %252F and 404ing a real change.
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}

	return "/changes/" + url.PathEscape(id) + suffix
}

// do performs an authenticated request and decodes the response into out.
//
// It is the only place that knows about the /a/ prefix, HTTP Basic auth and
// Gerrit's XSSI guard, so no endpoint method has to remember them. Pass a nil
// in to send no body, and a nil out to discard the response.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	body, err := encodeBody(in)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, query), body)
	if err != nil {
		return fmt.Errorf("building %s %s: %w", method, path, err)
	}

	req.SetBasicAuth(c.user, c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	// The body is fully read below; a failure to close it is not actionable.
	defer resp.Body.Close() //nolint:errcheck // see above

	// The status is settled before the body is read. A proxy answering 401
	// with a huge HTML login page must be reported as rejected credentials,
	// not as an oversized response.
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{
			Method:     method,
			Path:       path,
			Body:       readErrorBody(resp.Body),
			StatusCode: resp.StatusCode,
		}
	}

	payload, err := readBody(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s %s: %w", method, path, err)
	}

	if out == nil {
		return nil
	}

	if err = json.Unmarshal(stripXSSI(payload), out); err != nil {
		return fmt.Errorf("decoding %s %s: %w", method, path, err)
	}

	return nil
}

// endpoint renders the absolute URL for path.
//
// The URL is assembled as a string rather than through url.URL fields because
// path arrives already escaped; assigning it to URL.Path would escape the
// percent signs a second time and turn %2F into %252F.
func (c *Client) endpoint(path string, query url.Values) string {
	endpoint := c.baseURL.String() + "/a" + path

	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	return endpoint
}

// encodeBody marshals in, or yields an empty body when there is nothing to
// send. It never returns a nil reader, so callers have no nil case to handle.
func encodeBody(in any) (io.Reader, error) {
	if in == nil {
		return http.NoBody, nil
	}

	encoded, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encoding request body: %w", err)
	}

	return bytes.NewReader(encoded), nil
}

// readBody reads at most maxResponseBytes, refusing anything larger rather
// than letting one enormous diff exhaust memory.
func readBody(r io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if len(payload) > maxResponseBytes {
		return nil, fmt.Errorf("%w: over %d bytes", ErrResponseTooLarge, maxResponseBytes)
	}

	return payload, nil
}

// readErrorBody reads as much of a failure response as is worth keeping.
//
// Failures never need the full 32 MiB budget: the body only has to explain the
// refusal, and an unreadable one is not itself worth reporting.
func readErrorBody(r io.Reader) string {
	payload, err := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes+1))
	if err != nil {
		return ""
	}

	return truncate(string(payload), maxErrorBodyBytes)
}

// stripXSSI removes Gerrit's cross-site script inclusion guard, which may or
// may not be followed by a line break.
func stripXSSI(payload []byte) []byte {
	trimmed := bytes.TrimPrefix(payload, []byte(xssiPrefix))

	return bytes.TrimLeft(trimmed, "\r\n")
}

// truncate shortens s to at most limit bytes, marking that it did so.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	return s[:limit] + "..."
}
