package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

// A failed invocation must exit non-zero: scripts and CI rely on $?.
func TestUnknownCommandExitsNonZero(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ugo")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "no-such-verb")
	cmd.Dir = t.TempDir()
	cmd.Env = append(cmd.Environ(), "HOME="+t.TempDir())
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected non-zero exit for unknown command, got err = %v", err)
	}
}
