package cli

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/config"
)

const testConfigPath = "/home/alice/.config/gerrit-cli/config.json"

// written is what an init attempt put on the filesystem.
type written struct {
	files map[string][]byte
	perms map[string]fs.FileMode
	dirs  map[string]fs.FileMode
}

// hostileStdin fails the test if it is ever read.
//
// It stands in for the pipe an agent hands a command it should not have run:
// a real read there does not fail, it blocks, and a blocked init is a session
// that never comes back. Nothing in a test can observe a hang, so this
// observes the read instead.
type hostileStdin struct{ t *testing.T }

func (h hostileStdin) Read([]byte) (int, error) {
	h.t.Error("init read stdin without a terminal or -non-interactive, which would hang a caller")

	return 0, io.EOF
}

// initOptions returns options for an init test, plus the record of what the
// attempt wrote.
func initOptions(t *testing.T, stdin io.Reader, existing fstest.MapFS) (*Options, *written, *strings.Builder) {
	t.Helper()

	record := &written{
		files: make(map[string][]byte),
		perms: make(map[string]fs.FileMode),
		dirs:  make(map[string]fs.FileMode),
	}

	var stdout, stderr strings.Builder

	opts := &Options{
		Lookup: lookupFrom(nil),
		ReadFile: func(path string) ([]byte, error) {
			body, ok := record.files[path]
			if ok {
				return body, nil
			}

			return existing.ReadFile(strings.TrimPrefix(path, "/"))
		},
		WriteFile: func(path string, body []byte, perm fs.FileMode) error {
			record.files[path] = body
			record.perms[path] = perm

			return nil
		},
		MkdirAll: func(path string, perm fs.FileMode) error {
			record.dirs[path] = perm

			return nil
		},
		Rename: func(from, to string) error {
			record.files[to] = record.files[from]
			record.perms[to] = record.perms[from]
			delete(record.files, from)
			delete(record.perms, from)

			return nil
		},
		Stat: func(path string) (fs.FileInfo, error) {
			if _, ok := record.files[path]; ok {
				return nil, nil //nolint:nilnil // only the error is consulted
			}

			return existing.Stat(strings.TrimPrefix(path, "/"))
		},
		Stdin:       stdin,
		Stdout:      &stdout,
		Stderr:      &stderr,
		ConfigPath:  testConfigPath,
		Interactive: true,
	}

	return opts, record, &stdout
}

// answers is a stdin holding the three lines init asks for.
func answers(url, user, token string) io.Reader {
	return strings.NewReader(url + "\n" + user + "\n" + token + "\n")
}

func TestInitWritesTheConfiguration(t *testing.T) {
	t.Parallel()

	opts, record, stdout := initOptions(t,
		answers("https://gerrit.example.com", "alice", "s3cret"), nil)

	if err := runInit(opts, false, false); err != nil {
		t.Fatalf("runInit() returned an unexpected error: %v", err)
	}

	var got config.File
	if err := json.Unmarshal(record.files[testConfigPath], &got); err != nil {
		t.Fatalf("the written file is not valid json: %v", err)
	}

	if got.URL != "https://gerrit.example.com" || got.User != "alice" || got.Token != "s3cret" {
		t.Errorf("wrote %+v, want the three answers", got)
	}

	// The permission argument, not the resulting mode: Windows will not honour
	// it and there is nothing to assert there beyond what was asked for.
	if perm := record.perms[testConfigPath]; perm != configFilePerm {
		t.Errorf("wrote the file with %v, want %v", perm, configFilePerm)
	}

	if perm := record.dirs[filepath.Dir(testConfigPath)]; perm != configDirPerm {
		t.Errorf("created the directory with %v, want %v", perm, configDirPerm)
	}

	// init produces no rendered output, so stdout stays clean for the commands
	// that do.
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want init to say nothing there", stdout.String())
	}
}

// TestInitNeverWritesAllowWrite pins the one key that must be a deliberate
// hand edit. A file saying allow_write arms every future invocation on the
// machine, permanently, which is strictly more than the MCP path grants and
// far more than answering three questions should buy.
func TestInitNeverWritesAllowWrite(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t, answers("https://gerrit.example.com", "alice", "s3cret"), nil)

	if err := runInit(opts, false, false); err != nil {
		t.Fatalf("runInit() returned an unexpected error: %v", err)
	}

	if body := string(record.files[testConfigPath]); strings.Contains(body, "allow_write") {
		t.Errorf("the written file mentions allow_write:\n%s", body)
	}
}

// TestInitNeverReadsStdinWithoutATerminal is the anti-hang guarantee. It is
// the reason init is designed the way it is, so it is the test worth having.
func TestInitNeverReadsStdinWithoutATerminal(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t, hostileStdin{t: t}, nil)
	opts.Interactive = false

	err := runInit(opts, false, false)
	if !errors.Is(err, ErrNotInteractive) {
		t.Errorf("error = %v, want it to match %v", err, ErrNotInteractive)
	}

	// The refusal has to say who should run it instead, or whoever hit it has
	// nowhere to go.
	if !strings.Contains(err.Error(), ProgramName+" init") {
		t.Errorf("error = %v, want it to name the command to run", err)
	}

	if len(record.files) != 0 {
		t.Errorf("init wrote %v, want nothing", record.files)
	}
}

func TestInitReadsAPipeWhenToldTo(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t,
		answers("https://gerrit.example.com", "alice", "s3cret"), nil)
	opts.Interactive = false

	if err := runInit(opts, false, true); err != nil {
		t.Fatalf("runInit() with -non-interactive returned an unexpected error: %v", err)
	}

	if len(record.files) != 1 {
		t.Errorf("init wrote %d files, want the configuration", len(record.files))
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	t.Parallel()

	existing := fstest.MapFS{
		strings.TrimPrefix(testConfigPath, "/"): {Data: []byte(`{"url":"https://old.example.com"}`)},
	}

	opts, record, _ := initOptions(t, answers("https://gerrit.example.com", "alice", "s3cret"), existing)

	err := runInit(opts, false, false)
	if !errors.Is(err, ErrConfigExists) {
		t.Errorf("error = %v, want it to match %v", err, ErrConfigExists)
	}

	if !strings.Contains(err.Error(), "-force") {
		t.Errorf("error = %v, want it to name the way past it", err)
	}

	// An agent running init speculatively must not cost someone their working
	// configuration.
	if len(record.files) != 0 {
		t.Errorf("init wrote %v over an existing file, want nothing", record.files)
	}
}

func TestInitOverwritesWithForce(t *testing.T) {
	t.Parallel()

	existing := fstest.MapFS{
		strings.TrimPrefix(testConfigPath, "/"): {Data: []byte(`{"url":"https://old.example.com"}`)},
	}

	opts, record, _ := initOptions(t, answers("https://gerrit.example.com", "alice", "s3cret"), existing)

	if err := runInit(opts, true, false); err != nil {
		t.Fatalf("runInit(-force) returned an unexpected error: %v", err)
	}

	if !strings.Contains(string(record.files[testConfigPath]), "gerrit.example.com") {
		t.Errorf("wrote %q, want the new host", record.files[testConfigPath])
	}
}

// TestInitWritesThroughATemporaryFile pins the atomic write. os.WriteFile
// honours its permission argument only when it creates the file, so -force
// straight over an existing one would leave whatever mode it already had.
func TestInitWritesThroughATemporaryFile(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t, answers("https://gerrit.example.com", "alice", "s3cret"), nil)

	var renamed bool

	opts.Rename = func(from, to string) error {
		renamed = true

		if from == to {
			t.Errorf("renamed %s onto itself, want a sibling temporary file", from)
		}

		record.files[to] = record.files[from]
		record.perms[to] = record.perms[from]

		return nil
	}

	if err := runInit(opts, false, false); err != nil {
		t.Fatalf("runInit() returned an unexpected error: %v", err)
	}

	if !renamed {
		t.Error("init wrote the configuration in place, want a temporary file and a rename")
	}
}

func TestInitReportsEverythingMissingAtOnce(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t, answers("", "", ""), nil)

	err := runInit(opts, false, false)
	if !errors.Is(err, ErrIncompleteAnswers) {
		t.Fatalf("error = %v, want it to match %v", err, ErrIncompleteAnswers)
	}

	for _, want := range []string{"url", "user", "token"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name the missing %s", err, want)
		}
	}

	if len(record.files) != 0 {
		t.Errorf("init wrote %v, want nothing", record.files)
	}
}

// TestInitRefusesAConfigurationThatWouldNotLoad catches a typo now rather than
// on the first real command, using the same validator every invocation runs.
func TestInitRefusesAConfigurationThatWouldNotLoad(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t, answers("gerrit.example.com", "alice", "s3cret"), nil)

	err := runInit(opts, false, false)
	if !errors.Is(err, config.ErrInvalid) {
		t.Errorf("error = %v, want it to match %v", err, config.ErrInvalid)
	}

	if len(record.files) != 0 {
		t.Errorf("init wrote %v, want nothing it could not load back", record.files)
	}
}

func TestInitDoesNotEchoTheToken(t *testing.T) {
	t.Parallel()

	const token = "hunter2"

	opts, record, stdout := initOptions(t, answers("https://gerrit.example.com", "alice", token), nil)

	stderr := &strings.Builder{}
	opts.Stderr = stderr

	if err := runInit(opts, false, false); err != nil {
		t.Fatalf("runInit() returned an unexpected error: %v", err)
	}

	// It has to reach the file, and it must not reach anything a colleague
	// might be looking at over a shoulder or in a pasted transcript.
	if !strings.Contains(string(record.files[testConfigPath]), token) {
		t.Error("the token did not reach the configuration file")
	}

	for name, output := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
		if strings.Contains(output, token) {
			t.Errorf("%s repeats the token back: %q", name, output)
		}
	}
}

// TestInitFlagsReachTheWork drives init through Run, which is the only thing
// that checks the flags are bound to what actually uses them. Every other init
// test calls runInit directly and would pass with -force wired to nothing.
func TestInitFlagsReachTheWork(t *testing.T) {
	t.Parallel()

	existing := fstest.MapFS{
		strings.TrimPrefix(testConfigPath, "/"): {Data: []byte(`{"url":"https://old.example.com"}`)},
	}

	tests := map[string]struct {
		wantErr     error
		args        []string
		interactive bool
		wantWritten bool
	}{
		"force replaces an existing file": {
			args:        []string{"init", "-force"},
			interactive: true,
			wantWritten: true,
		},
		"without force it is refused": {
			args:        []string{"init"},
			interactive: true,
			wantErr:     ErrConfigExists,
		},
		"non-interactive opens a pipe": {
			args:        []string{"init", "-non-interactive", "-force"},
			interactive: false,
			wantWritten: true,
		},
		"a pipe alone is still refused": {
			args:        []string{"init", "-force"},
			interactive: false,
			wantErr:     ErrNotInteractive,
		},
		"an unknown flag is rejected": {
			args:        []string{"init", "-nonexistent"},
			interactive: true,
			wantErr:     ErrUsage,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, record, _ := initOptions(t,
				answers("https://gerrit.example.com", "alice", "s3cret"), existing)
			opts.Interactive = test.interactive

			err := Run(t.Context(), test.args, opts)

			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Errorf("error = %v, want it to match %v", err, test.wantErr)
			}

			if test.wantErr == nil && err != nil {
				t.Fatalf("Run(%v) returned an unexpected error: %v", test.args, err)
			}

			written := strings.Contains(string(record.files[testConfigPath]), "gerrit.example.com")
			if written != test.wantWritten {
				t.Errorf("wrote the new configuration = %t, want %t", written, test.wantWritten)
			}
		})
	}
}

// TestInitReportsAFailedWrite covers the three ways the filesystem can refuse.
// Each one leaves a different amount behind, and a caller told only "init
// failed" cannot tell which.
func TestInitReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	broken := errors.New("read-only filesystem")

	tests := map[string]struct {
		breaks func(*Options)
		want   string
	}{
		"the directory cannot be created": {
			breaks: func(o *Options) {
				o.MkdirAll = func(string, fs.FileMode) error { return broken }
			},
			want: "creating",
		},
		"the temporary file cannot be written": {
			breaks: func(o *Options) {
				o.WriteFile = func(string, []byte, fs.FileMode) error { return broken }
			},
			want: "writing",
		},
		"the rename into place fails": {
			breaks: func(o *Options) {
				o.Rename = func(string, string) error { return broken }
			},
			want: "renaming",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, _, _ := initOptions(t, answers("https://gerrit.example.com", "alice", "s3cret"), nil)
			test.breaks(opts)

			err := runInit(opts, false, false)
			if err == nil {
				t.Fatal("runInit() succeeded on a filesystem that refused, want the failure reported")
			}

			if !errors.Is(err, broken) {
				t.Errorf("error = %v, want it to wrap the filesystem's own failure", err)
			}

			// Naming the stage is what tells the reader whether anything was
			// left behind.
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %v, want it to say which stage failed (%q)", err, test.want)
			}
		})
	}
}

// TestInitReportsAnUnreadableStdin covers a stdin that fails rather than ends.
// EOF is an answer -- an empty one -- but a broken pipe is not, and treating
// the two alike would report "you gave me nothing" for a terminal that died.
func TestInitReportsAnUnreadableStdin(t *testing.T) {
	t.Parallel()

	broken := errors.New("input/output error")

	opts, record, _ := initOptions(t, brokenReader{err: broken}, nil)

	err := runInit(opts, false, false)
	if !errors.Is(err, broken) {
		t.Errorf("error = %v, want it to wrap the reader's own failure", err)
	}

	if len(record.files) != 0 {
		t.Errorf("init wrote %v, want nothing", record.files)
	}
}

// TestInitReportsAnUnwritablePrompt covers a terminal that has gone away
// between the prompt and the answer.
func TestInitReportsAnUnwritablePrompt(t *testing.T) {
	t.Parallel()

	opts, _, _ := initOptions(t, answers("https://gerrit.example.com", "alice", "s3cret"), nil)
	opts.Stderr = errWriter{}

	err := runInit(opts, false, false)
	if err == nil {
		t.Fatal("runInit() succeeded prompting a closed terminal, want the failure reported")
	}

	if !strings.Contains(err.Error(), "prompt") {
		t.Errorf("error = %v, want it to name the failed prompt", err)
	}
}

// TestInitReportsAnUnwritableConfirmation covers the last write init makes.
// Reaching it means the file is already on disk, so failing silently here
// would tell a caller nothing happened when in fact everything did.
func TestInitReportsAnUnwritableConfirmation(t *testing.T) {
	t.Parallel()

	opts, record, _ := initOptions(t,
		answers("https://gerrit.example.com", "alice", "s3cret"), nil)

	// Not interactive, so nothing is prompted and the confirmation is the only
	// thing this writer ever sees.
	opts.Interactive = false
	opts.Stderr = errWriter{}

	err := runInit(opts, false, true)
	if err == nil {
		t.Fatal("runInit() succeeded reporting to a closed stderr, want the failure reported")
	}

	if len(record.files) == 0 {
		t.Error("the configuration was not written, so this is not the failure under test")
	}
}

// brokenReader fails rather than ending, standing in for a terminal that died
// mid-answer.
type brokenReader struct{ err error }

func (b brokenReader) Read([]byte) (int, error) {
	return 0, b.err
}

func TestInitNeedsSomewhereToPutTheFile(t *testing.T) {
	t.Parallel()

	opts, _, _ := initOptions(t, hostileStdin{t: t}, nil)
	opts.ConfigPath = ""

	err := runInit(opts, false, false)
	if !errors.Is(err, ErrNoConfigPath) {
		t.Errorf("error = %v, want it to match %v", err, ErrNoConfigPath)
	}

	if !strings.Contains(err.Error(), config.EnvConfigPath) {
		t.Errorf("error = %v, want it to name the variable that fixes it", err)
	}
}
