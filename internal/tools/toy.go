package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Repo is a tiny in-memory code repository the demo tools operate on. It ships
// with a deliberate bug (a subtraction where the test expects addition) so an
// agent has something real to find and fix. It is safe for concurrent use so a
// later milestone can run many agents against copies of it.
type Repo struct {
	mu    sync.Mutex
	files map[string]string
}

// NewBuggyRepo returns a repo whose test fails until the bug in math.py is
// fixed. add() subtracts; the test expects add(2, 3) == 5.
func NewBuggyRepo() *Repo {
	return &Repo{files: map[string]string{
		"math.py":      "def add(a, b):\n    return a - b\n",
		"test_math.py": "from math import add\n\n\ndef test_add():\n    assert add(2, 3) == 5\n",
	}}
}

func (r *Repo) get(path string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.files[path]
	return c, ok
}

func (r *Repo) set(path, content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[path] = content
}

func (r *Repo) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.files))
	for name := range r.files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// TestsPass reports whether the repo's test would pass. It is a stand-in for a
// real test runner: the fix is present once add() adds instead of subtracts.
func (r *Repo) TestsPass() bool {
	c, ok := r.get("math.py")
	return ok && strings.Contains(c, "a + b")
}

// --- tools over the repo ---

// ListFiles lists the files in the repo.
type ListFiles struct{ repo *Repo }

func NewListFiles(repo *Repo) *ListFiles { return &ListFiles{repo} }
func (t *ListFiles) Name() string        { return "list_files" }
func (t *ListFiles) Description() string {
	return "List the files in the repository. Input is ignored."
}
func (t *ListFiles) Run(_ context.Context, _ string) (string, error) {
	return strings.Join(t.repo.names(), "\n"), nil
}

// ReadFile returns the contents of one file.
type ReadFile struct{ repo *Repo }

func NewReadFile(repo *Repo) *ReadFile  { return &ReadFile{repo} }
func (t *ReadFile) Name() string        { return "read_file" }
func (t *ReadFile) Description() string { return "Read a file. Input is the file path." }
func (t *ReadFile) Run(_ context.Context, input string) (string, error) {
	path := strings.TrimSpace(input)
	c, ok := t.repo.get(path)
	if !ok {
		return "", fmt.Errorf("no such file: %s", path)
	}
	return c, nil
}

// WriteFile replaces the contents of a file. Input is the path on the first
// line, then the full new contents on the lines after it.
type WriteFile struct{ repo *Repo }

func NewWriteFile(repo *Repo) *WriteFile { return &WriteFile{repo} }
func (t *WriteFile) Name() string        { return "write_file" }
func (t *WriteFile) Description() string {
	return "Write a file. First line of input is the path; the rest is the new file contents."
}
func (t *WriteFile) Run(_ context.Context, input string) (string, error) {
	path, content, found := strings.Cut(input, "\n")
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("write_file needs a path on the first line")
	}
	if !found {
		content = ""
	}
	t.repo.set(path, content)
	return fmt.Sprintf("wrote %s (%d bytes)", path, len(content)), nil
}

// RunTests runs the repo's test suite.
type RunTests struct{ repo *Repo }

func NewRunTests(repo *Repo) *RunTests  { return &RunTests{repo} }
func (t *RunTests) Name() string        { return "run_tests" }
func (t *RunTests) Description() string { return "Run the test suite. Input is ignored." }
func (t *RunTests) Run(_ context.Context, _ string) (string, error) {
	if t.repo.TestsPass() {
		return "PASS: 1 passed (test_add)", nil
	}
	return "FAIL: test_add: expected add(2, 3) == 5", nil
}
