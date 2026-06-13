package checker

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PyratLabs/ugo/internal/config"
)

func TestCheckTools(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "fake-tool")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"fake-tool v1.5.0\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	badScriptPath := filepath.Join(tmp, "bad-version")
	if err := os.WriteFile(badScriptPath, []byte("#!/bin/sh\necho \"no version here\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("installed tool with passing version", func(t *testing.T) {
		tools := map[string]config.Tool{
			scriptPath: {
				MinVersion: "1.0.0",
				VersionCmd: scriptPath,
			},
		}
		issues := CheckTools(tools)
		if HasErrors(issues) {
			t.Errorf("expected no errors, got: %v", FormatErrors(issues))
		}
	})

	t.Run("installed tool with failing version", func(t *testing.T) {
		tools := map[string]config.Tool{
			scriptPath: {
				MinVersion: "2.0.0",
				VersionCmd: scriptPath,
			},
		}
		issues := CheckTools(tools)
		if !HasErrors(issues) {
			t.Error("expected errors for version below minimum")
		}
		if len(issues) != 1 {
			t.Fatalf("expected 1 issue, got %d", len(issues))
		}
	})

	t.Run("missing tool without download URL", func(t *testing.T) {
		tools := map[string]config.Tool{
			"nonexistent-tool-xyz": {},
		}
		issues := CheckTools(tools)
		if !HasErrors(issues) {
			t.Error("expected errors for missing tool")
		}
		if len(issues) != 1 {
			t.Fatalf("expected 1 issue, got %d", len(issues))
		}
		if issues[0].Tool != "nonexistent-tool-xyz" {
			t.Errorf("issue.Tool = %q, want %q", issues[0].Tool, "nonexistent-tool-xyz")
		}
	})

	t.Run("missing tool with download URL", func(t *testing.T) {
		tools := map[string]config.Tool{
			"nonexistent-tool-xyz": {
				DownloadURL: "https://example.com/download",
			},
		}
		issues := CheckTools(tools)
		if !HasErrors(issues) {
			t.Error("expected errors for missing tool")
		}
		if len(issues) != 1 {
			t.Fatalf("expected 1 issue, got %d", len(issues))
		}
		if !slices.Contains(issues[0].Errors, "nonexistent-tool-xyz is not installed, download at: https://example.com/download") {
			t.Errorf("expected download URL in error, got: %v", issues[0].Errors)
		}
	})

	t.Run("tool with unparseable version", func(t *testing.T) {
		tools := map[string]config.Tool{
			badScriptPath: {
				MinVersion: "1.0.0",
				VersionCmd: badScriptPath,
			},
		}
		issues := CheckTools(tools)
		if !HasErrors(issues) {
			t.Error("expected errors for unparseable version")
		}
	})

	t.Run("empty tools map", func(t *testing.T) {
		tools := map[string]config.Tool{}
		issues := CheckTools(tools)
		if len(issues) != 0 {
			t.Errorf("expected no issues, got %d", len(issues))
		}
	})

	t.Run("issues are returned in sorted, deterministic order", func(t *testing.T) {
		// All three are missing, so each yields one issue. Map iteration in Go
		// is randomized, so a stable result must come from explicit sorting.
		tools := map[string]config.Tool{
			"zzz-missing-tool": {},
			"aaa-missing-tool": {},
			"mmm-missing-tool": {},
		}
		want := []string{"aaa-missing-tool", "mmm-missing-tool", "zzz-missing-tool"}

		for range 20 {
			issues := CheckTools(tools)
			if len(issues) != len(want) {
				t.Fatalf("expected %d issues, got %d", len(want), len(issues))
			}
			for j, w := range want {
				if issues[j].Tool != w {
					t.Errorf("issues[%d].Tool = %q, want %q", j, issues[j].Tool, w)
				}
			}
		}
	})
}

func TestHasErrors(t *testing.T) {
	tests := []struct {
		name    string
		issues  []Issue
		wantErr bool
	}{
		{
			name:    "no issues",
			issues:  nil,
			wantErr: false,
		},
		{
			name: "only version info",
			issues: []Issue{
				{Tool: "docker", Errors: []string{"version: v24.0.7"}},
			},
			wantErr: false,
		},
		{
			name: "with error",
			issues: []Issue{
				{Tool: "missing", Errors: []string{"missing is not installed"}},
			},
			wantErr: true,
		},
		{
			name: "mixed",
			issues: []Issue{
				{Tool: "docker", Errors: []string{"version: v24.0.7"}},
				{Tool: "missing", Errors: []string{"missing is not installed"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasErrors(tt.issues)
			if got != tt.wantErr {
				t.Errorf("HasErrors() = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestFormatErrors(t *testing.T) {
	issues := []Issue{
		{Tool: "missing", Errors: []string{"missing is not installed"}},
		{Tool: "docker", Errors: []string{"version: v24.0.7"}},
	}

	got := FormatErrors(issues)
	if got == "" {
		t.Error("expected non-empty output")
	}
	// Should not include version info
	if len(got) > 0 && got[0:2] != "  " {
		t.Errorf("expected indented output, got: %q", got)
	}
}
