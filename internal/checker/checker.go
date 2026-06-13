package checker

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/PyratLabs/ugo/internal/config"
	"github.com/PyratLabs/ugo/internal/version"
)

// Issue represents a problem found during tool checking
type Issue struct {
	Tool   string
	Errors []string
}

// CheckTools validates all required tools are available with correct versions
func CheckTools(tools map[string]config.Tool) []Issue {
	var issues []Issue

	// Iterate in sorted order so results (and the printed check output) are
	// deterministic rather than following Go's randomized map iteration.
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		tool := tools[name]
		var errs []string

		if _, err := exec.LookPath(name); err != nil {
			msg := fmt.Sprintf("%s is not installed", name)
			if tool.DownloadURL != "" {
				msg += fmt.Sprintf(", download at: %s", tool.DownloadURL)
			}
			errs = append(errs, msg)
			issues = append(issues, Issue{Tool: name, Errors: errs})
			continue
		}

		if tool.MinVersion != "" || tool.MaxVersion != "" {
			cmd := tool.VersionCmd
			if cmd == "" {
				cmd = fmt.Sprintf("%s --version", name)
			}

			foundVer, err := version.Check(name, cmd, tool.MinVersion, tool.MaxVersion)
			if err != nil {
				errs = append(errs, err.Error())
			} else if tool.MinVersion != "" || tool.MaxVersion != "" {
				errs = append(errs, fmt.Sprintf("version: %s", foundVer))
			}
		}

		if len(errs) > 0 {
			issues = append(issues, Issue{Tool: name, Errors: errs})
		}
	}

	return issues
}

// FormatErrors renders issues as user-friendly error messages
func FormatErrors(issues []Issue) string {
	var b strings.Builder
	for _, issue := range issues {
		for _, err := range issue.Errors {
			if strings.HasPrefix(err, "version:") {
				continue // version info, not an error
			}
			b.WriteString(fmt.Sprintf("  - %s: %s\n", issue.Tool, err))
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// HasErrors returns true if any issues contain actual errors (not just version info)
func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		for _, err := range issue.Errors {
			if !strings.HasPrefix(err, "version:") {
				return true
			}
		}
	}
	return false
}
