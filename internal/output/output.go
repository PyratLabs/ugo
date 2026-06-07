package output

import (
	"fmt"
	"os"

	"github.com/mgutz/ansi"
)

var noColor bool

// SetNoColor enables or disables color output
func SetNoColor(v bool) {
	noColor = v
	ansi.DisableColors(v)
}

const (
	checkPass = "✅"
	checkFail = "❌"
	info      = "💡"
	rocket    = "🚀"
)

// CheckPass prints a passing check with green checkmark
func CheckPass(msg string) {
	fmt.Fprintf(os.Stdout, "    %s %s\n", green(checkPass), msg)
}

// CheckFail prints a failing check with red cross
func CheckFail(msg string) {
	fmt.Fprintf(os.Stderr, "    %s %s\n", red(checkFail), msg)
}

// Info prints an info message with blue icon
func Info(msg string) {
	fmt.Fprintf(os.Stdout, "    %s %s\n", blue(info), msg)
}

// CommandRunning prints a command about to execute with rocket
func CommandRunning(verb string, cmd string) {
	fmt.Fprintf(os.Stdout, "%s %s: %s\n\n", yellow(rocket), yellow(verb), cmd)
}

// CommandSuccess prints a success after command execution
func CommandSuccess(verb string) {
	fmt.Fprintf(os.Stdout, "\n  %s %s completed successfully\n", green(checkPass), green(verb))
}

// CommandFail prints a failure after command execution
func CommandFail(verb string) {
	fmt.Fprintf(os.Stderr, "\n  %s %s failed\n", red(checkFail), red(verb))
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

const mask = "********"

// MaskVar returns a masked placeholder if the variable name is sensitive,
// otherwise returns the original value.
func MaskVar(name string, value string, sensitiveNames map[string]bool) string {
	if sensitiveNames[name] {
		return mask
	}
	return value
}

// MaskedVars returns a copy of vars with sensitive values replaced by the mask.
func MaskedVars(vars map[string]string, sensitiveNames map[string]bool) map[string]string {
	masked := make(map[string]string, len(vars))
	for k, v := range vars {
		masked[k] = MaskVar(k, v, sensitiveNames)
	}
	return masked
}
