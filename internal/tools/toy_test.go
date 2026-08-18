package tools

import (
	"context"
	"strings"
	"testing"
)

func TestRunTestsReflectsTheFix(t *testing.T) {
	repo := NewBuggyRepo()
	rt := NewRunTests(repo)

	out, err := rt.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("run_tests errored: %v", err)
	}
	if !strings.HasPrefix(out, "FAIL") {
		t.Fatalf("buggy repo should fail, got %q", out)
	}

	if _, err := NewWriteFile(repo).Run(context.Background(), "math.py\ndef add(a, b):\n    return a + b\n"); err != nil {
		t.Fatalf("write_file errored: %v", err)
	}

	out, _ = rt.Run(context.Background(), "")
	if !strings.HasPrefix(out, "PASS") {
		t.Fatalf("fixed repo should pass, got %q", out)
	}
}

func TestReadFileMissingIsAnError(t *testing.T) {
	repo := NewBuggyRepo()
	_, err := NewReadFile(repo).Run(context.Background(), "mat.py")
	if err == nil {
		t.Fatal("reading a missing file should error so the agent can recover")
	}
}

func TestWriteFileParsesPathAndContent(t *testing.T) {
	repo := NewBuggyRepo()
	if _, err := NewWriteFile(repo).Run(context.Background(), "notes.txt\nhello\nworld"); err != nil {
		t.Fatalf("write_file errored: %v", err)
	}
	got, err := NewReadFile(repo).Run(context.Background(), "notes.txt")
	if err != nil {
		t.Fatalf("read_file errored: %v", err)
	}
	if got != "hello\nworld" {
		t.Fatalf("content = %q, want %q", got, "hello\nworld")
	}
}

func TestListFilesIsSortedAndStable(t *testing.T) {
	repo := NewBuggyRepo()
	out, _ := NewListFiles(repo).Run(context.Background(), "")
	if out != "math.py\ntest_math.py" {
		t.Fatalf("list_files = %q", out)
	}
}
