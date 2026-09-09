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

// CheckInstalled reports tools that are not on PATH. It never executes
// config-defined version commands — verbs use it as a cheap pre-flight,
// while the check command runs the full CheckTools.
func CheckInstalled(tools map[string]config.Tool) []Issue {
	var issues []Issue
	for _, name := range sortedNames(tools) {
		if _, err := exec.LookPath(name); err != nil {
			issues = append(issues, Issue{Tool: name, Errors: []string{notInstalledMsg(name, tools[name])}})
		}
	}
	return issues
}

// CheckTools validates all required tools are available with correct versions
func CheckTools(tools map[string]config.Tool) []Issue {
	var issues []Issue

	for _, name := range sortedNames(tools) {
		tool := tools[name]
		var errs []string

		if _, err := exec.LookPath(name); err != nil {
			errs = append(errs, notInstalledMsg(name, tool))
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
			} else {
				errs = append(errs, fmt.Sprintf("version: %s", foundVer))
			}
		}

		if len(errs) > 0 {
			issues = append(issues, Issue{Tool: name, Errors: errs})
		}
	}

	return issues
}

// sortedNames returns tool names in sorted order so results (and the printed
// check output) are deterministic rather than following Go's randomized map
// iteration.
func sortedNames(tools map[string]config.Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func notInstalledMsg(name string, tool config.Tool) string {
	msg := fmt.Sprintf("%s is not installed", name)
	if tool.DownloadURL != "" {
		msg += fmt.Sprintf(", download at: %s", tool.DownloadURL)
	}
	return msg
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
