// Package store keeps JSON files under the gh-argus config directory,
// ~/.config/gh-argus on Linux and %AppData%\gh-argus on Windows.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Store struct {
	dir string
}

// New returns a store rooted at dir, which is created on first Save.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Open returns the store in the platform config directory.
func Open() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return New(filepath.Join(base, "gh-argus")), nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name+".json")
}

// Load decodes name into v. It reports false with no error when nothing has
// been saved under name yet.
func (s *Store) Load(name string, v any) (bool, error) {
	data, err := os.ReadFile(s.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("%s: %w", s.path(name), err)
	}
	return true, nil
}

// Save writes v under name. It writes a temp file and renames it into place
// so a crash cannot leave a half-written file.
func (s *Store) Save(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, name+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path(name))
}
