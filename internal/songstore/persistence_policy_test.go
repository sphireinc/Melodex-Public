package songstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadMetadataJSONRejectsUnknownAndFutureSchema(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "unknown field", payload: `{"version":1,"title":"Song","futureField":true}`, want: "futureField"},
		{name: "future version", payload: `{"version":2,"title":"Song"}`, want: "unsupported song metadata schema version"},
		{name: "negative version", payload: `{"version":-1,"title":"Song"}`, want: "invalid song metadata schema version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Song.metadata.json")
			if err := os.WriteFile(path, []byte(tc.payload), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			if _, err := ReadMetadataJSON(path); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestReadMetadataJSONMigratesLegacyVersionZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Song.metadata.json")
	if err := os.WriteFile(path, []byte(`{"title":"Legacy Song","artist":"Artist"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	metadata, err := ReadMetadataJSON(path)
	if err != nil {
		t.Fatalf("ReadMetadataJSON: %v", err)
	}
	if metadata.Version != songMetadataSchemaVersion {
		t.Fatalf("version = %d, want %d", metadata.Version, songMetadataSchemaVersion)
	}
}
