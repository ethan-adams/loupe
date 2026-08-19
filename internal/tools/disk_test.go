package tools

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestDiskRepoRealFixCycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	ctx := context.Background()
	r := NewDiskRepo(t.TempDir())
	if err := r.SeedBuggyPython(); err != nil {
		t.Fatal(err)
	}

	rt := NewDiskRunTests(r)
	if out, _ := rt.Run(ctx, ""); !strings.HasPrefix(out, "FAIL") {
		t.Fatalf("seeded repo should fail its test, got %q", out)
	}

	if _, err := NewDiskWriteFile(r).Run(ctx, "calc.py\ndef add(a, b):\n    return a + b\n"); err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if out, _ := rt.Run(ctx, ""); !strings.HasPrefix(out, "PASS") {
		t.Fatalf("fixed repo should pass, got %q", out)
	}

	ls, _ := NewDiskListFiles(r).Run(ctx, "")
	if !strings.Contains(ls, "calc.py") {
		t.Errorf("list_files missing calc.py: %q", ls)
	}
	if _, err := NewDiskReadFile(r).Run(ctx, "missing.py"); err == nil {
		t.Error("reading a missing file should error")
	}
}
