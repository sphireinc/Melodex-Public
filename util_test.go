package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectLocalImportFilesRecursesFolders(t *testing.T) {
	root := t.TempDir()
	albumOne := filepath.Join(root, "Lacuna Coil", "Comalies")
	albumTwo := filepath.Join(root, "Lacuna Coil", "Karmacode")
	hidden := filepath.Join(root, "Lacuna Coil", ".melodex")

	if err := os.MkdirAll(albumOne, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(albumTwo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}

	first := filepath.Join(albumOne, "01 - Swamped.mp3")
	second := filepath.Join(albumTwo, "02 - Spellbound.flac")
	ignored := filepath.Join(hidden, "ignored.mp3")
	note := filepath.Join(albumTwo, "readme.txt")

	for _, path := range []string{first, second, ignored, note} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collectLocalImportFiles([]string{filepath.Join(root, "Lacuna Coil")})
	if err != nil {
		t.Fatalf("collectLocalImportFiles returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0].Path != first || files[1].Path != second {
		t.Fatalf("unexpected file order: %v", files)
	}
	if files[0].SourceRoot != filepath.Join(root, "Lacuna Coil") || files[1].SourceRoot != filepath.Join(root, "Lacuna Coil") {
		t.Fatalf("unexpected source roots: %#v", files)
	}
}

func TestDeriveLocalImportHintsFromArtistAlbumTree(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "Lacuna Coil", "Karmacode", "01 - Swamped.mp3")

	hints := deriveLocalImportHints(source, filepath.Join(root, "Lacuna Coil"))
	if !hints.Structured {
		t.Fatalf("expected structured hints")
	}
	if hints.Artist != "Lacuna Coil" {
		t.Fatalf("unexpected artist: %q", hints.Artist)
	}
	if hints.Album != "Karmacode" {
		t.Fatalf("unexpected album: %q", hints.Album)
	}
	if hints.Note == "" {
		t.Fatalf("expected a note describing the detected structure")
	}
}
