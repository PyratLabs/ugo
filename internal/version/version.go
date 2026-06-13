package version

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

var versionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// versionCmdTimeout bounds how long a tool's version command may run. Version
// checks are pre-flight — they run before every verb — so a hung version
// command must not hang uGo. It is a var (not a const) so tests can shorten it.
var versionCmdTimeout = 10 * time.Second

// versionCmdWaitDelay bounds the extra wait, after the timeout fires, for any
// descendant processes that still hold the command's output pipes open. Without
// it a version command that backgrounds a child would block I/O indefinitely
// even after its own process is killed.
var versionCmdWaitDelay = 1 * time.Second

// ExtractVersion finds the first semver-compatible version string in output
func ExtractVersion(output string) string {
	match := versionRe.FindStringSubmatch(output)
	if match == nil {
		return ""
	}
	v := match[1]
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// Check verifies a tool's version against min/max constraints
// versionCmd is the command to run (e.g. "ansible-playbook --version").
// Returns the found version and any error.
func Check(name string, versionCmd string, minVersion string, maxVersion string) (string, error) {
	parts := strings.Fields(versionCmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty version command for %s", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.WaitDelay = versionCmdWaitDelay

	// CombinedOutput captures stdout and stderr so tools that report their
	// version on stderr (e.g. "java -version") are still parsed.
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s: version command timed out after %s", name, versionCmdTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("%s: failed to run version command: %w", name, err)
	}

	found := ExtractVersion(string(out))
	if found == "" {
		return "", fmt.Errorf("%s: could not parse version from output", name)
	}

	if minVersion != "" {
		if !strings.HasPrefix(minVersion, "v") {
			minVersion = "v" + minVersion
		}
		if semver.Compare(found, minVersion) < 0 {
			return found, fmt.Errorf("%s: version %s is below minimum %s", name, found, minVersion)
		}
	}

	if maxVersion != "" {
		if !strings.HasPrefix(maxVersion, "v") {
			maxVersion = "v" + maxVersion
		}
		if semver.Compare(found, maxVersion) > 0 {
			return found, fmt.Errorf("%s: version %s exceeds maximum %s", name, found, maxVersion)
		}
	}

	return found, nil
}
