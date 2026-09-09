// Package trust implements a direnv-style trust store. Local configuration is
// loaded from the working directory, so before uGo executes anything defined in
// it the config must be trusted. Trust is content-addressed: a config is keyed
// by its absolute path and the SHA-256 of its contents, so editing a trusted
// config revokes trust until it is granted again.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Status describes the trust state of a config file.
type Status int

const (
	// Unknown means the config path has never been trusted.
	Unknown Status = iota
	// Changed means the path was trusted before but its contents have changed.
	Changed
	// Trusted means the path is trusted and its contents match.
	Trusted
)

// Store is the on-disk trust database, mapping an absolute config path to the
// hex SHA-256 of the contents that were trusted.
type Store struct {
	path    string
	entries map[string]string
}

// Load reads the trust store at path. A missing or empty file yields an empty
// store rather than an error.
func Load(path string) (*Store, error) {
	s := &Store{path: path, entries: map[string]string{}}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.entries); err != nil {
		return nil, err
	}
	return s, nil
}

// Status reports whether configPath with the given content hash is trusted.
func (s *Store) Status(configPath, hash string) Status {
	stored, ok := s.entries[configPath]
	switch {
	case !ok:
		return Unknown
	case stored == hash:
		return Trusted
	default:
		return Changed
	}
}

// Trust records configPath+hash as trusted and persists the store.
func (s *Store) Trust(configPath, hash string) error {
	s.entries[configPath] = hash
	return s.save()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	// Write-to-temp + rename so a crash mid-write cannot corrupt the store,
	// which would otherwise block every trusted run until manually deleted.
	// 0600: this is per-user security state, not shared config.
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".trust-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// HashBytes returns the hex-encoded SHA-256 of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashFile returns the hex-encoded SHA-256 of the file at path.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashBytes(data), nil
}

// DefaultStorePath returns the trust store location for a binary:
// ~/.config/<binaryName>/trust.json, alongside the global config.
func DefaultStorePath(binaryName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", binaryName, "trust.json"), nil
}
