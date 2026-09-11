package main

import (
	"errors"
	"os"
	"path/filepath"
)

func defaultLibraryRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", "Melodex Music")
	}
	return filepath.Join(home, "Music", "Melodex Music")
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		root = defaultLibraryRoot()
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func ensureDir(path string) error {
	if path == "" {
		return errors.New("empty path")
	}
	return os.MkdirAll(path, 0o755)
}

func ensureLibraryLayout(root string) (RootInfo, error) {
	root, err := resolveRoot(root)
	if err != nil {
		return RootInfo{}, err
	}
	appData := filepath.Join(root, ".melodex")
	incoming := filepath.Join(root, "Incoming")
	cache := filepath.Join(root, "Cache")
	if err := ensureDir(root); err != nil {
		return RootInfo{}, err
	}
	if err := ensureDir(appData); err != nil {
		return RootInfo{}, err
	}
	if err := ensureDir(incoming); err != nil {
		return RootInfo{}, err
	}
	if err := ensureDir(cache); err != nil {
		return RootInfo{}, err
	}
	return RootInfo{
		LibraryRoot: root,
		AppDataDir:  appData,
		IncomingDir: incoming,
		LibraryDir:  root,
		CacheDir:    cache,
	}, nil
}
