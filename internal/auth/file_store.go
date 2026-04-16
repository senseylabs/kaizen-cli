//go:build !darwin

package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type fileStore struct {
	path    string
	homeErr error
}

// NewCredentialStore returns a file-based credential store for non-macOS systems.
func NewCredentialStore() CredentialStore {
	home, err := os.UserHomeDir()
	if err != nil {
		return &fileStore{
			path:    "",
			homeErr: fmt.Errorf("failed to determine home directory: %w", err),
		}
	}
	return &fileStore{
		path: filepath.Join(home, ".kaizen", "credentials"),
	}
}

func (f *fileStore) Save(creds Credentials) error {
	if f.homeErr != nil {
		return f.homeErr
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize credentials: %w", err)
	}

	if err := os.WriteFile(f.path, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}
	return nil
}

func (f *fileStore) Load() (Credentials, error) {
	if f.homeErr != nil {
		return Credentials{}, f.homeErr
	}
	data, err := os.ReadFile(f.path)
	if err != nil {
		return Credentials{}, fmt.Errorf("no credentials found at %s: %w", f.path, err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("failed to parse credentials: %w", err)
	}
	return creds, nil
}

func (f *fileStore) Delete() error {
	if f.homeErr != nil {
		return f.homeErr
	}
	if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete credentials: %w", err)
	}
	return nil
}
