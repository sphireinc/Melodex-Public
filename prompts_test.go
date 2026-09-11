package main

import (
	"strings"
	"testing"
)

func TestLoadPromptLibraryUsesUpdatedMetadataPrompts(t *testing.T) {
	t.Parallel()

	library, err := loadPromptLibrary()
	if err != nil {
		t.Fatalf("loadPromptLibrary: %v", err)
	}

	system, user, err := library.MetadataPrompt(map[string]string{
		"FileName":        "Song Title.mp3",
		"SourceKind":      "youtube",
		"SourceRef":       "https://youtube.com/watch?v=abc123",
		"SourceURL":       "https://youtube.com/watch?v=abc123",
		"LibraryRoot":     t.TempDir(),
		"TargetArtistDir": "Artist",
		"TargetAlbumDir":  "Album",
		"TargetBaseName":  "Song Title",
	})
	if err != nil {
		t.Fatalf("MetadataPrompt: %v", err)
	}

	if !strings.Contains(system, `"artist_links"`) || !strings.Contains(system, `"song_meaning"`) || !strings.Contains(system, `"enrichment_confidence"`) {
		t.Fatalf("expected updated metadata system prompt shape, got: %s", system)
	}
	if !strings.Contains(user, "Artist directory: Artist") || !strings.Contains(user, "Metadata path:") {
		t.Fatalf("expected updated metadata user prompt content, got: %s", user)
	}

	infos := library.Info()
	var seenUpdatedSystem, seenUpdatedUser, seenLegacySystem, seenLegacyUser bool
	for _, info := range infos {
		switch info.Name {
		case "metadata_system_updated.md":
			seenUpdatedSystem = true
		case "metadata_user_updated.md":
			seenUpdatedUser = true
		case "metadata_system_legacy.txt":
			seenLegacySystem = true
		case "metadata_user_legacy.txt":
			seenLegacyUser = true
		}
	}
	if !seenUpdatedSystem || !seenUpdatedUser || !seenLegacySystem || !seenLegacyUser {
		t.Fatalf("expected both updated and legacy prompt files to be embedded, got %+v", infos)
	}
}
