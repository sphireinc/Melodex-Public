package main

import (
	"os"
	"path/filepath"
	"testing"

	"melodex/internal/songstore"
)

func TestResolveTrackVideoPathPrefersDerivedAndMetadataCandidates(t *testing.T) {
	root := t.TempDir()

	t.Run("storage dir derived audio base", func(t *testing.T) {
		storageDir := filepath.Join(root, "Artist", "Album")
		if err := os.MkdirAll(storageDir, 0o755); err != nil {
			t.Fatalf("MkdirAll storage: %v", err)
		}
		derived := filepath.Join(storageDir, "01 - Song.mp4")
		fallback := filepath.Join(storageDir, "video.mp4")
		if err := os.WriteFile(derived, []byte("video"), 0o644); err != nil {
			t.Fatalf("WriteFile derived: %v", err)
		}
		if err := os.WriteFile(fallback, []byte("video"), 0o644); err != nil {
			t.Fatalf("WriteFile fallback: %v", err)
		}
		track := TrackRecord{
			StorageDir: storageDir,
			AudioPath:  filepath.Join(storageDir, "01 - Song.mp3"),
		}
		if got := resolveTrackVideoPath(track); got != derived {
			t.Fatalf("expected derived video path, got %q want %q", got, derived)
		}
	})

	t.Run("metadata dir fallback", func(t *testing.T) {
		metadataDir := filepath.Join(root, "Artist", "Album-2")
		if err := os.MkdirAll(metadataDir, 0o755); err != nil {
			t.Fatalf("MkdirAll metadata: %v", err)
		}
		videoPath := filepath.Join(metadataDir, "video.mp4")
		if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
			t.Fatalf("WriteFile video: %v", err)
		}
		track := TrackRecord{
			MetadataPath: filepath.Join(metadataDir, "01 - Song.metadata.json"),
		}
		if got := resolveTrackVideoPath(track); got != videoPath {
			t.Fatalf("expected metadata fallback video path, got %q want %q", got, videoPath)
		}
	})
}

func TestBuildTrackRecordCarriesVideoMetadata(t *testing.T) {
	root := t.TempDir()
	metadata := songstore.SongMetadata{
		TrackID: "track-1",
		Title:   "Song",
		Artist:  "Artist",
		Album:   "Album",
		Video: songstore.SongVideo{
			URL: "https://example.com/video.mp4",
		},
	}
	plan, err := songstore.BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(plan.MetadataPath), 0o755); err != nil {
		t.Fatalf("MkdirAll metadata dir: %v", err)
	}
	if err := os.WriteFile(plan.MetadataPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile metadata: %v", err)
	}
	source := filepath.Join(root, "source.mp3")
	if err := os.WriteFile(source, []byte("audio"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}
	record, err := buildTrackRecord(plan, metadata, "url", "https://example.com/watch?v=123", source)
	if err != nil {
		t.Fatalf("buildTrackRecord: %v", err)
	}
	if record.VideoPath != plan.VideoPath {
		t.Fatalf("expected record video path to match plan, got %q want %q", record.VideoPath, plan.VideoPath)
	}
	if record.VideoURL != metadata.Video.URL {
		t.Fatalf("expected record video URL to be preserved, got %q want %q", record.VideoURL, metadata.Video.URL)
	}
}
