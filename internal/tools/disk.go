package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DiskRepo is a working directory of real files. Its tools read and write real
// files, and run_tests actually runs Python, so the agent touches real code and
// its edits survive the process. That durability is what makes a resumed run
// correct across workers, where the in-memory toy repo cannot be.
type DiskRepo struct {
	dir string
}

// NewDiskRepo wraps an existing directory as a repo.
func NewDiskRepo(dir string) *DiskRepo { return &DiskRepo{dir: dir} }

// Dir returns the working directory, so a caller can show or clean it up.
func (r *DiskRepo) Dir() string { return r.dir }

// SeedBuggyPython writes the fix-a-failing-test scenario to disk: a calc module
// whose add() subtracts, and a test that expects it to add.
func (r *DiskRepo) SeedBuggyPython() error {
	files := map[string]string{
		"calc.py": "def add(a, b):\n    return a - b\n",
		"test_calc.py": "from calc import add\n\n" +
			"assert add(2, 3) == 5, f\"add(2, 3) should be 5, got {add(2, 3)}\"\n" +
			"print(\"PASS: add works\")\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (r *DiskRepo) path(name string) string {
	// Keep tools inside the repo dir: strip any leading path and refuse escapes.
	return filepath.Join(r.dir, filepath.Base(filepath.Clean(name)))
}

// --- disk-backed tools; same names as the in-memory ones so prompts match ---

type diskList struct{ repo *DiskRepo }

func NewDiskListFiles(r *DiskRepo) Tool { return &diskList{r} }
func (t *diskList) Name() string        { return "list_files" }
func (t *diskList) Description() string { return "List the files in the repository. Input is ignored." }
func (t *diskList) Run(_ context.Context, _ string) (string, error) {
	entries, err := os.ReadDir(t.repo.dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return strings.Join(names, "\n"), nil
}

type diskRead struct{ repo *DiskRepo }

func NewDiskReadFile(r *DiskRepo) Tool  { return &diskRead{r} }
func (t *diskRead) Name() string        { return "read_file" }
func (t *diskRead) Description() string { return "Read a file. Input is the file path." }
func (t *diskRead) Run(_ context.Context, input string) (string, error) {
	b, err := os.ReadFile(t.repo.path(strings.TrimSpace(input)))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type diskWrite struct{ repo *DiskRepo }

func NewDiskWriteFile(r *DiskRepo) Tool { return &diskWrite{r} }
func (t *diskWrite) Name() string       { return "write_file" }
func (t *diskWrite) Description() string {
	return "Write a file. First line of input is the path; the rest is the new file contents."
}
func (t *diskWrite) Run(_ context.Context, input string) (string, error) {
	path, content, found := strings.Cut(input, "\n")
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errBadWrite
	}
	if !found {
		content = ""
	}
	if err := os.WriteFile(t.repo.path(path), []byte(content), 0o644); err != nil {
		return "", err
	}
	return "wrote " + path, nil
}

type diskTests struct{ repo *DiskRepo }

func NewDiskRunTests(r *DiskRepo) Tool   { return &diskTests{r} }
func (t *diskTests) Name() string        { return "run_tests" }
func (t *diskTests) Description() string { return "Run the test suite. Input is ignored." }
func (t *diskTests) Run(ctx context.Context, _ string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// -B, not -I: the test imports the local calc module, so the run directory
	// must stay on Python's path (which -I drops). -B stops Python writing a
	// .pyc, which otherwise gets reused staler than the fix when the edit keeps
	// the file the same size within the same second.
	cmd := exec.CommandContext(cctx, "python3", "-B", "test_calc.py")
	cmd.Dir = t.repo.dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err == nil {
		if text == "" {
			text = "PASS"
		}
		return text, nil
	}
	// A failing test is a normal observation the agent recovers from: return the
	// last, most useful line rather than a raw error.
	line := lastLine(text)
	if line == "" {
		line = err.Error()
	}
	return "FAIL: " + line, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

type badWrite struct{}

func (badWrite) Error() string { return "write_file needs a path on the first line" }

var errBadWrite = badWrite{}

// NewDiskRegistry builds the standard fix-a-test toolset over a real directory.
func NewDiskRegistry(r *DiskRepo) *Registry {
	return NewRegistry(NewDiskListFiles(r), NewDiskReadFile(r), NewDiskWriteFile(r), NewDiskRunTests(r))
}
