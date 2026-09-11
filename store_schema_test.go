package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRejectsFutureSchemas(t *testing.T) {
	root := t.TempDir()
	store, _, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	writeJSON := func(path string, value any) {
		t.Helper()
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatalf("marshal %s: %v", path, marshalErr)
		}
		if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", path, writeErr)
		}
	}
	writeJSON(store.settingsPath, storedSettings{SchemaVersion: settingsSchemaVersion + 1})
	if _, err := store.loadSettings(); err == nil {
		t.Fatal("expected future settings schema to be rejected")
	}
	writeJSON(store.catalogPath, catalogFile{Version: catalogSchemaVersion + 1})
	if _, err := store.loadCatalog(); err == nil {
		t.Fatal("expected future catalog schema to be rejected")
	}
	writeJSON(store.playlistsPath, playlistFile{Version: playlistSchemaVersion + 1})
	if _, err := store.loadPlaylists(); err == nil {
		t.Fatal("expected future playlist schema to be rejected")
	}
	writeJSON(store.importHistoryPath, importHistoryFile{Version: importHistorySchemaVersion + 1})
	if _, err := store.loadImportHistory(); err == nil {
		t.Fatal("expected future import history schema to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, ".melodex")); err != nil {
		t.Fatalf("expected app data directory: %v", err)
	}
}
