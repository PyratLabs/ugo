package args

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PyratLabs/ugo/internal/config"
)

func TestValidate(t *testing.T) {
	t.Run("no rules", func(t *testing.T) {
		arg := config.Argument{Name: "foo"}
		if err := Validate(arg, "anything"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("enum pass", func(t *testing.T) {
		arg := config.Argument{
			Name:   "env",
			Values: []string{"dev", "staging", "prod"},
		}
		for _, v := range arg.Values {
			if err := Validate(arg, v); err != nil {
				t.Errorf("Validate(%q) = %v, want nil", v, err)
			}
		}
	})

	t.Run("enum fail", func(t *testing.T) {
		arg := config.Argument{
			Name:   "env",
			Values: []string{"dev", "staging", "prod"},
		}
		err := Validate(arg, "invalid")
		if err == nil {
			t.Error("expected error for invalid enum value")
		}
	})

	t.Run("regex pass", func(t *testing.T) {
		arg := config.Argument{
			Name:  "service",
			Match: "[a-z][a-z0-9-]+",
		}
		for _, v := range []string{"my-service", "api", "web-app-2"} {
			if err := Validate(arg, v); err != nil {
				t.Errorf("Validate(%q) = %v, want nil", v, err)
			}
		}
	})

	t.Run("regex fail", func(t *testing.T) {
		arg := config.Argument{
			Name:  "service",
			Match: "[a-z][a-z0-9-]+",
		}
		for _, v := range []string{"InvalidService", "123start", ""} {
			err := Validate(arg, v)
			if err == nil {
				t.Errorf("Validate(%q) = nil, want error", v)
			}
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		arg := config.Argument{
			Name:  "foo",
			Match: "[invalid",
		}
		err := Validate(arg, "anything")
		if err == nil {
			t.Error("expected error for invalid regex")
		}
	})

	t.Run("glob pass - full path", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []string{"a.yaml", "b.yaml"} {
			if err := os.WriteFile(filepath.Join(dir, f), nil, 0644); err != nil {
				t.Fatal(err)
			}
		}
		arg := config.Argument{
			Name:  "file",
			Match: filepath.Join(dir, "*.yaml"),
		}
		if err := Validate(arg, filepath.Join(dir, "a.yaml")); err != nil {
			t.Errorf("Validate(full path) = %v, want nil", err)
		}
	})

	t.Run("glob pass - basename", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		arg := config.Argument{
			Name:  "file",
			Match: filepath.Join(dir, "*.yaml"),
		}
		if err := Validate(arg, "config.yaml"); err != nil {
			t.Errorf("Validate(basename) = %v, want nil", err)
		}
	})

	t.Run("glob pass - basename without ext", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "playbook.yaml"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		arg := config.Argument{
			Name:  "file",
			Match: filepath.Join(dir, "*.yaml"),
		}
		if err := Validate(arg, "playbook"); err != nil {
			t.Errorf("Validate(basename without ext) = %v, want nil", err)
		}
	})

	t.Run("glob pass - directory name", func(t *testing.T) {
		dir := t.TempDir()
		envDir := filepath.Join(dir, "prod")
		if err := os.Mkdir(envDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(envDir, "inventory.yaml"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		arg := config.Argument{
			Name:  "environment",
			Match: filepath.Join(dir, "*/inventory.yaml"),
		}
		if err := Validate(arg, "prod"); err != nil {
			t.Errorf("Validate(directory name) = %v, want nil", err)
		}
	})

	t.Run("glob fail", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "only.yaml"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		arg := config.Argument{
			Name:  "file",
			Match: filepath.Join(dir, "*.yaml"),
		}
		err := Validate(arg, "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("glob no matches", func(t *testing.T) {
		dir := t.TempDir()
		arg := config.Argument{
			Name:  "file",
			Match: filepath.Join(dir, "*.yaml"),
		}
		err := Validate(arg, "anything")
		if err == nil {
			t.Error("expected error when no files match glob")
		}
	})

	t.Run("exclude pass", func(t *testing.T) {
		arg := config.Argument{
			Name:    "workspace",
			Values:  []string{"dev", "staging", "prod"},
			Exclude: []string{"staging"},
		}
		if err := Validate(arg, "dev"); err != nil {
			t.Errorf("Validate(dev) = %v, want nil", err)
		}
	})

	t.Run("exclude fail", func(t *testing.T) {
		arg := config.Argument{
			Name:    "workspace",
			Values:  []string{"dev", "staging", "prod"},
			Exclude: []string{"staging"},
		}
		err := Validate(arg, "staging")
		if err == nil {
			t.Error("expected error for excluded value")
		}
	})

	t.Run("exclude with glob - directory name", func(t *testing.T) {
		dir := t.TempDir()
		for _, env := range []string{"dev", "default", "prod"} {
			envDir := filepath.Join(dir, env)
			os.Mkdir(envDir, 0755)
			os.WriteFile(filepath.Join(envDir, "inventory.yaml"), nil, 0644)
		}
		arg := config.Argument{
			Name:    "environment",
			Match:   filepath.Join(dir, "*/inventory.yaml"),
			Exclude: []string{"default"},
		}
		if err := Validate(arg, "dev"); err != nil {
			t.Errorf("Validate(dev) = %v, want nil", err)
		}
		err := Validate(arg, "default")
		if err == nil {
			t.Error("expected error for excluded directory name")
		}
	})

	t.Run("both enum and regex fail enum", func(t *testing.T) {
		arg := config.Argument{
			Name:   "env",
			Values: []string{"dev", "prod"},
			Match:  "[a-z]+",
		}
		err := Validate(arg, "invalid")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestValidateArgs(t *testing.T) {
	argDefs := []config.Argument{
		{Name: "env", Values: []string{"dev", "prod"}},
		{Name: "service", Match: "[a-z]+"},
	}

	t.Run("all valid", func(t *testing.T) {
		errs := ValidateArgs(argDefs, []string{"dev", "api"})
		if len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
	})

	t.Run("first invalid", func(t *testing.T) {
		errs := ValidateArgs(argDefs, []string{"staging", "api"})
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}
	})

	t.Run("both invalid", func(t *testing.T) {
		errs := ValidateArgs(argDefs, []string{"staging", "123"})
		if len(errs) != 2 {
			t.Errorf("expected 2 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestArgNames(t *testing.T) {
	args := []config.Argument{
		{Name: "env"},
		{Name: "service"},
	}
	names := ArgNames(args)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "env" || names[1] != "service" {
		t.Errorf("names = %v, want [env, service]", names)
	}
}

func TestArgMap(t *testing.T) {
	argDefs := []config.Argument{
		{Name: "env"},
		{Name: "service"},
	}
	m := ArgMap(argDefs, []string{"dev", "api"})
	if m["env"] != "dev" {
		t.Errorf("m[env] = %q, want %q", m["env"], "dev")
	}
	if m["service"] != "api" {
		t.Errorf("m[service] = %q, want %q", m["service"], "api")
	}
}

func TestIsGlob(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"*.yaml", true},
		{"playbooks/*", true},
		{"?.txt", true},
		{"[a-z]+", false},
		{"^[a-z]+$", false},
		{"exact.yaml", false},
		{"simple", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := IsGlob(tt.pattern)
			if got != tt.want {
				t.Errorf("isGlob(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestStripExt(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"playbook.yaml", "playbook"},
		{"config.json", "config"},
		{"noext", "noext"},
		{".hidden", ""}, // filepath.Ext treats .hidden as extension
		{"file.tar.gz", "file.tar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripExt(tt.name)
			if got != tt.want {
				t.Errorf("stripExt(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestRegexFullMatch(t *testing.T) {
	arg := config.Argument{
		Name:  "id",
		Match: "[a-z][0-9]",
	}

	if err := Validate(arg, "a1"); err != nil {
		t.Errorf("Validate(a1) = %v, want nil", err)
	}

	if err := Validate(arg, "xa1x"); err == nil {
		t.Error("Validate(xa1x) = nil, want error (should not partial match)")
	}
}

func TestGlobMatches(t *testing.T) {
	t.Run("file glob returns basenames without ext", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []string{"web.yaml", "db.yaml", "api.yaml"} {
			os.WriteFile(filepath.Join(dir, f), nil, 0644)
		}
		got := GlobMatches(filepath.Join(dir, "*.yaml"), nil)
		if len(got) != 3 {
			t.Fatalf("expected 3 matches, got %d: %v", len(got), got)
		}
	})

	t.Run("directory glob returns dir names", func(t *testing.T) {
		dir := t.TempDir()
		for _, env := range []string{"dev", "staging", "prod"} {
			envDir := filepath.Join(dir, env)
			os.Mkdir(envDir, 0755)
			os.WriteFile(filepath.Join(envDir, "inventory.yaml"), nil, 0644)
		}
		got := GlobMatches(filepath.Join(dir, "*/inventory.yaml"), nil)
		if len(got) != 3 {
			t.Fatalf("expected 3 matches, got %d: %v", len(got), got)
		}
	})

	t.Run("exclude filters out values", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []string{"default.yaml", "prod.yaml", "dev.yaml"} {
			os.WriteFile(filepath.Join(dir, f), nil, 0644)
		}
		got := GlobMatches(filepath.Join(dir, "*.yaml"), []string{"default"})
		for _, name := range got {
			if name == "default" {
				t.Errorf("expected 'default' to be excluded, got %v", got)
			}
		}
		if len(got) != 2 {
			t.Errorf("expected 2 matches after exclude, got %d: %v", len(got), got)
		}
	})

	t.Run("exclude filters out directory names", func(t *testing.T) {
		dir := t.TempDir()
		for _, env := range []string{"default", "prod", "dev"} {
			envDir := filepath.Join(dir, env)
			os.Mkdir(envDir, 0755)
			os.WriteFile(filepath.Join(envDir, "inventory.yaml"), nil, 0644)
		}
		got := GlobMatches(filepath.Join(dir, "*/inventory.yaml"), []string{"default"})
		for _, name := range got {
			if name == "default" {
				t.Errorf("expected 'default' to be excluded, got %v", got)
			}
		}
		if len(got) != 2 {
			t.Errorf("expected 2 matches after exclude, got %d: %v", len(got), got)
		}
	})

	t.Run("no matches returns nil", func(t *testing.T) {
		dir := t.TempDir()
		got := GlobMatches(filepath.Join(dir, "*.yaml"), nil)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}
