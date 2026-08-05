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
