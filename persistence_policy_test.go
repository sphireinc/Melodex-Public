package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthoritativePersistenceRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	store, _, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	cases := []struct {
		name string
		path string
		load func() error
		json string
	}{
		{name: "settings", path: store.settingsPath, load: func() error { _, err := store.loadSettings(); return err }, json: `{"schemaVersion":3,"futureField":true}`},
		{name: "catalog", path: store.catalogPath, load: func() error { _, err := store.loadCatalog(); return err }, json: `{"version":1,"futureField":true}`},
		{name: "playlists", path: store.playlistsPath, load: func() error { _, err := store.loadPlaylists(); return err }, json: `{"version":1,"playlists":[],"futureField":true}`},
		{name: "import history", path: store.importHistoryPath, load: func() error { _, err := store.loadImportHistory(); return err }, json: `{"version":1,"entries":[],"futureField":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(tc.path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			err := tc.load()
			if err == nil || !strings.Contains(err.Error(), "futureField") {
				t.Fatalf("expected actionable unknown-field error, got %v", err)
			}
		})
	}
}

func TestPersistedSchemaVersionPolicyRejectsInvalidAndFutureVersions(t *testing.T) {
	root := t.TempDir()
	store, _, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	cases := []struct {
		name string
		path string
		load func() error
		json string
	}{
		{name: "settings future", path: store.settingsPath, load: func() error { _, err := store.loadSettings(); return err }, json: `{"schemaVersion":4}`},
		{name: "catalog negative", path: store.catalogPath, load: func() error { _, err := store.loadCatalog(); return err }, json: `{"version":-1}`},
		{name: "playlist future", path: store.playlistsPath, load: func() error { _, err := store.loadPlaylists(); return err }, json: `{"version":2}`},
		{name: "history future", path: store.importHistoryPath, load: func() error { _, err := store.loadImportHistory(); return err }, json: `{"version":2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(tc.path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			if err := tc.load(); err == nil || !strings.Contains(err.Error(), "schema version") {
				t.Fatalf("expected actionable schema-version error, got %v", err)
			}
		})
	}
}

func TestDisposableCachePolicyIgnoresUnknownFields(t *testing.T) {
	root := t.TempDir()
	store, _, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	if err := os.WriteFile(store.libraryCachePath, []byte(`{"version":1,"catalog":{"version":1,"tracks":[],"jobs":[]},"futureField":true}`), 0o600); err != nil {
		t.Fatalf("write library cache fixture: %v", err)
	}
	if _, err := store.loadLibraryCache(); err != nil {
		t.Fatalf("library cache should tolerate additive fields: %v", err)
	}

	if err := os.WriteFile(store.pipelineCachePath, []byte(`{"version":2,"futureField":true}`), 0o600); err != nil {
		t.Fatalf("write pipeline cache fixture: %v", err)
	}
	cache, err := loadPipelineCache(store.pipelineCachePath)
	if err != nil {
		t.Fatalf("pipeline cache should tolerate additive fields: %v", err)
	}
	if cache.file.Version != pipelineCacheSchemaVersion || len(cache.file.Downloads) != 0 {
		t.Fatalf("unexpected normalized disposable cache: %+v", cache.file)
	}

	if err := os.WriteFile(store.libraryCachePath, []byte(`{"version":2,"catalog":{"version":1,"tracks":[],"jobs":[]}}`), 0o600); err != nil {
		t.Fatalf("write future library cache fixture: %v", err)
	}
	reset, err := store.loadLibraryCache()
	if err != nil {
		t.Fatalf("future library cache should be discarded: %v", err)
	}
	if reset.Version != catalogSchemaVersion || len(reset.Tracks) != 0 || len(reset.Jobs) != 0 {
		t.Fatalf("unexpected reset catalog from future cache: %+v", reset)
	}
}

func TestAuthoritativeDecoderRejectsTrailingJSON(t *testing.T) {
	var settings storedSettings
	err := decodeAuthoritativeJSON([]byte(`{"schemaVersion":3} {"second":true}`), &settings, "settings")
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("expected trailing JSON error, got %v", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal("unexpected filesystem error")
	}
}

func TestImportStageManifestRejectsUnknownFields(t *testing.T) {
	stageDir := t.TempDir()
	path := filepath.Join(stageDir, importStageManifestName)
	if err := os.WriteFile(path, []byte(`{"version":1,"jobId":"job-1","source":"source","stages":{},"futureField":true}`), 0o600); err != nil {
		t.Fatalf("write stage manifest fixture: %v", err)
	}
	if _, err := loadImportStageManifest(stageDir, "job-1", "source"); err == nil || !strings.Contains(err.Error(), "futureField") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestBundleManifestPortablePolicyAllowsAdditiveFieldsAndRejectsFutureVersion(t *testing.T) {
	for _, tc := range []struct {
		name        string
		payload     string
		wantErr     string
		wantVersion int
	}{
		{name: "legacy and additive", payload: `{"artist":"Artist","futureField":true}`, wantVersion: bundleManifestSchemaVersion},
		{name: "future version", payload: `{"version":2,"artist":"Artist"}`, wantErr: "unsupported bundle manifest schema version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "track.mldx")
			file, err := os.Create(path)
			if err != nil {
				t.Fatalf("create bundle: %v", err)
			}
			writer := zip.NewWriter(file)
			entry, err := writer.Create("manifest.json")
			if err == nil {
				_, err = fmt.Fprint(entry, tc.payload)
			}
			if err == nil {
				err = writer.Close()
			} else {
				_ = writer.Close()
			}
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				t.Fatalf("write bundle: %v", err)
			}

			opened, err := os.Open(path)
			if err != nil {
				t.Fatalf("open bundle: %v", err)
			}
			manifest, err := readManifest(opened)
			_ = opened.Close()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected %q, got manifest=%+v err=%v", tc.wantErr, manifest, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("read legacy/additive bundle: %v", err)
			}
			if manifest.Version != tc.wantVersion {
				t.Fatalf("version = %d, want %d", manifest.Version, tc.wantVersion)
			}
		})
	}
}
