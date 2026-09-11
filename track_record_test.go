package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"melodex/internal/songstore"
)

func TestBuildTrackRecordIncludesDurationSeconds(t *testing.T) {
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "track.mp3")
	metadataPath := filepath.Join(tempDir, "track.metadata.json")
	if err := os.WriteFile(audioPath, []byte("audio-bytes"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := os.WriteFile(metadataPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	duration := 225
	metadata := songstore.SongMetadata{
		TrackID:         "track-123",
		Title:           "The Clincher",
		Artist:          "Chevelle",
		Album:           "This Type of Thinking (Could Do Us In)",
		Genre:           "Alternative metal",
		DurationSeconds: &duration,
		Lyrics:          songstore.SongLyrics{HasLyrics: true},
		Audio:           songstore.SongAudio{Path: audioPath, Filename: "track.mp3", Format: "mp3"},
		AI: songstore.SongAI{
			GeneratedAt: time.Now().UTC(),
		},
		MetadataPath: metadataPath,
	}
	plan := songstore.SongStoragePlan{
		AlbumDir:     filepath.Join(tempDir, "Chevelle", "This Type of Thinking (Could Do Us In)"),
		MetadataPath: metadataPath,
		AudioPath:    audioPath,
	}

	record, err := buildTrackRecord(plan, metadata, "url", "https://www.youtube.com/watch?v=trinU3VD1Zo", audioPath)
	if err != nil {
		t.Fatalf("buildTrackRecord: %v", err)
	}
	if record.DurationSeconds == nil {
		t.Fatalf("expected duration seconds to be preserved")
	}
	if got := *record.DurationSeconds; got != duration {
		t.Fatalf("unexpected duration seconds: got %d want %d", got, duration)
	}
}
