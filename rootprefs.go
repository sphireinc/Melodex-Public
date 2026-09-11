package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type rootPreferenceFile struct {
	LibraryRoot string `json:"libraryRoot"`
}

func rootPreferenceDir() string {
	cfg, err := os.UserConfigDir()
	if err != nil || cfg == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || home == "" {
			return filepath.Join(".", ".melodex")
		}
		return filepath.Join(home, ".melodex")
	}
	return filepath.Join(cfg, "Melodex")
}

func rootPreferencePath() string {
	return filepath.Join(rootPreferenceDir(), "root.json")
}

func loadRootPreference() (string, error) {
	data, err := os.ReadFile(rootPreferencePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var pref rootPreferenceFile
	if err := json.Unmarshal(data, &pref); err != nil {
		return "", err
	}
	return pref.LibraryRoot, nil
}

func saveRootPreference(root string) error {
	if root == "" {
		return errors.New("empty root")
	}
	if err := ensureDir(rootPreferenceDir()); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rootPreferenceFile{LibraryRoot: root}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(rootPreferencePath(), data, 0o600)
}
