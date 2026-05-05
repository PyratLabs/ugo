package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PyratLabs/ugo/internal/config"
)

func TestBuildUse(t *testing.T) {
	tests := []struct {
		name      string
		verb      string
		arguments []config.Argument
		want      string
	}{
		{
			name:      "no arguments",
			verb:      "lint",
			arguments: nil,
			want:      "lint",
		},
		{
			name:      "single argument",
			verb:      "plan",
			arguments: []config.Argument{{Name: "environment"}},
			want:      "plan <environment>",
		},
		{
			name:      "multiple arguments",
			verb:      "deploy",
			arguments: []config.Argument{{Name: "environment"}, {Name: "service"}, {Name: "region"}},
			want:      "deploy <environment> <service> <region>",
		},
		{
			name:      "empty arguments slice",
			verb:      "test",
			arguments: []config.Argument{},
			want:      "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildUse(tt.verb, tt.arguments)
			if got != tt.want {
				t.Errorf("buildUse(%q, %v) = %q, want %q", tt.verb, tt.arguments, got, tt.want)
			}
		})
	}
}

func TestRootCmd(t *testing.T) {
	// Save and restore os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "testgo.yaml")
	if err := os.WriteFile(configPath, []byte(`
commands:
  plan:
    cmd: echo plan
    description: "Plan the environment"
    arguments:
      - name: environment
      - name: playbook
  lint:
    cmd: echo lint
    description: "Run linter"
`), 0644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"testgo"}
	os.Chdir(dir)

	root := RootCmd()

	// Verify root command properties
	if root.Use != "testgo" {
		t.Errorf("root.Use = %q, want %q", root.Use, "testgo")
	}
	if !strings.Contains(root.Short, "testgo") {
		t.Errorf("root.Short = %q, want to contain %q", root.Short, "testgo")
	}

	// Verify subcommands were created
	sub := root.Commands()
	found := map[string]bool{}
	for _, c := range sub {
		found[c.Name()] = true
	}

	if !found["plan"] {
		t.Error("expected subcommand 'plan'")
	}
	if !found["lint"] {
		t.Error("expected subcommand 'lint'")
	}
	if !found["check"] {
		t.Error("expected built-in subcommand 'check'")
	}

	// Verify help is available (Cobra adds it implicitly)
	_, _, err := root.Find([]string{"help"})
	if err != nil {
		t.Logf("help command lookup: %v (may be implicit)", err)
	}
}

func TestRootCmdNoConfig(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	os.Args = []string{"noconfig"}
	os.Chdir(dir)

	// Should not panic when no config exists
	root := RootCmd()
	if root.Use != "noconfig" {
		t.Errorf("root.Use = %q, want %q", root.Use, "noconfig")
	}
}

func TestRootCmdNoColorFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	os.Args = []string{"testgo"}
	os.Chdir(dir)

	root := RootCmd()

	// Verify --no-color flag exists
	flag := root.PersistentFlags().Lookup("no-color")
	if flag == nil {
		t.Fatal("expected --no-color flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("no-color default = %q, want %q", flag.DefValue, "false")
	}
}

func TestBuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmdName  string
		def      config.Command
		wantUse  string
		wantDesc string
	}{
		{
			name:    "with arguments",
			cmdName: "plan",
			def: config.Command{
				Cmd:         "echo plan",
				Description: "Plan the env",
				Arguments:   []config.Argument{{Name: "environment"}},
			},
			wantUse:  "plan <environment>",
			wantDesc: "Plan the env",
		},
		{
			name:    "without arguments",
			cmdName: "lint",
			def: config.Command{
				Cmd:         "echo lint",
				Description: "Run linter",
			},
			wantUse:  "lint",
			wantDesc: "Run linter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildCommand(tt.cmdName, tt.def)
			if cmd.Use != tt.wantUse {
				t.Errorf("command.Use = %q, want %q", cmd.Use, tt.wantUse)
			}
			if cmd.Short != tt.wantDesc {
				t.Errorf("command.Short = %q, want %q", cmd.Short, tt.wantDesc)
			}
		})
	}
}

func TestRootCmdExecute(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "execgo.yaml")
	if err := os.WriteFile(configPath, []byte(`
commands:
  hello:
    cmd: echo hello
    description: "Say hello"
`), 0644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"execgo"}
	os.Chdir(dir)

	root := RootCmd()

	// Capture stdout since the command writes to os.Stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root.SetArgs([]string{"hello"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	w.Close()
	var out bytes.Buffer
	out.ReadFrom(r)
	os.Stdout = oldStdout

	if !strings.Contains(out.String(), "hello") {
		t.Errorf("output = %q, want to contain %q", out.String(), "hello")
	}
}

func TestRootCmdExecuteMultiline(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "multigo.yaml")
	if err := os.WriteFile(configPath, []byte(`
commands:
  multi:
    cmd: |
      echo step1
      echo step2
      echo step3
    description: "Run multiple commands"
`), 0644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"multigo"}
	os.Chdir(dir)

	root := RootCmd()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root.SetArgs([]string{"multi"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	w.Close()
	var out bytes.Buffer
	out.ReadFrom(r)
	os.Stdout = oldStdout

	output := out.String()
	for _, want := range []string{"step1", "step2", "step3"} {
		if !strings.Contains(output, want) {
			t.Errorf("output = %q, want to contain %q", output, want)
		}
	}
}

func TestRootCmdExecuteCmdsList(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "cmdsgo.yaml")
	if err := os.WriteFile(configPath, []byte(`
commands:
  deploy:
    cmds:
      - echo "step 1"
      - echo "step 2"
      - echo "step 3"
    description: "Run multiple commands"
`), 0644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"cmdsgo"}
	os.Chdir(dir)

	root := RootCmd()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root.SetArgs([]string{"deploy"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	w.Close()
	var out bytes.Buffer
	out.ReadFrom(r)
	os.Stdout = oldStdout

	output := out.String()
	for _, want := range []string{"step 1", "step 2", "step 3"} {
		if !strings.Contains(output, want) {
			t.Errorf("output = %q, want to contain %q", output, want)
		}
	}
}

func TestRootCmdExecuteCmdsMultiline(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "cmdsmulti.yaml")
	if err := os.WriteFile(configPath, []byte(`
commands:
  script:
    cmds:
      - |
        echo "line1"
        echo "line2"
    description: "Run multi-line script"
`), 0644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"cmdsmulti"}
	os.Chdir(dir)

	root := RootCmd()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root.SetArgs([]string{"script"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	w.Close()
	var out bytes.Buffer
	out.ReadFrom(r)
	os.Stdout = oldStdout

	output := out.String()
	for _, want := range []string{"line1", "line2"} {
		if !strings.Contains(output, want) {
			t.Errorf("output = %q, want to contain %q", output, want)
		}
	}
}

func TestBuildLong(t *testing.T) {
	tests := []struct {
		name      string
		arguments []config.Argument
		want      []string
	}{
		{
			name:      "no arguments",
			arguments: nil,
			want:      nil,
		},
		{
			name: "enum values",
			arguments: []config.Argument{
				{Name: "env", Values: []string{"dev", "staging", "prod"}},
			},
			want: []string{"dev, staging, prod"},
		},
		{
			name: "regex match",
			arguments: []config.Argument{
				{Name: "service", Match: "[a-z][a-z0-9-]+"},
			},
			want: []string{"^[a-z][a-z0-9-]+$"},
		},
		{
			name: "no validation",
			arguments: []config.Argument{
				{Name: "freeform"},
			},
			want: []string{"(no validation)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLong(tt.arguments)
			if len(tt.want) == 0 {
				if got != "" {
					t.Errorf("buildLong() = %q, want empty", got)
				}
				return
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("buildLong() = %q, want to contain %q", got, w)
				}
			}
		})
	}
}
