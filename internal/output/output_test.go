package output

import (
	"bytes"
	"os"
	"strings"
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

func TestSanitize(t *testing.T) {
	SetNoColor(true)
	defer SetNoColor(false)

	t.Run("control characters replaced", func(t *testing.T) {
		in := "a\x1b]52;c;stolen\x07b\rc\x07d\x7f"
		got := Sanitize(in)
		for i := 0; i < len(got); i++ {
			if r := rune(got[i]); r < 0x20 && r != '\t' && r != '\n' || r == 0x7f || r == '\x1b' {
				t.Errorf("Sanitize left unsafe byte %#U in %q", r, got)
			}
		}
		if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
			t.Errorf("Sanitize dropped safe content: %q", got)
		}
	})

	t.Run("bidi overrides replaced", func(t *testing.T) {
		got := Sanitize("safe\u202Eevil\u2066x")
		if strings.ContainsRune(got, '\u202E') || strings.ContainsRune(got, '\u2066') {
			t.Errorf("Sanitize left bidi control: %q", got)
		}
	})

	t.Run("newline and tab preserved", func(t *testing.T) {
		if got := Sanitize("a\nb\tc"); got != "a\nb\tc" {
			t.Errorf("Sanitize(%q) = %q, want unchanged", "a\nb\tc", got)
		}
	})

	t.Run("SanitizeInline strips newlines", func(t *testing.T) {
		got := SanitizeInline("a\r\nb\nc")
		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("SanitizeInline(%q) = %q, want no line breaks", "a\r\nb\nc", got)
		}
		if !strings.Contains(got, "a") || !strings.Contains(got, "b") || !strings.Contains(got, "c") {
			t.Errorf("SanitizeInline dropped content: %q", got)
		}
	})

	t.Run("clean text unchanged", func(t *testing.T) {
		const s = "terraform plan -out=tfplan"
		if Sanitize(s) != s || SanitizeInline(s) != s {
			t.Errorf("clean text was modified: %q / %q", Sanitize(s), SanitizeInline(s))
		}
	})

	t.Run("CheckFail scrubs messages", func(t *testing.T) {
		oldStderr := os.Stderr
		rErr, wErr, _ := os.Pipe()
		os.Stderr = wErr
		CheckFail("bad \x1b]0;pwned\x07 value")
		wErr.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		buf.ReadFrom(rErr)
		if strings.Contains(buf.String(), "\x1b") {
			t.Errorf("CheckFail output contains escape: %q", buf.String())
		}
	})
}
