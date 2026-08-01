package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

// stdout is the JSON-RPC channel. Anything written there that is not protocol
// traffic corrupts the stream and the client disconnects, so what lands on
// which writer is the one thing this command cannot get wrong.

func TestRunPrintsTheVersionOnStdout(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder

	if err := run(t.Context(), []string{"-version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() returned an unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "gerrit-mcp-server") {
		t.Errorf("stdout = %q, want it to name the binary", stdout.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want the version on stdout alone", stderr.String())
	}
}

func TestRunKeepsDiagnosticsOffStdout(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		wantErr error
		args    []string
	}{
		// -h is not a failure; main returns without a message for it.
		"usage text":  {args: []string{"-h"}, wantErr: flag.ErrHelp},
		"parse error": {args: []string{"-nonexistent"}, wantErr: nil},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder

			err := run(t.Context(), test.args, &stdout, &stderr)
			if err == nil {
				t.Fatalf("run(%v) succeeded, want it to report the flag problem", test.args)
			}

			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Errorf("run(%v) error = %v, want it to match %v", test.args, err, test.wantErr)
			}

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing but JSON-RPC on stdout", stdout.String())
			}

			if stderr.Len() == 0 {
				t.Errorf("stderr is empty, want the flag package's message on it")
			}
		})
	}
}
