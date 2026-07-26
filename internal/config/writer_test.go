package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic_RoundTrip(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := defaultConfig()
	cfg.Charging.MinAmps = 8
	cfg.Charging.MaxAmps = 24

	if err := WriteAtomic(path, cfg); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Charging.MinAmps != 8 {
		t.Errorf("MinAmps = %d, want 8", loaded.Charging.MinAmps)
	}
	if loaded.Charging.MaxAmps != 24 {
		t.Errorf("MaxAmps = %d, want 24", loaded.Charging.MaxAmps)
	}
}

func TestWriteAtomic_BakCreation(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// First write — no .bak expected.
	cfg1 := defaultConfig()
	if err := WriteAtomic(path, cfg1); err != nil {
		t.Fatalf("WriteAtomic(1st): %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err == nil {
		t.Error("first write should not create .bak")
	}

	// Second write — .bak expected with first content.
	cfg2 := defaultConfig()
	cfg2.Charging.MinAmps = 10
	if err := WriteAtomic(path, cfg2); err != nil {
		t.Fatalf("WriteAtomic(2nd): %v", err)
	}

	bakData, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("read .bak: %v", err)
	}
	loaded, err := Load(filepath.Join(t.TempDir(), "bak.yaml"))
	if err != nil {
		t.Fatalf("Load from default: %v", err)
	}
	// .bak should contain original default MinAmps (6), not 10.
	if loaded.Charging.MinAmps != 6 {
		t.Logf("default MinAmps = %d", loaded.Charging.MinAmps)
	}
	want := []byte("min_amps: 6")
	got := string(bakData)
	if !containsSubset(bakData, want) {
		t.Errorf(".bak does not contain %q, got:\n%s", string(want), got)
	}
}

func TestWriteAtomic_NoTempFilesOnSuccess(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := defaultConfig()
	if err := WriteAtomic(path, cfg); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, ".config-*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) > 0 {
		t.Errorf("expected no temp files, got %d", len(matches))
	}
}

func TestWriteAtomic_ReadOnlyDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write initial config.
	cfg := defaultConfig()
	if err := WriteAtomic(path, cfg); err != nil {
		t.Fatalf("WriteAtomic(initial): %v", err)
	}
	originalData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Make directory read-only.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	// Attempt write — should fail, original untouched.
	cfg2 := defaultConfig()
	cfg2.Charging.MinAmps = 12
	err = WriteAtomic(path, cfg2)
	if err == nil {
		t.Error("WriteAtomic should fail on read-only directory")
	}

	gotData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after): %v", err)
	}
	if string(gotData) != string(originalData) {
		t.Error("target file was modified despite write failure")
	}
}

func TestWriteAtomic_NoBakOnNewFile(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "brand-new.yaml")

	cfg := defaultConfig()
	if err := WriteAtomic(path, cfg); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak should not exist for new file")
	}
}

func containsSubset(data, subset []byte) bool {
	for i := 0; i <= len(data)-len(subset); i++ {
		if string(data[i:i+len(subset)]) == string(subset) {
			return true
		}
	}
	return false
}