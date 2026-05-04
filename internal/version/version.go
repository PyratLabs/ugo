package version

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

var versionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

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

	out, err := exec.Command(parts[0], parts[1:]...).Output()
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
