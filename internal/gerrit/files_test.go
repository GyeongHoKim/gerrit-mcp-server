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

func TestGetFileDiff(t *testing.T) {
	t.Parallel()

	const body = `{
	  "meta_a": {"name": "src/widget.go", "content_type": "text/x-go", "lines": 120},
	  "meta_b": {"name": "src/widget.go", "content_type": "text/x-go", "lines": 159},
	  "change_type": "MODIFIED",
	  "diff_header": ["diff --git a/src/widget.go b/src/widget.go", "index 1234..5678 100644"],
	  "content": [
	    {"ab": ["package widget", ""]},
	    {"a": ["func old() {}"], "b": ["func new() {}", "func extra() {}"]},
	    {"skip": 40},
	    {"b": ["// trailing addition"]}
	  ]
	}`

	var gotPath string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		if _, err := w.Write([]byte(xssiPrefix + "\n" + body)); err != nil {
			t.Errorf("writing test response: %v", err)
		}
	})

	got, err := client.GetFileDiff(t.Context(), "12345", "src/widget.go")
	if err != nil {
		t.Fatalf("GetFileDiff() returned an unexpected error: %v", err)
	}

	// The file path is a path segment, so its slashes have to be escaped or
	// Gerrit sees a different endpoint entirely.
	if want := "/a/changes/12345/revisions/current/files/src%2Fwidget.go/diff"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}

	if got.ChangeType != "MODIFIED" {
		t.Errorf("ChangeType = %q, want MODIFIED", got.ChangeType)
	}

	if len(got.Content) != 4 {
		t.Fatalf("len(Content) = %d, want 4", len(got.Content))
	}

	if got.Content[2].Skip != 40 {
		t.Errorf("Skip = %d, want 40", got.Content[2].Skip)
	}

	if got.MetaB == nil || got.MetaB.Lines != 159 {
		t.Errorf("MetaB = %+v, want the new side with 159 lines", got.MetaB)
	}
}

func TestGetFileDiffRejectsEmptyArguments(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want           error
		changeID, file string
	}{
		"no change":    {changeID: " ", file: "a.go", want: ErrEmptyChangeID},
		"no file path": {changeID: "12345", file: "  ", want: ErrEmptyFilePath},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("GetFileDiff() reached the server, want it to refuse before that")
			})

			if _, err := client.GetFileDiff(t.Context(), test.changeID, test.file); !errors.Is(err, test.want) {
				t.Errorf("GetFileDiff() error = %v, want %v", err, test.want)
			}
		})
	}
}
