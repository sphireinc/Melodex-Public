package main

import (
	"os"
	"path/filepath"
	"testing"

	"melodex/internal/songstore"
)

func TestLoadLocalImportExistingAssets(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "Song.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	metadataPath := localMetadataSidecarPath(audioPath)
	metadata := songstore.SongMetadata{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
		Audio: songstore.SongAudio{
			Filename: "Song.mp3",
			Format:   "mp3",
			Path:     audioPath,
		},
	}
	if err := songstore.WriteMetadataJSON(metadataPath, metadata); err != nil {
		t.Fatalf("write metadata fixture: %v", err)
	}
	if err := os.WriteFile(localPlainLyricsSidecarPath(audioPath), []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("write plain lyrics fixture: %v", err)
	}
	if err := os.WriteFile(localTimedLyricsSidecarPath(audioPath), []byte("[00:01.00]line one\n[00:02.00]line two\n"), 0o644); err != nil {
		t.Fatalf("write timed lyrics fixture: %v", err)
	}

	assets, err := loadLocalImportExistingAssets(audioPath)
	if err != nil {
		t.Fatalf("loadLocalImportExistingAssets returned error: %v", err)
	}
	if !assets.HasMetadata {
		t.Fatalf("expected metadata sidecar to be detected")
	}
	if !assets.HasPlainLyrics {
		t.Fatalf("expected plain lyrics sidecar to be detected")
	}
	if !assets.HasTimedLyrics {
		t.Fatalf("expected timed lyrics sidecar to be detected")
	}
	if got, want := assets.Metadata.Title, "Song"; got != want {
		t.Fatalf("loaded metadata title = %q, want %q", got, want)
	}
	if got, want := assets.PlainLyrics, "line one\nline two\n"; got != want {
		t.Fatalf("loaded plain lyrics = %q, want %q", got, want)
	}
	if got, want := len(assets.TimedLyrics), 2; got != want {
		t.Fatalf("loaded timed lyrics count = %d, want %d", got, want)
	}
}

func TestLoadLocalImportExistingAssetsDerivesPlainLyricsFromTimedOnly(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "Song.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}
	if err := os.WriteFile(localTimedLyricsSidecarPath(audioPath), []byte("[00:01.00]line one\n[00:02.00]line two\n"), 0o644); err != nil {
		t.Fatalf("write timed lyrics fixture: %v", err)
	}

	assets, err := loadLocalImportExistingAssets(audioPath)
	if err != nil {
		t.Fatalf("loadLocalImportExistingAssets returned error: %v", err)
	}
	if !assets.HasPlainLyrics {
		t.Fatalf("expected plain lyrics to be derived from the timed sidecar")
	}
	if !assets.HasTimedLyrics {
		t.Fatalf("expected timed lyrics sidecar to be detected")
	}
	if got, want := assets.PlainLyrics, "line one\nline two"; got != want {
		t.Fatalf("derived plain lyrics = %q, want %q", got, want)
	}
}

func TestMergeLocalLyricsResults(t *testing.T) {
	existing := songstore.LyricsResult{
		Text:       "existing plain",
		Source:     "local-sidecar",
		Notes:      "kept note",
		Confidence: "high",
	}
	fetched := songstore.LyricsResult{
		Text:        "fetched plain",
		TimedLyrics: []songstore.TimedLyricLine{{StartSeconds: 1.0, Text: "timed line"}},
		Source:      "lrclib",
		Notes:       "fetched note",
		Confidence:  "medium",
	}
	merged := mergeLocalLyricsResults(existing, fetched, false, true)
	if got, want := merged.Text, "existing plain"; got != want {
		t.Fatalf("merged text = %q, want %q", got, want)
	}
	if got, want := len(merged.TimedLyrics), 1; got != want {
		t.Fatalf("merged timed lyrics count = %d, want %d", got, want)
	}
	if got, want := merged.Source, "local-sidecar, lrclib"; got != want {
		t.Fatalf("merged source = %q, want %q", got, want)
	}
	if got, want := merged.IsComplete, true; got != want {
		t.Fatalf("merged completeness = %v, want %v", got, want)
	}
}
