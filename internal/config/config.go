package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// Tool defines a required tool dependency
type Tool struct {
	MinVersion  string `mapstructure:"min_version"`
	MaxVersion  string `mapstructure:"max_version"`
	VersionCmd  string `mapstructure:"version_cmd"`
	DownloadURL string `mapstructure:"download_url"`
}

// Argument defines a single command argument with optional validation
type Argument struct {
	Name    string   `mapstructure:"name"`
	Values  []string `mapstructure:"values"`
	Match   string   `mapstructure:"match"`
	Exclude []string `mapstructure:"exclude"`
}

// Prompt defines an interactive prompt that collects user input at runtime.
type Prompt struct {
	Name        string `mapstructure:"name"`
	Description string `mapstructure:"description"`
	Sensitive   bool   `mapstructure:"sensitive"`
	FromEnvVar  string `mapstructure:"from_env_var"`
}

// Command defines a single verb's configuration
type Command struct {
	Name        string            `mapstructure:"name"`
	Cmd         string            `mapstructure:"cmd"`
	Cmds        []string          `mapstructure:"cmds"`
	Env         map[string]string `mapstructure:"env"`
	Description string            `mapstructure:"description"`
	Arguments   []Argument        `mapstructure:"arguments"`
	Prompts     []Prompt          `mapstructure:"prompts"`
}

// Config represents the full YAML configuration
type Config struct {
	Commands      map[string]Command `mapstructure:"commands"`
	Tools         map[string]Tool    `mapstructure:"tools"`
	ShellOptions  string             `mapstructure:"shell_options"`   // prepended to all shell scripts (e.g., "set -euo pipefail")
}

// Load merges global and local configs. Local overrides global.
// binaryName is used to locate both config locations.
func Load(binaryName string) (*Config, error) {
	global, err := loadGlobalConfig(binaryName)
	if err != nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}

	local, err := loadLocalConfig(binaryName)
	if err != nil {
		return nil, fmt.Errorf("loading local config: %w", err)
	}

	merged := mergeConfigs(global, local)
	return merged, nil
}

func loadGlobalConfig(binaryName string) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	configPath := filepath.Join(home, ".config", binaryName, "config.yaml")
	return loadConfigFile(configPath)
}

func loadLocalConfig(binaryName string) (*Config, error) {
	configPath := filepath.Join(".", binaryName+".yaml")
	return loadConfigFile(configPath)
}

func loadConfigFile(path string) (*Config, error) {
	cfg := &Config{
		Commands: make(map[string]Command),
		Tools:    make(map[string]Tool),
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// Viper lowercases map keys, so we need to re-read env maps manually
	// to preserve the original case of environment variable names
	if err := rereadEnvMaps(v, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func rereadEnvMaps(v *viper.Viper, cfg *Config) error {
	configFile := v.ConfigFileUsed()
	if configFile == "" {
		return nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil // Ignore read errors, env will be empty
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil // Ignore parse errors
	}

	commandsRaw, ok := raw["commands"]
	if !ok {
		return nil
	}

	commands, ok := commandsRaw.(map[string]any)
	if !ok {
		return nil
	}

	for name, cmdRaw := range commands {
		cmdMap, ok := cmdRaw.(map[string]any)
		if !ok {
			continue
		}

		envRaw, ok := cmdMap["env"]
		if !ok {
			continue
		}

		envMap, ok := envRaw.(map[string]any)
		if !ok {
			continue
		}

		env := make(map[string]string)
		for k, v := range envMap {
			if strVal, ok := v.(string); ok {
				env[k] = strVal
			}
		}

		if cmd, exists := cfg.Commands[name]; exists {
			cmd.Env = env
			cfg.Commands[name] = cmd
		}
	}

	return nil
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

	// Local shell_options overrides global; otherwise inherit global.
	merged.ShellOptions = global.ShellOptions
	if local.ShellOptions != "" {
		merged.ShellOptions = local.ShellOptions
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
