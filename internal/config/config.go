package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
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

// Command defines a single verb's configuration
type Command struct {
	Name        string     `mapstructure:"name"`
	Cmd         string     `mapstructure:"cmd"`
	Description string     `mapstructure:"description"`
	Arguments   []Argument `mapstructure:"arguments"`
}

// Config represents the full YAML configuration
type Config struct {
	Commands map[string]Command `mapstructure:"commands"`
	Tools    map[string]Tool    `mapstructure:"tools"`
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

	return cfg, nil
}

func mergeConfigs(global, local *Config) *Config {
	merged := &Config{
		Commands: make(map[string]Command),
		Tools:    make(map[string]Tool),
	}

	for name, cmd := range global.Commands {
		merged.Commands[name] = cmd
	}

	for name, cmd := range local.Commands {
		merged.Commands[name] = cmd
	}

	for name, tool := range global.Tools {
		merged.Tools[name] = tool
	}

	for name, tool := range local.Tools {
		merged.Tools[name] = tool
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
