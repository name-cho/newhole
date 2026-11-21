package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
)

const storageDirectoryName = ".newhole"
const legacyStorageDirectoryName = ".loophole"

func homeDirectory() (string, error) {
	home, err := homedir.Dir()
	if err != nil || home == "" {
		return "", fmt.Errorf("unable to determine home directory: %w", err)
	}
	return home, nil
}

func storageRoot() (string, error) {
	custom := os.Getenv("NEWHOLE_HOME")
	if custom == "" {
		custom = os.Getenv("LOOPHOLE_HOME")
	}
	if custom != "" {
		return custom, nil
	}
	home, err := homeDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, storageDirectoryName), nil
}

func migrateLegacyStorage(root string) {
	if _, err := os.Stat(root); err == nil {
		return
	}
	home, err := homeDirectory()
	if err != nil {
		return
	}
	legacy := filepath.Join(home, legacyStorageDirectoryName)
	if info, err := os.Stat(legacy); err != nil || !info.IsDir() {
		return
	}
	_ = os.MkdirAll(filepath.Dir(root), 0o700)
	if err := os.Rename(legacy, root); err == nil {
		return
	}
	if err := copyTree(legacy, root); err != nil {
		return
	}
}

func copyTree(source string, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return nil
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
}

func GetLocalStorageDir(directoryName string) string {
	root, err := storageRoot()
	if err != nil {
		panic(err)
	}
	migrateLegacyStorage(root)
	directory := filepath.Join(root, directoryName)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		panic(fmt.Errorf("unable to create storage directory %s: %w", directory, err))
	}
	return directory
}

func GetLocalStorageFile(fileName string, directoryName string) string {
	directory := GetLocalStorageDir(directoryName)
	return filepath.Join(directory, fileName)
}

func StorageRoot() (string, error) {
	root, err := storageRoot()
	if err != nil {
		return "", err
	}
	migrateLegacyStorage(root)
	return root, nil
}
