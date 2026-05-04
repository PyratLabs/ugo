package output

import (
	"bytes"
	"os"
	"testing"
)

func TestSetNoColor(t *testing.T) {
	SetNoColor(true)
	if !noColor {
		t.Error("expected noColor to be true")
	}

	SetNoColor(false)
	if noColor {
		t.Error("expected noColor to be false")
	}
}

func TestOutputWithColor(t *testing.T) {
	SetNoColor(false)

	tests := []struct {
		name string
		fn   func()
	}{
		{"CheckPass", func() { CheckPass("test passed") }},
		{"CheckFail", func() { CheckFail("test failed") }},
		{"Info", func() { Info("some info") }},
		{"CommandRunning", func() { CommandRunning("plan", "echo plan") }},
		{"CommandSuccess", func() { CommandSuccess("plan") }},
		{"CommandFail", func() { CommandFail("plan") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify these don't panic
			tt.fn()
		})
	}
}

func TestOutputNoColor(t *testing.T) {
	SetNoColor(true)
	defer SetNoColor(false)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	CheckPass("test passed")
	CheckFail("test failed")

	wOut.Close()
	wErr.Close()

	var bufOut, bufErr bytes.Buffer
	bufOut.ReadFrom(rOut)
	bufErr.ReadFrom(rErr)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	out := bufOut.String()
	err := bufErr.String()

	if out == "" {
		t.Error("expected output on stdout")
	}
	if err == "" {
		t.Error("expected output on stderr")
	}

	// Verify no ANSI escape codes when noColor is true
	if containsANSI(out) {
		t.Errorf("stdout contains ANSI codes: %q", out)
	}
	if containsANSI(err) {
		t.Errorf("stderr contains ANSI codes: %q", err)
	}
}

func containsANSI(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			return true
		}
	}
	return false
}
