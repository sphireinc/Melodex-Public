package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type atomicPersistenceWriterCase struct {
	name  string
	setup func(*testing.T, *Store) (string, func() error)
}

func TestAtomicPersistencePreservesPreviousFilesOnFailure(t *testing.T) {
	cases := []atomicPersistenceWriterCase{
		{
			name: "settings",
			setup: func(t *testing.T, store *Store) (string, func() error) {
				path := store.settingsPath
				writeAtomicPersistenceFixture(t, path, storedSettings{SchemaVersion: settingsSchemaVersion})
				return path, func() error {
					return store.saveSettings(storedSettings{SchemaVersion: settingsSchemaVersion, PublicSettings: PublicSettings{LibraryRoot: "new-root"}})
				}
			},
		},
		{
			name: "catalog",
			setup: func(t *testing.T, store *Store) (string, func() error) {
				path := store.catalogPath
				writeAtomicPersistenceFixture(t, path, catalogFile{Version: catalogSchemaVersion})
				return path, func() error {
					return store.saveCatalog(catalogFile{Version: catalogSchemaVersion, Jobs: []Job{{ID: "new-job"}}})
				}
			},
		},
		{
			name: "library-cache",
			setup: func(t *testing.T, store *Store) (string, func() error) {
				path := store.libraryCachePath
				writeAtomicPersistenceFixture(t, path, libraryCacheFile{
					Version: libraryCacheSchemaVersion,
					Catalog: catalogFile{Version: catalogSchemaVersion},
				})
				return path, func() error {
					return store.saveLibraryCache(catalogFile{Version: catalogSchemaVersion, Jobs: []Job{{ID: "new-job"}}})
				}
			},
		},
		{
			name: "playlists",
			setup: func(t *testing.T, store *Store) (string, func() error) {
				path := store.playlistsPath
				writeAtomicPersistenceFixture(t, path, playlistFile{Version: playlistSchemaVersion})
				return path, func() error {
					return store.savePlaylists(playlistFile{Version: playlistSchemaVersion, Playlists: []Playlist{{ID: "new-playlist"}}})
				}
			},
		},
		{
			name: "import-history",
			setup: func(t *testing.T, store *Store) (string, func() error) {
				path := store.importHistoryPath
				writeAtomicPersistenceFixture(t, path, importHistoryFile{Version: importHistorySchemaVersion})
				return path, func() error {
					return store.saveImportHistory(importHistoryFile{Version: importHistorySchemaVersion, Entries: []ImportHistoryEntry{{URL: "https://example.com/new"}}})
				}
			},
		},
		{
			name: "pipeline-cache",
			setup: func(t *testing.T, store *Store) (string, func() error) {
				path := store.pipelineCachePath
				writeAtomicPersistenceFixture(t, path, pipelineCacheFile{Version: pipelineCacheSchemaVersion})
				cache, err := loadPipelineCache(path)
				if err != nil {
					t.Fatalf("load pipeline cache: %v", err)
				}
				return path, cache.saveLocked
			},
		},
	}

	for _, stageCase := range []struct {
		name  string
		stage atomicWriteStage
	}{
		{name: "write", stage: atomicWriteStageWrite},
		{name: "rename", stage: atomicWriteStageRename},
	} {
		t.Run(stageCase.name, func(t *testing.T) {
			injected := errors.New("injected atomic " + stageCase.name + " failure")
			restore := installAtomicWriteFailureHook(func(stage atomicWriteStage, _ string) error {
				if stage == stageCase.stage {
					return injected
				}
				return nil
			})
			t.Cleanup(restore)

			for _, writerCase := range cases {
				t.Run(writerCase.name, func(t *testing.T) {
					store, _, err := newStore(t.TempDir())
					if err != nil {
						t.Fatalf("newStore: %v", err)
					}
					path, write := writerCase.setup(t, store)
					previous, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("read previous fixture: %v", err)
					}

					if err := write(); !errors.Is(err, injected) {
						t.Fatalf("expected injected %s failure, got %v", stageCase.name, err)
					}
					assertAtomicPersistenceFixtureUnchanged(t, path, previous)
				})
			}
		})
	}
}

func TestBuildBundlePreservesPreviousFileOnAtomicFailure(t *testing.T) {
	for _, stageCase := range []struct {
		name  string
		stage atomicWriteStage
	}{
		{name: "write", stage: atomicWriteStageWrite},
		{name: "rename", stage: atomicWriteStageRename},
	} {
		t.Run(stageCase.name, func(t *testing.T) {
			injected := errors.New("injected bundle " + stageCase.name + " failure")
			restore := installAtomicWriteFailureHook(func(stage atomicWriteStage, _ string) error {
				if stage == stageCase.stage {
					return injected
				}
				return nil
			})
			t.Cleanup(restore)

			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "source.mp3")
			bundlePath := filepath.Join(dir, "track.mldx")
			if err := os.WriteFile(sourcePath, []byte("new audio"), 0o600); err != nil {
				t.Fatalf("write source: %v", err)
			}
			previous := []byte("previous valid bundle")
			if err := os.WriteFile(bundlePath, previous, 0o600); err != nil {
				t.Fatalf("write previous bundle: %v", err)
			}

			if _, err := buildBundle(bundlePath, sourcePath, "audio.mp3", "metadata.json", "lyrics.txt", EnrichmentResult{
				Artist: "New Artist",
				Title:  "New Track",
			}, "local", "local-ref", "source.mp3", "new-id"); !errors.Is(err, injected) {
				t.Fatalf("expected injected %s failure, got %v", stageCase.name, err)
			}
			assertAtomicPersistenceFixtureUnchanged(t, bundlePath, previous)
		})
	}
}

func writeAtomicPersistenceFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func assertAtomicPersistenceFixtureUnchanged(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved file %s: %v", path, err)
	}
	if string(actual) != string(expected) {
		t.Fatalf("atomic failure changed %s: got %q, want %q", path, actual, expected)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatalf("find temporary files for %s: %v", path, err)
	}
	if len(matches) != 0 {
		t.Fatalf("atomic failure left temporary files for %s: %v", path, matches)
	}
}
