package gerrit

import (
	"errors"
	"net/http"
	"testing"
)

func TestListFiles(t *testing.T) {
	t.Parallel()

	const body = `{
	  "/COMMIT_MSG": {"status": "A", "lines_inserted": 9, "size_delta": 320, "size": 320},
	  "src/widget.go": {"lines_inserted": 42, "lines_deleted": 3, "size_delta": 900, "size": 4200},
	  "src/legacy.go": {"status": "D", "lines_deleted": 18, "size_delta": -600, "size": 0},
	  "src/new.go": {"status": "R", "old_path": "src/old.go", "lines_inserted": 2, "lines_deleted": 2, "size_delta": 0, "size": 100},
	  "assets/logo.png": {"status": "A", "binary": true, "size_delta": 2048, "size": 2048}
	}`

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.ListFiles(t.Context(), "a/b~main~I1")
	if err != nil {
		t.Fatalf("ListFiles() returned an unexpected error: %v", err)
	}

	if want := "/a/changes/a%2Fb~main~I1/revisions/current/files/"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if len(got) != 5 {
		t.Fatalf("len(files) = %d, want 5", len(got))
	}

	// A plain modification carries no status at all, which is easy to mistake
	// for a missing field.
	if status := got["src/widget.go"].Status; status != "" {
		t.Errorf("status of a modified file = %q, want it to be absent", status)
	}

	if got["src/new.go"].OldPath != "src/old.go" {
		t.Errorf("OldPath = %q, want the pre-rename path", got["src/new.go"].OldPath)
	}

	if !got["assets/logo.png"].Binary {
		t.Error("Binary = false for a png, want true")
	}

	if got["src/legacy.go"].SizeDelta != -600 {
		t.Errorf("SizeDelta = %d, want -600 to survive as negative", got["src/legacy.go"].SizeDelta)
	}
}

func TestListFilesRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("ListFiles() reached the server, want it to refuse before that")
	})

	if _, err := client.ListFiles(t.Context(), ""); !errors.Is(err, ErrEmptyChangeID) {
		t.Errorf("ListFiles() error = %v, want ErrEmptyChangeID", err)
	}
}

func TestListFilesMapsNotFound(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := client.ListFiles(t.Context(), "99999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ListFiles() error = %v, want ErrNotFound", err)
	}
}
