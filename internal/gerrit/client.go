// Package gerrit is a thin client for the Gerrit REST API.
//
// All HTTP in this server happens here. The package deliberately covers only
// the endpoints the MCP tools need rather than the whole API surface.
package gerrit

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

// maxResponseBytes caps a single response body. A diff on a generated file can
// be enormous, and an unbounded read is a memory bug waiting for the right
// change to come along.
const maxResponseBytes = 32 << 20 // 32 MiB

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
)

// errNotImplemented is returned by the stub until [Client.do] is written.
var errNotImplemented = errors.New("not implemented")

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
func (*APIError) Error() string {
	return ""
}

// Is maps the HTTP status onto the package's sentinel errors so that callers
// can write errors.Is(err, gerrit.ErrNotFound) instead of comparing numbers.
func (*APIError) Is(target error) bool {
	_ = target

	return false
}

// Client talks to one Gerrit host as one account.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	user       string
	token      string
}

// New returns a client for the host in cfg.
//
// cfg is expected to have come from [config.Load], which has already rejected
// a missing host or credentials.
func New(cfg config.Config) *Client {
	return &Client{
		baseURL:    cfg.BaseURL,
		user:       cfg.User,
		token:      cfg.Token,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// changePath returns the API path for a change, escaping the id.
//
// Escaping is not optional: the triplet form of a change id is
// project~branch~Ihash and Gerrit projects routinely contain a slash.
func changePath(id, suffix string) string {
	return "/changes/" + url.PathEscape(id) + suffix
}

// do performs an authenticated request and decodes the response into out.
//
// It is the only place that knows about the /a/ prefix, HTTP Basic auth and
// Gerrit's XSSI guard, so no endpoint method has to remember them. Pass a nil
// in to send no body, and a nil out to discard the response.
func (*Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	_, _, _, _, _, _ = ctx, method, path, query, in, out

	return errNotImplemented
}
