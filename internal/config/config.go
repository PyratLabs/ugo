package config

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Tool defines a required tool dependency
type Tool struct {
	MinVersion  string `yaml:"min_version"`
	MaxVersion  string `yaml:"max_version"`
	VersionCmd  string `yaml:"version_cmd"`
	DownloadURL string `yaml:"download_url"`
}

// Argument defines a single command argument with optional validation
type Argument struct {
	Name    string   `yaml:"name"`
	Values  []string `yaml:"values"`
	Match   string   `yaml:"match"`
	Exclude []string `yaml:"exclude"`
}

// Group defines a named section for organising verbs in help output.
// Declaration order in YAML controls display order.
type Group struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Prompt defines an interactive prompt that collects user input at runtime.
type Prompt struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Sensitive   bool   `yaml:"sensitive"`
	FromEnvVar  string `yaml:"from_env_var"`
}

// Platform defines an OS/arch-specific command variant. OS and Arch match
// runtime.GOOS and runtime.GOARCH; an empty field matches anything.
type Platform struct {
	OS   string   `yaml:"os"`
	Arch string   `yaml:"arch"`
	Cmd  string   `yaml:"cmd"`
	Cmds []string `yaml:"cmds"`
}

// Command defines a single verb's configuration. A command's name is its key
// in the commands map.
type Command struct {
	Cmd         string            `yaml:"cmd"`
	Cmds        []string          `yaml:"cmds"`
	Env         map[string]string `yaml:"env"`
	Description string            `yaml:"description"`
	Group       string            `yaml:"group"`
	Arguments   []Argument        `yaml:"arguments"`
	Prompts     []Prompt          `yaml:"prompts"`
	Platforms   []Platform        `yaml:"platforms"`
}

// Valid GOOS/GOARCH values (from "go tool dist list"), used only to warn on
// typos — an unknown value silently matches nothing, hiding the verb with no
// explanation otherwise.
var knownOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true,
	"freebsd": true, "illumos": true, "ios": true, "js": true, "linux": true,
	"netbsd": true, "openbsd": true, "plan9": true, "solaris": true,
	"wasip1": true, "windows": true,
}

var knownArch = map[string]bool{
	"386": true, "amd64": true, "arm": true, "arm64": true, "loong64": true,
	"mips": true, "mipsle": true, "mips64": true, "mips64le": true,
	"ppc64": true, "ppc64le": true, "riscv64": true, "s390x": true,
	"wasm": true,
}

// resolvePlatforms rewrites each command to its first matching platform
// variant, keeping the top-level cmd/cmds as fallback. Commands with no
// matching variant and no fallback are removed. os/arch values match
// runtime.GOOS/GOARCH case-insensitively; unrecognised values are reported
// on warn.
func resolvePlatforms(commands map[string]Command, goos, goarch string, warn io.Writer) {
	for name, cmd := range commands {
		matched := false
		for _, p := range cmd.Platforms {
			if p.OS != "" && !knownOS[strings.ToLower(p.OS)] {
				fmt.Fprintf(warn, "warning: command %q: unknown os %q in platforms\n", name, p.OS)
			}
			if p.Arch != "" && !knownArch[strings.ToLower(p.Arch)] {
				fmt.Fprintf(warn, "warning: command %q: unknown arch %q in platforms\n", name, p.Arch)
			}
			if !matched && (p.OS == "" || strings.EqualFold(p.OS, goos)) && (p.Arch == "" || strings.EqualFold(p.Arch, goarch)) {
				cmd.Cmd, cmd.Cmds = p.Cmd, p.Cmds
				matched = true
			}
		}
		if !matched && len(cmd.Platforms) > 0 && cmd.Cmd == "" && len(cmd.Cmds) == 0 {
			delete(commands, name)
			continue
		}
		commands[name] = cmd
	}
}

// Config represents the full YAML configuration
type Config struct {
	Commands     map[string]Command `yaml:"commands"`
	Tools        map[string]Tool    `yaml:"tools"`
	Groups       []Group            `yaml:"groups"`
	ShellOptions string             `yaml:"shell_options"` // prepended to all shell scripts (e.g., "set -euo pipefail")
}

// Load merges global and local configs. Local overrides global. It also
// returns the local config's raw bytes (nil when absent) so callers can
// trust-check exactly the content that was parsed, not whatever is on disk
// by the time the check runs. binaryName is used to locate both configs.
func Load(binaryName string) (*Config, []byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("getting home directory: %w", err)
	}

	global, _, err := loadConfigFile(filepath.Join(home, ".config", binaryName, "config.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("loading global config: %w", err)
	}

	local, localRaw, err := loadConfigFile(filepath.Join(".", binaryName+".yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("loading local config: %w", err)
	}

	merged := mergeConfigs(global, local)
	resolvePlatforms(merged.Commands, runtime.GOOS, runtime.GOARCH, os.Stderr)
	return merged, localRaw, nil
}

// loadConfigFile parses the config at path. raw is the exact bytes that were
// parsed, and is nil if and only if the file does not exist — the trust gate
// relies on that distinction, so an existing-but-empty file stays non-nil.
func loadConfigFile(path string) (cfg *Config, raw []byte, err error) {
	cfg = &Config{
		Commands: make(map[string]Command),
		Tools:    make(map[string]Tool),
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, nil, err
	}

	if data == nil {
		data = []byte{}
	}
	return cfg, data, nil
}

func mergeConfigs(global, local *Config) *Config {
	merged := &Config{
		Commands: make(map[string]Command),
		Tools:    make(map[string]Tool),
	}

	// Local entries override global ones with the same key.
	maps.Copy(merged.Commands, global.Commands)
	maps.Copy(merged.Commands, local.Commands)
	maps.Copy(merged.Tools, global.Tools)
	maps.Copy(merged.Tools, local.Tools)

	merged.Groups = mergeGroups(global.Groups, local.Groups)

	// Local shell_options overrides global; otherwise inherit global.
	merged.ShellOptions = global.ShellOptions
	if local.ShellOptions != "" {
		merged.ShellOptions = local.ShellOptions
	}

	return merged
}

// mergeGroups deduplicates groups by name: global groups keep their declared
// order, local-only groups are appended after in their declared order, and a
// local group with the same name overrides the global one in place.
func mergeGroups(global, local []Group) []Group {
	localByName := make(map[string]Group, len(local))
	for _, g := range local {
		localByName[g.Name] = g
	}

	var merged []Group
	seen := make(map[string]bool, len(global)+len(local))
	for _, g := range global {
		if seen[g.Name] {
			continue
		}
		seen[g.Name] = true
		if lg, ok := localByName[g.Name]; ok {
			g = lg
		}
		merged = append(merged, g)
	}
	for _, g := range local {
		if seen[g.Name] {
			continue
		}
		seen[g.Name] = true
		merged = append(merged, g)
	}
	return merged
}

// ConfigPaths returns the human-readable paths checked during load
func ConfigPaths(binaryName string) (global, local string) {
	home, _ := os.UserHomeDir()
	global = filepath.Join(home, ".config", binaryName, "config.yaml")
	local = filepath.Join(".", binaryName+".yaml")
	return
}

// BinaryName extracts the base name from os.Args[0], stripping path and extension
func BinaryName() string {
	base := filepath.Base(os.Args[0])
	return strings.TrimSuffix(base, filepath.Ext(base))
}
