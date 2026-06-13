package cmd

import (
	"bufio"
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
		{
			name:    "with prompts",
			cmdName: "login",
			def: config.Command{
				Cmd:         "echo ${password}",
				Description: "Login",
				Prompts:     []config.Prompt{{Name: "password", Description: "Enter password", Sensitive: true}},
			},
			wantUse:  "login",
			wantDesc: "Login",
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
	out := runVerb(t, "execgo", `
commands:
  hello:
    cmd: echo hello
    description: "Say hello"
`, "hello")

	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want to contain %q", out, "hello")
	}
}

func TestRootCmdExecuteMultiline(t *testing.T) {
	out := runVerb(t, "multigo", `
commands:
  multi:
    cmd: |
      echo step1
      echo step2
      echo step3
    description: "Run multiple commands"
`, "multi")

	for _, want := range []string{"step1", "step2", "step3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want to contain %q", out, want)
		}
	}
}

func TestRootCmdExecuteCmdsList(t *testing.T) {
	out := runVerb(t, "cmdsgo", `
commands:
  deploy:
    cmds:
      - echo "step 1"
      - echo "step 2"
      - echo "step 3"
    description: "Run multiple commands"
`, "deploy")

	for _, want := range []string{"step 1", "step 2", "step 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want to contain %q", out, want)
		}
	}
}

func TestRootCmdExecuteCmdsMultiline(t *testing.T) {
	out := runVerb(t, "cmdsmulti", `
commands:
  script:
    cmds:
      - |
        echo "line1"
        echo "line2"
    description: "Run multi-line script"
`, "script")

	for _, want := range []string{"line1", "line2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want to contain %q", out, want)
		}
	}
}

func TestBuildLong(t *testing.T) {
	tests := []struct {
		name      string
		arguments []config.Argument
		prompts   []config.Prompt
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
			want: []string{"^(?:[a-z][a-z0-9-]+)$"},
		},
		{
			name: "no validation",
			arguments: []config.Argument{
				{Name: "freeform"},
			},
			want: []string{"(no validation)"},
		},
		{
			name: "prompts in help",
			prompts: []config.Prompt{
				{Name: "token", Description: "API token", Sensitive: true},
				{Name: "username", Description: "User name"},
			},
			want: []string{"token", "API token", "(sensitive)", "username", "User name"},
		},
		{
			name: "both arguments and prompts",
			arguments: []config.Argument{
				{Name: "env", Values: []string{"dev", "prod"}},
			},
			prompts: []config.Prompt{
				{Name: "password", Description: "Enter password", Sensitive: true},
			},
			want: []string{"Arguments", "Prompts", "password", "Enter password", "(sensitive)", "env", "dev, prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLong(tt.arguments, tt.prompts)
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

// runVerb writes configYAML to a temp <binaryName>.yaml, builds the root
// command, runs the given args, and returns captured stdout. It sandboxes HOME
// (so the trust store and global config live in a temp dir) and passes --trust
// so the local config is accepted without prompting.
func runVerb(t *testing.T, binaryName, configYAML string, args ...string) string {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	configPath := filepath.Join(dir, binaryName+".yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()

	os.Args = []string{binaryName}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := RootCmd()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root.SetArgs(append([]string{"--trust"}, args...))
	execErr := root.Execute()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if execErr != nil {
		t.Fatalf("Execute() error = %v", execErr)
	}
	return buf.String()
}

// containsLine reports whether any line of s, trimmed of surrounding
// whitespace, equals want. Used to assert on a command's actual output
// rather than the echoed "running" display line.
func containsLine(s, want string) bool {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// trustFixture writes a local config and returns its path plus a fresh
// (empty) trust store path.
func trustFixture(t *testing.T) (localPath, storePath string) {
	t.Helper()
	dir := t.TempDir()
	localPath = filepath.Join(dir, "ugo.yaml")
	if err := os.WriteFile(localPath, []byte("commands:\n  build:\n    cmd: echo hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	storePath = filepath.Join(t.TempDir(), "trust.json")
	return localPath, storePath
}

func gate(localPath, storePath, answer string, allow, interactive bool) (string, error) {
	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader(answer))
	err := trustGate(localPath, storePath, in, &out, allow, interactive)
	return out.String(), err
}

func TestTrustGate(t *testing.T) {
	t.Run("no local config is allowed", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.yaml")
		storePath := filepath.Join(t.TempDir(), "trust.json")
		if _, err := gate(missing, storePath, "", false, false); err != nil {
			t.Errorf("expected nil for absent local config, got %v", err)
		}
	})

	t.Run("--trust bypasses prompt and records trust", func(t *testing.T) {
		localPath, storePath := trustFixture(t)
		if _, err := gate(localPath, storePath, "", true /*allow*/, false); err != nil {
			t.Fatalf("--trust should allow, got %v", err)
		}
		// A subsequent non-interactive run must now pass without --trust.
		if _, err := gate(localPath, storePath, "", false, false); err != nil {
			t.Errorf("config should be trusted after --trust, got %v", err)
		}
	})

	t.Run("non-interactive untrusted is blocked", func(t *testing.T) {
		localPath, storePath := trustFixture(t)
		_, err := gate(localPath, storePath, "", false, false /*interactive*/)
		if err == nil || !strings.Contains(err.Error(), "--trust") {
			t.Errorf("expected a blocking error mentioning --trust, got %v", err)
		}
	})

	t.Run("interactive yes trusts and persists", func(t *testing.T) {
		localPath, storePath := trustFixture(t)
		out, err := gate(localPath, storePath, "y\n", false, true)
		if err != nil {
			t.Fatalf("expected trust granted, got %v", err)
		}
		if !strings.Contains(out, "Trust it?") {
			t.Errorf("expected a prompt, got %q", out)
		}
		if _, err := gate(localPath, storePath, "", false, false); err != nil {
			t.Errorf("config should be trusted after yes, got %v", err)
		}
	})

	t.Run("interactive no aborts", func(t *testing.T) {
		localPath, storePath := trustFixture(t)
		for _, answer := range []string{"n\n", "\n", "nope\n"} {
			_, err := gate(localPath, storePath, answer, false, true)
			if err == nil || !strings.Contains(err.Error(), "aborting") {
				t.Errorf("answer %q: expected abort error, got %v", answer, err)
			}
		}
	})

	t.Run("editing a trusted config re-prompts", func(t *testing.T) {
		localPath, storePath := trustFixture(t)
		if _, err := gate(localPath, storePath, "", true, false); err != nil {
			t.Fatalf("initial trust: %v", err)
		}

		// Modify the config: trust must be revoked (Changed), so a
		// non-interactive run is blocked again.
		if err := os.WriteFile(localPath, []byte("commands:\n  build:\n    cmd: echo PWNED\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := gate(localPath, storePath, "", false, false); err == nil {
			t.Error("expected a changed config to be blocked until re-trusted")
		}

		// Re-prompt should mention that it changed.
		out, err := gate(localPath, storePath, "y\n", false, true)
		if err != nil {
			t.Fatalf("re-trust: %v", err)
		}
		if !strings.Contains(out, "changed") {
			t.Errorf("expected 'changed' notice on re-prompt, got %q", out)
		}
	})
}

func TestRootCmdTrustFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"trustflaggo"}
	os.Chdir(t.TempDir())

	root := RootCmd()
	if flag := root.PersistentFlags().Lookup("trust"); flag == nil {
		t.Fatal("expected --trust flag")
	}
}

func TestRootCmdExecuteSingleLineQuoting(t *testing.T) {
	// Regression: a single-line cmd must preserve quoted whitespace instead of
	// collapsing it during whitespace tokenization (strings.Fields).
	out := runVerb(t, "qgo", `
commands:
  spaces:
    cmd: echo "hello   world"
    description: "Quoted whitespace"
`, "spaces")

	if !containsLine(out, "hello   world") {
		t.Errorf("expected an output line %q, got:\n%s", "hello   world", out)
	}
}

func TestRootCmdExecuteSingleLineShellOperators(t *testing.T) {
	// Regression: shell operators in a single-line cmd must be interpreted by
	// the shell, not passed as literal arguments to the first word.
	t.Run("&& chains", func(t *testing.T) {
		out := runVerb(t, "andgo", `
commands:
  chain:
    cmd: echo first && echo second
    description: "Operator chain"
`, "chain")

		if !containsLine(out, "first") || !containsLine(out, "second") {
			t.Errorf("expected output lines %q and %q, got:\n%s", "first", "second", out)
		}
	})

	t.Run("pipes", func(t *testing.T) {
		out := runVerb(t, "pipego", `
commands:
  pipe:
    cmd: echo "hello world" | tr ' ' '_'
    description: "Pipeline"
`, "pipe")

		if !containsLine(out, "hello_world") {
			t.Errorf("expected an output line %q, got:\n%s", "hello_world", out)
		}
	})
}

func TestExpandEnv(t *testing.T) {
	vars := map[string]string{
		"env":      "prod",
		"api_key":  "secret123",
		"username": "admin",
	}

	t.Run("expands all env values", func(t *testing.T) {
		env := map[string]string{
			"DEPLOY_ENV":  "${env}",
			"API_KEY":     "${api_key}",
			"USER":        "${username}",
		}
		got := expandEnv(env, vars)
		if got["DEPLOY_ENV"] != "prod" {
			t.Errorf("DEPLOY_ENV = %q, want %q", got["DEPLOY_ENV"], "prod")
		}
		if got["API_KEY"] != "secret123" {
			t.Errorf("API_KEY = %q, want %q", got["API_KEY"], "secret123")
		}
		if got["USER"] != "admin" {
			t.Errorf("USER = %q, want %q", got["USER"], "admin")
		}
	})

	t.Run("preserves literal values with no vars", func(t *testing.T) {
		env := map[string]string{
			"STATIC": "hello",
		}
		got := expandEnv(env, vars)
		if got["STATIC"] != "hello" {
			t.Errorf("STATIC = %q, want %q", got["STATIC"], "hello")
		}
	})

	t.Run("empty env returns empty", func(t *testing.T) {
		got := expandEnv(nil, vars)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
		got2 := expandEnv(map[string]string{}, vars)
		if len(got2) != 0 {
			t.Errorf("expected empty map, got %v", got2)
		}
	})

	t.Run("missing var leaves placeholder", func(t *testing.T) {
		env := map[string]string{
			"MISSING": "${nonexistent}",
		}
		got := expandEnv(env, vars)
		if got["MISSING"] != "${nonexistent}" {
			t.Errorf("MISSING = %q, want %q", got["MISSING"], "${nonexistent}")
		}
	})

	t.Run("mixed static and dynamic values", func(t *testing.T) {
		env := map[string]string{
			"ENV":      "${env}",
			"STATIC":   "static-value",
		}
		got := expandEnv(env, vars)
		if got["ENV"] != "prod" {
			t.Errorf("ENV = %q, want %q", got["ENV"], "prod")
		}
		if got["STATIC"] != "static-value" {
			t.Errorf("STATIC = %q, want %q", got["STATIC"], "static-value")
		}
	})
}
