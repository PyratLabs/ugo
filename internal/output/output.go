package output

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mgutz/ansi"
)

var noColor bool

// SetNoColor enables or disables color output
func SetNoColor(v bool) {
	noColor = v
	ansi.DisableColors(v)
}

// Sanitize replaces terminal-unsafe characters in s with U+FFFD: C0 controls
// (except tab), DEL, C1 controls, Unicode bidi controls, and invalid UTF-8
// (a raw 0x9b byte is an 8-bit CSI to some terminals), all of which could
// otherwise rewrite the terminal when printed (e.g. from a filename matching
// a glob). Tab and newline are preserved for multiline output.
func Sanitize(s string) string { return sanitize(s, true) }

// SanitizeInline is Sanitize for single-line contexts: it also replaces
// newlines so one value cannot forge additional output lines.
func SanitizeInline(s string) string { return sanitize(s, false) }

func sanitize(s string, keepNewline bool) string {
	// range decodes each invalid byte as U+FFFD, so the loop below replaces
	// them; the fast path must check validity explicitly.
	if utf8.ValidString(s) && !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || (keepNewline && r == '\n') {
			b.WriteRune(r)
		} else if unsafeRune(r) {
			b.WriteRune('�')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unsafeRune(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) ||
		r == 0x061c || r == 0x200e || r == 0x200f ||
		(r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

const (
	checkPass = "✅"
	checkFail = "❌"
	info      = "💡"
	rocket    = "🚀"
)

// CheckPass prints a passing check with green checkmark
func CheckPass(msg string) {
	fmt.Fprintf(os.Stdout, "    %s %s\n", green(checkPass), Sanitize(msg))
}

// CheckFail prints a failing check with red cross
func CheckFail(msg string) {
	fmt.Fprintf(os.Stderr, "    %s %s\n", red(checkFail), Sanitize(msg))
}

// Info prints an info message with blue icon
func Info(msg string) {
	fmt.Fprintf(os.Stdout, "    %s %s\n", blue(info), Sanitize(msg))
}

// CommandRunning prints a command about to execute with rocket
func CommandRunning(verb string, cmd string) {
	fmt.Fprintf(os.Stdout, "%s %s: %s\n\n", yellow(rocket), yellow(SanitizeInline(verb)), Sanitize(cmd))
}

// CommandSuccess prints a success after command execution
func CommandSuccess(verb string) {
	fmt.Fprintf(os.Stdout, "\n  %s %s completed successfully\n", green(checkPass), green(SanitizeInline(verb)))
}

// CommandFail prints a failure after command execution
func CommandFail(verb string) {
	fmt.Fprintf(os.Stderr, "\n  %s %s failed\n", red(checkFail), red(SanitizeInline(verb)))
}

func green(s string) string {
	if noColor {
		return s
	}
	return ansi.Color(s, "green")
}

func red(s string) string {
	if noColor {
		return s
	}
	return ansi.Color(s, "red")
}

func yellow(s string) string {
	if noColor {
		return s
	}
	return ansi.Color(s, "yellow+b")
}

func blue(s string) string {
	if noColor {
		return s
	}
	return ansi.Color(s, "blue")
}

func bold(s string) string {
	if noColor {
		return s
	}
	return ansi.Color(s, "default+b")
}

// Bold returns a bold string
func Bold(s string) string {
	return bold(s)
}
