package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetLocalStorageDirReturnsCorrectPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEWHOLE_HOME", root)
	directory := GetLocalStorageDir("expected-dirname")
	expected := filepath.Join(root, "expected-dirname")
	if directory != expected {
		t.Fatalf("created directory %q differs from expected %q", directory, expected)
	}
}

func TestGetLocalStorageDirCreatesDirectory(t *testing.T) {
	t.Setenv("NEWHOLE_HOME", t.TempDir())
	created := GetLocalStorageDir("expected-dirname")
	info, err := os.Stat(created)
	if err != nil {
		t.Fatalf("directory %q does not exist: %v", created, err)
	}
	if !info.IsDir() {
		t.Fatalf("path %q is not a directory", created)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions %v differ from 0700", info.Mode().Perm())
	}
}

func TestGetLocalStorageFileReturnsCorrectPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEWHOLE_HOME", root)
	filePath := GetLocalStorageFile("expected-filename", "")
	expected := filepath.Join(root, "expected-filename")
	if filePath != expected {
		t.Fatalf("file path %q differs from expected %q", filePath, expected)
	}
}

func TestGetLocalStorageFileDoesNotCreateFile(t *testing.T) {
	t.Setenv("NEWHOLE_HOME", t.TempDir())
	filePath := GetLocalStorageFile("expected-filename", "")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file %q not to exist", filePath)
	}
}

func TestLegacyStorageIsMigrated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NEWHOLE_HOME", "")
	legacy := filepath.Join(home, legacyStorageDirectoryName)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "tokens.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrated := GetLocalStorageFile("tokens.json", "")
	if _, err := os.Stat(migrated); err != nil {
		t.Fatalf("tokens were not migrated: %v", err)
	}
}
