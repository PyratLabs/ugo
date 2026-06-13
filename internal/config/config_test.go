package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFile(t *testing.T) {
	t.Run("missing file returns empty config", func(t *testing.T) {
		cfg, err := loadConfigFile("/nonexistent/path/config.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Commands) != 0 {
			t.Error("expected empty commands")
		}
		if len(cfg.Tools) != 0 {
			t.Error("expected empty tools")
		}
	})

	t.Run("valid config file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(`
commands:
  plan:
    cmd: echo plan
    description: "Plan"
    arguments:
      - name: env
        values: [dev, prod]
tools:
  docker:
    min_version: "20.0.0"
`), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := loadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := cfg.Commands["plan"]; !ok {
			t.Error("expected command 'plan'")
		}
		if cfg.Commands["plan"].Cmd != "echo plan" {
			t.Errorf("plan.cmd = %q, want %q", cfg.Commands["plan"].Cmd, "echo plan")
		}
		if len(cfg.Commands["plan"].Arguments) != 1 || cfg.Commands["plan"].Arguments[0].Name != "env" {
			t.Errorf("plan.arguments = %v, want [{env}]", cfg.Commands["plan"].Arguments)
		}
		if len(cfg.Commands["plan"].Arguments[0].Values) != 2 {
			t.Errorf("plan.arguments[0].values = %v, want [dev, prod]", cfg.Commands["plan"].Arguments[0].Values)
		}
		if _, ok := cfg.Tools["docker"]; !ok {
			t.Error("expected tool 'docker'")
		}
		if cfg.Tools["docker"].MinVersion != "20.0.0" {
			t.Errorf("docker.min_version = %q, want %q", cfg.Tools["docker"].MinVersion, "20.0.0")
		}
	})

	t.Run("parses shell_options", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(`
shell_options: "set -euo pipefail"
commands:
  plan:
    cmd: echo plan
`), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := loadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ShellOptions != "set -euo pipefail" {
			t.Errorf("ShellOptions = %q, want %q", cfg.ShellOptions, "set -euo pipefail")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(":\n  - invalid yaml{{"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := loadConfigFile(path)
		if err == nil {
			t.Error("expected error for invalid yaml")
		}
	})
}

func TestMergeConfigs(t *testing.T) {
	t.Run("empty configs", func(t *testing.T) {
		global := &Config{
			Commands: make(map[string]Command),
			Tools:    make(map[string]Tool),
		}
		local := &Config{
			Commands: make(map[string]Command),
			Tools:    make(map[string]Tool),
		}

		merged := mergeConfigs(global, local)
		if len(merged.Commands) != 0 {
			t.Error("expected empty commands")
		}
		if len(merged.Tools) != 0 {
			t.Error("expected empty tools")
		}
	})

	t.Run("local overrides global", func(t *testing.T) {
		global := &Config{
			Commands: map[string]Command{
				"plan": {Cmd: "echo global"},
				"lint": {Cmd: "echo global lint"},
			},
			Tools: map[string]Tool{
				"docker": {MinVersion: "1.0.0"},
				"go":     {MinVersion: "1.20"},
			},
		}
		local := &Config{
			Commands: map[string]Command{
				"plan": {Cmd: "echo local"},
				"test": {Cmd: "echo test"},
			},
			Tools: map[string]Tool{
				"docker": {MinVersion: "2.0.0"},
				"make":   {MinVersion: "4.0"},
			},
		}

		merged := mergeConfigs(global, local)

		// Local overrides
		if merged.Commands["plan"].Cmd != "echo local" {
			t.Errorf("plan.cmd = %q, want %q", merged.Commands["plan"].Cmd, "echo local")
		}
		if merged.Tools["docker"].MinVersion != "2.0.0" {
			t.Errorf("docker.min_version = %q, want %q", merged.Tools["docker"].MinVersion, "2.0.0")
		}

		// Global preserved
		if merged.Commands["lint"].Cmd != "echo global lint" {
			t.Errorf("lint.cmd = %q, want %q", merged.Commands["lint"].Cmd, "echo global lint")
		}
		if merged.Tools["go"].MinVersion != "1.20" {
			t.Errorf("go.min_version = %q, want %q", merged.Tools["go"].MinVersion, "1.20")
		}

		// Local additions
		if _, ok := merged.Commands["test"]; !ok {
			t.Error("expected command 'test' from local")
		}
		if _, ok := merged.Tools["make"]; !ok {
			t.Error("expected tool 'make' from local")
		}
	})

	t.Run("shell_options is preserved through merge", func(t *testing.T) {
		tests := []struct {
			name           string
			global         string
			local          string
			wantShellOpts  string
		}{
			{"local only", "", "set -euo pipefail", "set -euo pipefail"},
			{"global only", "set -e", "", "set -e"},
			{"local overrides global", "set -e", "set -euo pipefail", "set -euo pipefail"},
			{"neither set", "", "", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				global := &Config{ShellOptions: tt.global}
				local := &Config{ShellOptions: tt.local}

				merged := mergeConfigs(global, local)
				if merged.ShellOptions != tt.wantShellOpts {
					t.Errorf("merged.ShellOptions = %q, want %q", merged.ShellOptions, tt.wantShellOpts)
				}
			})
		}
	})
}

func TestConfigPaths(t *testing.T) {
	global, local := ConfigPaths("ugo")
	if global == "" {
		t.Error("expected non-empty global path")
	}
	if local != "ugo.yaml" {
		t.Errorf("local = %q, want %q", local, "ugo.yaml")
	}

	global2, local2 := ConfigPaths("myproject")
	if global2 == "" {
		t.Error("expected non-empty global path")
	}
	if local2 != "myproject.yaml" {
		t.Errorf("local = %q, want %q", local2, "myproject.yaml")
	}
}

func TestBinaryName(t *testing.T) {
	// BinaryName uses os.Args[0], which in tests is usually the test binary path
	name := BinaryName()
	if name == "" {
		t.Error("expected non-empty binary name")
	}
}
