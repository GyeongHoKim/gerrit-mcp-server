package gerrit

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	testUser  = "alice"
	testToken = "s3cret"
)

// newTestClient starts a stub Gerrit on a local port and returns a client
// pointed at it.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parsing test server url: %v", err)
	}

	return New(Options{
		BaseURL: base,
		User:    testUser,
		Token:   testToken,
		Timeout: 5 * time.Second,
	})
}

// writeOK replies with an empty object the way Gerrit does, XSSI guard and all.
func writeOK(t *testing.T, w http.ResponseWriter) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")

	if _, err := io.WriteString(w, xssiPrefix+"\n{}"); err != nil {
		t.Errorf("writing test response: %v", err)
	}
}

func TestDoRequestShape(t *testing.T) {
	t.Parallel()

	var got *http.Request

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		writeOK(t, w)
	})

	err := client.do(t.Context(), http.MethodGet, "/changes/", url.Values{"q": {"status:open"}}, nil, &struct{}{})
	if err != nil {
		t.Fatalf("do() returned an unexpected error: %v", err)
	}

	t.Run("authenticated requests are prefixed with /a/", func(t *testing.T) {
		t.Parallel()

		if want := "/a/changes/"; got.URL.Path != want {
			t.Errorf("path = %q, want %q", got.URL.Path, want)
		}
	})

	t.Run("query parameters are forwarded", func(t *testing.T) {
		t.Parallel()

		if want := "status:open"; got.URL.Query().Get("q") != want {
			t.Errorf("q = %q, want %q", got.URL.Query().Get("q"), want)
		}
	})

	t.Run("basic auth carries the configured credentials", func(t *testing.T) {
		t.Parallel()

		user, token, ok := got.BasicAuth()
		if !ok {
			t.Fatalf("Authorization header = %q, want basic auth", got.Header.Get("Authorization"))
		}

		if user != testUser || token != testToken {
			t.Errorf("basic auth = %q:%q, want %q:%q", user, token, testUser, testToken)
		}
	})

	t.Run("the token never appears outside the authorization header", func(t *testing.T) {
		t.Parallel()

		encoded := base64.StdEncoding.EncodeToString([]byte(testUser + ":" + testToken))
		for name, values := range got.Header {
			if name == "Authorization" {
				continue
			}

			for _, value := range values {
				if strings.Contains(value, testToken) || strings.Contains(value, encoded) {
					t.Errorf("header %s leaks the token: %q", name, value)
				}
			}
		}

		if strings.Contains(got.URL.String(), testToken) {
			t.Errorf("url leaks the token: %q", got.URL)
		}
	})

	t.Run("json is requested", func(t *testing.T) {
		t.Parallel()

		if want := "application/json"; got.Header.Get("Accept") != want {
			t.Errorf("Accept = %q, want %q", got.Header.Get("Accept"), want)
		}
	})

	t.Run("the client identifies itself", func(t *testing.T) {
		t.Parallel()

		if agent := got.Header.Get("User-Agent"); !strings.Contains(agent, "gerrit-mcp-server") {
			t.Errorf("User-Agent = %q, want it to mention gerrit-mcp-server", agent)
		}
	})
}

func TestDoStripsXSSIPrefix(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"with the guard and a newline":  xssiPrefix + "\n" + `{"subject":"fix the thing"}`,
		"with the guard and no newline": xssiPrefix + `{"subject":"fix the thing"}`,
		"without the guard":             `{"subject":"fix the thing"}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if _, err := io.WriteString(w, body); err != nil {
					t.Errorf("writing test response: %v", err)
				}
			})

			var got struct {
				Subject string `json:"subject"`
			}

			if err := client.do(t.Context(), http.MethodGet, "/changes/1", nil, nil, &got); err != nil {
				t.Fatalf("do() returned an unexpected error: %v", err)
			}

			if want := "fix the thing"; got.Subject != want {
				t.Errorf("subject = %q, want %q", got.Subject, want)
			}
		})
	}
}

func TestDoSendsRequestBody(t *testing.T) {
	t.Parallel()

	var (
		gotBody        string
		gotContentType string
	)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}

		gotBody = string(body)
		gotContentType = r.Header.Get("Content-Type")

		writeOK(t, w)
	})

	in := struct {
		Message string `json:"message"`
	}{Message: "please take another look"}

	if err := client.do(t.Context(), http.MethodPost, "/changes/1/abandon", nil, in, nil); err != nil {
		t.Fatalf("do() returned an unexpected error: %v", err)
	}

	if want := `{"message":"please take another look"}`; strings.TrimSpace(gotBody) != want {
		t.Errorf("request body = %q, want %q", gotBody, want)
	}

	if want := "application/json"; gotContentType != want {
		t.Errorf("Content-Type = %q, want %q", gotContentType, want)
	}
}

func TestDoStatusErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want   error
		status int
	}{
		"401 is unauthorized": {status: http.StatusUnauthorized, want: ErrUnauthorized},
		"403 is forbidden":    {status: http.StatusForbidden, want: ErrForbidden},
		"404 is not found":    {status: http.StatusNotFound, want: ErrNotFound},
		"409 is conflict":     {status: http.StatusConflict, want: ErrConflict},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			})

			err := client.do(t.Context(), http.MethodGet, "/changes/1", nil, nil, nil)
			if !errors.Is(err, test.want) {
				t.Errorf("do() error = %v, want it to match %v", err, test.want)
			}
		})
	}
}

func TestDoUnmappedStatusCarriesDetail(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)

		if _, err := io.WriteString(w, "the index is rebuilding"); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	err := client.do(t.Context(), http.MethodGet, "/changes/1", nil, nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("do() error = %v, want an *APIError", err)
	}

	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusInternalServerError)
	}

	if !strings.Contains(apiErr.Body, "the index is rebuilding") {
		t.Errorf("Body = %q, want it to carry Gerrit's explanation", apiErr.Body)
	}

	// A 500 is not one of the sentinels, so it must not masquerade as one.
	for _, sentinel := range []error{ErrNotFound, ErrUnauthorized, ErrForbidden, ErrConflict} {
		if errors.Is(err, sentinel) {
			t.Errorf("500 wrongly matches %v", sentinel)
		}
	}

	if !strings.Contains(apiErr.Error(), "500") {
		t.Errorf("Error() = %q, want it to mention the status", apiErr.Error())
	}
}

func TestDoRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := io.WriteString(w, xssiPrefix+"\n"); err != nil {
			return
		}

		// Stream past the cap without allocating it all here.
		chunk := strings.Repeat("a", 1<<20)
		for range (maxResponseBytes >> 20) + 1 {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
	})

	err := client.do(t.Context(), http.MethodGet, "/changes/1", nil, nil, &struct{}{})

	// Asserting only err != nil would pass with no cap at all: an oversized
	// body fails json.Unmarshal anyway, which is a different bug entirely.
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("do() error = %v, want ErrResponseTooLarge", err)
	}
}

// uncomparableError is an error value that cannot be used as a map key.
type uncomparableError struct{ notes []string }

func (uncomparableError) Error() string { return "uncomparable" }

func TestAPIErrorIsToleratesUncomparableTarget(t *testing.T) {
	t.Parallel()

	// errors.Is calls Is(target) whatever the target is; it only guards the
	// == fast path. A target that cannot be hashed must not bring the process
	// down with it.
	if errors.Is(&APIError{StatusCode: http.StatusNotFound}, uncomparableError{notes: []string{"x"}}) {
		t.Error("an unrelated error matched ErrNotFound")
	}
}

func TestDoReportsStatusRatherThanSizeForHugeErrorBodies(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)

		// An SSO proxy in front of Gerrit answers 401 with an HTML login page,
		// which can be arbitrarily large.
		chunk := strings.Repeat("<html>padding</html>", 1<<16)
		for range (maxResponseBytes >> 20) + 1 {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
	})

	err := client.do(t.Context(), http.MethodGet, "/changes/1", nil, nil, nil)

	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("do() error = %v, want ErrUnauthorized -- the status must win over the body size", err)
	}

	if errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("do() error = %v, want the rejected credentials reported, not the body size", err)
	}
}

func TestDoHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		writeOK(t, w)
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := client.do(ctx, http.MethodGet, "/changes/1", nil, nil, nil)
	close(release)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("do() error = %v, want context.Canceled", err)
	}
}

func TestDoPreservesBaseURLSubPath(t *testing.T) {
	t.Parallel()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeOK(t, w)
	}))
	t.Cleanup(server.Close)

	base, err := url.Parse(server.URL + "/gerrit")
	if err != nil {
		t.Fatalf("parsing test server url: %v", err)
	}

	client := New(Options{
		BaseURL: base,
		User:    testUser,
		Token:   testToken,
		Timeout: 5 * time.Second,
	})

	if doErr := client.do(t.Context(), http.MethodGet, "/changes/", nil, nil, &struct{}{}); doErr != nil {
		t.Fatalf("do() returned an unexpected error: %v", doErr)
	}

	if want := "/gerrit/a/changes/"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestChangePathEscapesID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		id     string
		suffix string
		want   string
	}{
		"numeric id": {
			id:   "12345",
			want: "/changes/12345",
		},
		"triplet with a slash in the project": {
			id:   "platform/frameworks/base~main~I8473b959",
			want: "/changes/platform%2Fframeworks%2Fbase~main~I8473b959",
		},
		"an id gerrit returned is already escaped": {
			// ChangeInfo.id is "<project>~<_number>" with project URL encoded,
			// so escaping it again would send %252F and 404 a real change.
			id:   "platform%2Fbase~12345",
			want: "/changes/platform%2Fbase~12345",
		},
		"suffix is appended after the escaped id": {
			id:     "a/b~main~I1",
			suffix: "/detail",
			want:   "/changes/a%2Fb~main~I1/detail",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(test.want, changePath(test.id, test.suffix)); diff != "" {
				t.Errorf("changePath() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
