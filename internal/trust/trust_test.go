package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	writeFile(t, a, "commands: {}\n")
	writeFile(t, b, "commands: {}\n")

	ha, err := HashFile(a)
	if err != nil {
		t.Fatalf("HashFile(a): %v", err)
	}
	if ha == "" {
		t.Fatal("expected a non-empty hash")
	}

	hb, _ := HashFile(b)
	if ha != hb {
		t.Errorf("identical content should hash equal: %q vs %q", ha, hb)
	}

	writeFile(t, b, "commands: {}\n# changed\n")
	hb2, _ := HashFile(b)
	if hb2 == ha {
		t.Error("changed content should produce a different hash")
	}

	if _, err := HashFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("expected error hashing a missing file")
	}
}

func TestStoreStatus(t *testing.T) {
	store := filepath.Join(t.TempDir(), "trust.json")
	s, err := Load(store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	const path = "/projects/app/ugo.yaml"

	if got := s.Status(path, "hash1"); got != Unknown {
		t.Errorf("Status(new) = %v, want Unknown", got)
	}

	if err := s.Trust(path, "hash1"); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	if got := s.Status(path, "hash1"); got != Trusted {
		t.Errorf("Status(same hash) = %v, want Trusted", got)
	}
	if got := s.Status(path, "hash2"); got != Changed {
		t.Errorf("Status(different hash) = %v, want Changed", got)
	}
	if got := s.Status("/other/ugo.yaml", "hash1"); got != Unknown {
		t.Errorf("Status(other path) = %v, want Unknown", got)
	}
}

func TestStorePersistence(t *testing.T) {
	store := filepath.Join(t.TempDir(), "nested", "dir", "trust.json")

	s, err := Load(store)
	if err != nil {
		t.Fatalf("Load (missing): %v", err)
	}
	if err := s.Trust("/projects/app/ugo.yaml", "abc123"); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	// Trust must create parent directories and persist to disk.
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("trust store not written: %v", err)
	}

	// A fresh load must see the recorded entry.
	reloaded, err := Load(store)
	if err != nil {
		t.Fatalf("Load (existing): %v", err)
	}
	if got := reloaded.Status("/projects/app/ugo.yaml", "abc123"); got != Trusted {
		t.Errorf("after reload Status = %v, want Trusted", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load of missing file should not error: %v", err)
	}
	if got := s.Status("/x", "h"); got != Unknown {
		t.Errorf("empty store Status = %v, want Unknown", got)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	store := filepath.Join(t.TempDir(), "trust.json")
	writeFile(t, store, "{ not valid json")
	if _, err := Load(store); err == nil {
		t.Error("expected error loading corrupt trust store")
	}
}

func TestDefaultStorePath(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	got, err := DefaultStorePath("myproj")
	if err != nil {
		t.Fatalf("DefaultStorePath: %v", err)
	}
	want := filepath.Join("/home/tester", ".config", "myproj", "trust.json")
	if got != want {
		t.Errorf("DefaultStorePath = %q, want %q", got, want)
	}
}
