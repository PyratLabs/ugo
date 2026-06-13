package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"bare semver", "2.10.5", "v2.10.5"},
		{"with v prefix", "v1.2.3", "v1.2.3"},
		{"embedded in text", "ansible-playbook 2.14.0", "v2.14.0"},
		{"with v in text", "version v3.0.1-beta", "v3.0.1"},
		{"docker style", "Docker version 24.0.7, build afdd53b", "v24.0.7"},
		{"terraform style", "Terraform v1.6.2\non linux_amd64", "v1.6.2"},
		{"no version", "some random output", ""},
		{"empty", "", ""},
		{"partial version", "v1.2", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractVersion(tt.input)
			if got != tt.expect {
				t.Errorf("ExtractVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "fake-version")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"fake-tool v1.5.0\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		versionCmd  string
		minVersion  string
		maxVersion  string
		wantVer     string
		wantErr     bool
		errContains string
	}{
		{
			name:       "version within range",
			versionCmd: scriptPath,
			minVersion: "1.0.0",
			maxVersion: "2.0.0",
			wantVer:    "v1.5.0",
		},
		{
			name:        "below minimum",
			versionCmd:  scriptPath,
			minVersion:  "2.0.0",
			wantErr:     true,
			errContains: "below minimum",
		},
		{
			name:        "above maximum",
			versionCmd:  scriptPath,
			maxVersion:  "1.4.0",
			wantErr:     true,
			errContains: "exceeds maximum",
		},
		{
			name:       "no constraints",
			versionCmd: scriptPath,
			wantVer:    "v1.5.0",
		},
		{
			name:       "min only",
			versionCmd: scriptPath,
			minVersion: "1.5.0",
			wantVer:    "v1.5.0",
		},
		{
			name:       "max only",
			versionCmd: scriptPath,
			maxVersion: "1.5.0",
			wantVer:    "v1.5.0",
		},
		{
			name:        "command not found",
			versionCmd:  "nonexistent-command-xyz --version",
			wantErr:     true,
			errContains: "failed to run version command",
		},
		{
			name:        "empty version command",
			versionCmd:  "",
			wantErr:     true,
			errContains: "empty version command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVer, err := Check("test-tool", tt.versionCmd, tt.minVersion, tt.maxVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil {
					t.Errorf("Check() expected error containing %q, got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Check() error = %q, want to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if gotVer != tt.wantVer {
				t.Errorf("Check() version = %q, want %q", gotVer, tt.wantVer)
			}
		})
	}
}

func TestCheckReadsStderr(t *testing.T) {
	// Many tools (e.g. "java -version") print their version to stderr.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "stderr-version")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"stderr-tool v2.3.4\" >&2\n"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := Check("stderr-tool", scriptPath, "2.0.0", "3.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v2.3.4" {
		t.Errorf("Check() version = %q, want %q", got, "v2.3.4")
	}
}

func TestCheckTimeout(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "slow-version")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 5\necho v1.0.0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	orig := versionCmdTimeout
	versionCmdTimeout = 100 * time.Millisecond
	defer func() { versionCmdTimeout = orig }()

	start := time.Now()
	_, err := Check("slow-tool", scriptPath, "", "")
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Check() took %s, expected it to abort well before the script's 5s sleep", elapsed)
	}
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Check() error = %v, want a timeout error", err)
	}
}
