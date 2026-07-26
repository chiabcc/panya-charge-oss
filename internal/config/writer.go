package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WriteAtomic marshals cfg to YAML and writes it to path using an atomic
// rename strategy. The temp file is created in the same directory to ensure
// a same-filesystem rename (no cross-device move).
//
// If path already exists, its contents are copied to path+".bak" before
// overwriting. On any failure, the target file remains untouched.
func WriteAtomic(path string, cfg *Config) error {
	dir := filepath.Dir(path)

	existed := false
	if _, err := os.Stat(path); err == nil {
		existed = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target %s: %w", path, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}

	if existed {
		bakName := path + ".bak"

		data, err := os.ReadFile(path)
		if err != nil {
			cleanup()
			return fmt.Errorf("read existing config %s for backup: %w", path, err)
		}
		if err := os.WriteFile(bakName, data, 0644); err != nil {
			cleanup()
			return fmt.Errorf("write backup %s: %w", bakName, err)
		}
	}

	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp to %s: %w", path, err)
	}

	return nil
}