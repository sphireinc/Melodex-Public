package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFindURLImportDuplicateMatchesTrackSourceAndVideoURL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		catalog: catalogFile{
			Version: catalogSchemaVersion,
			Tracks: []TrackRecord{
				{
					ID:           "track-1",
					Title:        "The Red",
					Artist:       "Chevelle",
					Album:        "Wonder What's Next",
					SourceRef:    "https://www.youtube.com/watch?v=trinU3VD1Zo",
					VideoURL:     "https://www.youtube.com/watch?v=trinU3VD1Zo",
					MetadataPath: filepath.Join(info.LibraryDir, "Chevelle", "Wonder What's Next", "The Red.metadata.json"),
					CreatedAt:    time.Now().UTC(),
					SourceKind:   "url",
					SourceTitle:  "Chevelle - The Red (Official Audio)",
					GeneratedAt:  time.Now().UTC(),
				},
			},
		},
	}

	infoResult, err := app.FindURLImportDuplicate("https://www.youtube.com/watch?v=trinU3VD1Zo&list=RDjhC1pI76Rqo&index=4")
	if err != nil {
		t.Fatalf("FindURLImportDuplicate: %v", err)
	}
	if !infoResult.Exists {
		t.Fatalf("expected duplicate to be detected")
	}
	if infoResult.TrackID != "track-1" {
		t.Fatalf("expected duplicate track id track-1, got %q", infoResult.TrackID)
	}
	if infoResult.MetadataPath == "" {
		t.Fatalf("expected metadata path to be returned")
	}
	if infoResult.MatchReason == "" {
		t.Fatalf("expected match reason to be returned")
	}
}

func TestFindURLImportDuplicateDoesNotMatchDifferentVideoID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		catalog: catalogFile{
			Version: catalogSchemaVersion,
			Tracks: []TrackRecord{
				{
					ID:           "track-1",
					Title:        "Track One",
					Artist:       "Artist",
					Album:        "Album",
					SourceRef:    "https://www.youtube.com/watch?v=v2H4l9RpkwM",
					VideoURL:     "https://www.youtube.com/watch?v=v2H4l9RpkwM",
					MetadataPath: filepath.Join(info.LibraryDir, "Artist", "Album", "Track One.metadata.json"),
					CreatedAt:    time.Now().UTC(),
					SourceKind:   "url",
					GeneratedAt:  time.Now().UTC(),
				},
			},
		},
	}

	infoResult, err := app.FindURLImportDuplicate("https://www.youtube.com/watch?v=Gd9OhYroLN0")
	if err != nil {
		t.Fatalf("FindURLImportDuplicate: %v", err)
	}
	if infoResult.Exists {
		t.Fatalf("expected no duplicate for distinct video id, got %+v", infoResult)
	}
}
