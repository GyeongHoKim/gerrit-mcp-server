package gerrit

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

// The table below is an inventory of the shapes GET /config/server/version has
// been seen to answer with, not a set of invented cases. A host this cannot
// read is offered every tool, so a miss here is a missing diagnosis rather
// than a broken client -- but it is still the one place a surprise lands. Add
// to it whenever a new one turns up.
func TestParseServerVersion(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw     string
		want    ServerVersion
		wantErr bool
	}{
		"a release":                 {raw: "3.14.1", want: ServerVersion{Major: 3, Minor: 14}},
		"the oldest supported":      {raw: "2.14.22", want: ServerVersion{Major: 2, Minor: 14}},
		"major and minor only":      {raw: "2.16", want: ServerVersion{Major: 2, Minor: 16}},
		"a build off a tag":         {raw: "3.9.1-1234-gabcdef", want: ServerVersion{Major: 3, Minor: 9}},
		"a vendor patch level":      {raw: "3.10.0.1", want: ServerVersion{Major: 3, Minor: 10}},
		"an enterprise suffix":      {raw: "3.8.2-acme4", want: ServerVersion{Major: 3, Minor: 8}},
		"a release candidate":       {raw: "3.11.0-rc2", want: ServerVersion{Major: 3, Minor: 11}},
		"a snapshot":                {raw: "3.12.0-SNAPSHOT", want: ServerVersion{Major: 3, Minor: 12}},
		"a leading v":               {raw: "v3.1", want: ServerVersion{Major: 3, Minor: 1}},
		"surrounding whitespace":    {raw: "  3.14.1\n", want: ServerVersion{Major: 3, Minor: 14}},
		"a minor with a suffix":     {raw: "2.16-rc0", want: ServerVersion{Major: 2, Minor: 16}},
		"built without a version":   {raw: "(unknown version)", wantErr: true},
		"empty":                     {raw: "", wantErr: true},
		"words":                     {raw: "not.a.version", wantErr: true},
		"a major with no minor":     {raw: "3", wantErr: true},
		"a negative":                {raw: "-3.1", wantErr: true},
		"a minor that is not a run": {raw: "3.-1", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseServerVersion(test.raw)

			if test.wantErr {
				if !errors.Is(err, ErrUnknownServerVersion) {
					t.Fatalf("ParseServerVersion(%q) error = %v, want ErrUnknownServerVersion", test.raw, err)
				}

				// A failure must not look like an ancient Gerrit, or every
				// unreadable host would have its tools hidden.
				if !got.IsZero() {
					t.Errorf("ParseServerVersion(%q) = %v on failure, want the zero value", test.raw, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseServerVersion(%q) returned an unexpected error: %v", test.raw, err)
			}

			if got != test.want {
				t.Errorf("ParseServerVersion(%q) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestServerVersionBefore(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		v    ServerVersion
		min  ServerVersion
		want bool
	}{
		"an older minor":                         {v: ServerVersion{2, 14}, min: ServerVersion{2, 15}, want: true},
		"the same release":                       {v: ServerVersion{2, 15}, min: ServerVersion{2, 15}, want: false},
		"a newer minor":                          {v: ServerVersion{2, 16}, min: ServerVersion{2, 15}, want: false},
		"an older major":                         {v: ServerVersion{2, 16}, min: ServerVersion{3, 2}, want: true},
		"a newer major":                          {v: ServerVersion{3, 0}, min: ServerVersion{2, 15}, want: false},
		"a higher minor is not a higher release": {v: ServerVersion{2, 30}, min: ServerVersion{3, 2}, want: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := test.v.Before(test.min); got != test.want {
				t.Errorf("%v.Before(%v) = %v, want %v", test.v, test.min, got, test.want)
			}
		})
	}
}

func TestServerVersionString(t *testing.T) {
	t.Parallel()

	if got, want := (ServerVersion{Major: 2, Minor: 14}).String(), "2.14"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestGetServerVersion(t *testing.T) {
	t.Parallel()

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		// A bare JSON string, which is what this endpoint answers with.
		if _, err := w.Write([]byte(xssiPrefix + "\n\"2.14.22\"")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.GetServerVersion(t.Context())
	if err != nil {
		t.Fatalf("GetServerVersion() returned an unexpected error: %v", err)
	}

	if want := "/a/config/server/version"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if want := (ServerVersion{Major: 2, Minor: 14}); got != want {
		t.Errorf("GetServerVersion() = %v, want %v", got, want)
	}
}

// A client asks its host once. Every gated endpoint consults the version on
// the way to reporting a failure, so an unmemoised probe would turn one bad
// call into two requests, and a firewalled version endpoint into a second
// timeout on every one of them.
func TestGetServerVersionIsAskedOnce(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)

		if _, err := w.Write([]byte(xssiPrefix + "\n\"3.14.1\"")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	for range 3 {
		if _, err := client.GetServerVersion(t.Context()); err != nil {
			t.Fatalf("GetServerVersion() returned an unexpected error: %v", err)
		}
	}

	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestGetServerVersionRemembersAFailure(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})

	for range 3 {
		if _, err := client.GetServerVersion(t.Context()); err == nil {
			t.Fatal("GetServerVersion() succeeded against a host that answered 404")
		}
	}

	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1: a failure is remembered too", got)
	}
}

// A cancelled call is the caller's failure, not the host's, so the next caller
// with a context of its own still gets an answer. Remembering this one would
// leave a client that dropped a single request deciding nothing from the
// version for the rest of the session.
func TestGetServerVersionRetriesAfterACancelledCall(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(xssiPrefix + "\n\"3.14.1\"")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := client.GetServerVersion(cancelled); err == nil {
		t.Fatal("GetServerVersion() succeeded with a cancelled context")
	}

	got, err := client.GetServerVersion(t.Context())
	if err != nil {
		t.Fatalf("GetServerVersion() returned an unexpected error: %v", err)
	}

	if want := (ServerVersion{Major: 3, Minor: 14}); got != want {
		t.Errorf("GetServerVersion() = %v, want %v", got, want)
	}
}

func TestGetServerVersionRejectsAnUnreadableAnswer(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(xssiPrefix + "\n\"(unknown version)\"")); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.GetServerVersion(t.Context())
	if !errors.Is(err, ErrUnknownServerVersion) {
		t.Fatalf("GetServerVersion() error = %v, want ErrUnknownServerVersion", err)
	}

	if !got.IsZero() {
		t.Errorf("GetServerVersion() = %v, want the zero value", got)
	}
}
