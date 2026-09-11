package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildLibraryManifestIsMetadataOnlyAndExcludesSecrets(t *testing.T) {
	root := t.TempDir()
	trackDir := filepath.Join(root, "Artist", "Album")
	metadataPath := filepath.Join(trackDir, "Song.metadata.json")
	audioPath := filepath.Join(trackDir, "Song.mp3")
	lyricsPath := filepath.Join(trackDir, "Song.txt")
	artworkPath := filepath.Join(trackDir, "cover.jpg")
	videoPath := filepath.Join(trackDir, "Song.mp4")

	track := TrackRecord{
		ID:                     "track-1",
		Artist:                 "Artist",
		Album:                  "Album",
		Title:                  "Song",
		Genre:                  "alternative",
		Year:                   "1999",
		MetadataPath:           metadataPath,
		StorageDir:             trackDir,
		AudioPath:              audioPath,
		LyricsPath:             lyricsPath,
		ArtworkPath:            artworkPath,
		VideoPath:              videoPath,
		ArtworkDataURL:         "data:image/jpeg;base64,MEDIA-BYTES-MUST-NOT-EXPORT",
		ArtworkMediaURL:        "http://127.0.0.1/media?token=media-secret",
		VideoURL:               "http://127.0.0.1/video?access_token=video-secret",
		ArtworkURL:             "https://art.example/cover.jpg?signature=art-secret&size=large",
		SourceRef:              "https://video.example/watch?id=abc&token=source-secret",
		Sources:                []string{"https://source.example/item?api_key=api-secret&format=json"},
		LyricsSourceLoc:        "/Users/example/private/lyrics.txt",
		MetadataNotes:          "Keep this note; secret=note-secret",
		LyricsIncluded:         true,
		HasTimedLyrics:         true,
		LyricsConfidence:       "high",
		TimedLyricsConfidence:  "medium",
		TimedLyricsGranularity: "line",
		AIProvider:             "openai-compatible",
		AIModel:                "model-a",
		AIRan:                  true,
	}

	manifest, err := BuildLibraryManifest(LibraryManifestExportSource{
		LibraryRoot: root,
		Tracks:      []TrackRecord{track},
		Settings: PublicSettings{
			Provider:                "openai-compatible",
			AIModel:                 "model-a",
			AIBaseURL:               "https://api.example/v1?api_key=settings-secret",
			UpdateManifestURL:       "https://updates.example/manifest?token=update-secret",
			YTDLPCookiesPath:        "/Users/example/.cookies.txt",
			YTDLPCookiesFromBrowser: "Chrome",
			DownloadMusicVideo:      true,
		},
	}, LibraryManifestExportOptions{ExportedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}

	if got, want := manifest.Tracks[0].Paths.Metadata.RelativePath, "Artist/Album/Song.metadata.json"; got != want {
		t.Fatalf("metadata path = %q, want %q", got, want)
	}
	if got, want := manifest.Tracks[0].Paths.Audio.RelativePath, "Artist/Album/Song.mp3"; got != want {
		t.Fatalf("audio reference = %q, want %q", got, want)
	}
	if manifest.Tracks[0].Lyrics.Path.RelativePath != "Artist/Album/Song.txt" || manifest.Tracks[0].Artwork.Path.RelativePath != "Artist/Album/cover.jpg" {
		t.Fatalf("expected portable lyrics/artwork references, got %+v", manifest.Tracks[0])
	}
	if manifest.Tracks[0].Provenance.SourceURL != "https://video.example/watch?id=abc" {
		t.Fatalf("source URL was not sanitized: %q", manifest.Tracks[0].Provenance.SourceURL)
	}
	if manifest.Settings.AIModel != "model-a" || !manifest.Settings.DownloadMusicVideo {
		t.Fatalf("safe settings were not retained: %+v", manifest.Settings)
	}
	if manifest.Settings.Provider == "" {
		t.Fatal("expected provider metadata to be retained")
	}

	data, err := MarshalLibraryManifest(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	serialized := string(data)
	for _, forbidden := range []string{
		"MEDIA-BYTES-MUST-NOT-EXPORT",
		"media-secret",
		"video-secret",
		"source-secret",
		"api-secret",
		"settings-secret",
		"update-secret",
		".cookies.txt",
		"art-secret",
		"note-secret",
		"data:image/jpeg",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("serialized manifest contains excluded value %q:\n%s", forbidden, serialized)
		}
	}
	if !manifest.Exclusions.APIKeysExcluded || !manifest.Exclusions.SecretsExcluded || !manifest.Exclusions.MediaBinariesExcluded || !manifest.Exclusions.LyricsTextExcluded || !manifest.Exclusions.ArtworkBytesExcluded {
		t.Fatalf("manifest exclusion contract is incomplete: %+v", manifest.Exclusions)
	}
	if manifest.ManifestChecksum != "" {
		t.Fatal("builder should not claim a checksum before serialization")
	}
}

func TestLibraryManifestRoundTripAndChecksum(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, 8, 8, 13, 14, 15, 0, time.UTC)
	manifest, err := BuildLibraryManifest(LibraryManifestExportSource{
		LibraryRoot:  root,
		Tracks:       []TrackRecord{{ID: "b", Title: "B", MetadataPath: filepath.Join(root, "b.json")}, {ID: "a", Title: "A", MetadataPath: filepath.Join(root, "a.json")}},
		Playlists:    []Playlist{{ID: "playlist-1", Name: "Favorites", Description: "A playlist", TrackIDs: []string{"a", "b"}}},
		Ratings:      map[string]int{"b": 3, "a": 5},
		Favorites:    []string{"b", "a", "a"},
		History:      []ImportHistoryEntry{{URL: "https://example.test/import?id=1&token=history-secret", CompletedAt: when}},
		ProfileNotes: map[string]string{"profile": "local note"},
		Settings:     PublicSettings{Provider: "openai-compatible", AIModel: "model", MaxConcurrentDownloads: 2},
	}, LibraryManifestExportOptions{ExportedAt: when})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	data, err := MarshalLibraryManifest(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	decoded, err := UnmarshalLibraryManifest(data)
	if err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if decoded.ManifestChecksum == "" {
		t.Fatal("expected checksum after round trip")
	}
	if got := decoded.Favorites; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("favorites = %#v", got)
	}
	if got := []string{decoded.Tracks[0].ID, decoded.Tracks[1].ID}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("tracks were not deterministically ordered: %#v", got)
	}
	if decoded.History[0].URL != "https://example.test/import?id=1" {
		t.Fatalf("history URL was not sanitized: %q", decoded.History[0].URL)
	}

	tampered := append([]byte(nil), data...)
	tampered = []byte(strings.Replace(string(tampered), `"A"`, `"Changed"`, 1))
	if _, err := UnmarshalLibraryManifest(tampered); !errors.Is(err, ErrLibraryManifestChecksumMismatch) {
		t.Fatalf("tampered manifest error = %v, want checksum mismatch", err)
	}

	path := filepath.Join(t.TempDir(), "backup", "library.manifest.json")
	if err := WriteLibraryManifestFile(path, manifest); err != nil {
		t.Fatalf("write manifest file: %v", err)
	}
	fromFile, err := ReadLibraryManifestFile(path)
	if err != nil {
		t.Fatalf("read manifest file: %v", err)
	}
	if fromFile.ManifestChecksum != decoded.ManifestChecksum || len(fromFile.Tracks) != 2 {
		t.Fatalf("file round trip mismatch: %+v", fromFile)
	}
}

func TestLibraryManifestPathRemappingRejectsTraversal(t *testing.T) {
	mapped, err := RemapLibraryManifestRelativePath("Artist/Album/song.mp3", []LibraryManifestPathMapping{
		{From: "Artist", To: "Imported/Artist"},
		{From: "Artist/Album", To: "Imported/Albums"},
	})
	if err != nil {
		t.Fatalf("remap path: %v", err)
	}
	if mapped != "Imported/Albums/song.mp3" {
		t.Fatalf("longest path mapping = %q", mapped)
	}
	resolved, err := ResolveLibraryManifestPath(LibraryManifestPathReference{RelativePath: mapped}, "/new/library", nil)
	if err != nil {
		t.Fatalf("resolve remapped path: %v", err)
	}
	if resolved != filepath.Join("/new/library", "Imported", "Albums", "song.mp3") {
		t.Fatalf("resolved path = %q", resolved)
	}
	if _, err := RemapLibraryManifestRelativePath("../outside.mp3", nil); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	if _, err := ResolveLibraryManifestPath(LibraryManifestPathReference{External: true, PathDigest: "digest"}, "/new/library", nil); err == nil {
		t.Fatal("expected external path reference to require review")
	}
}

func TestLibraryManifestImportPlanReportsConflictsAndHonorsDryRun(t *testing.T) {
	sourceRoot := t.TempDir()
	manifest, err := BuildLibraryManifest(LibraryManifestExportSource{
		LibraryRoot: sourceRoot,
		Tracks:      []TrackRecord{{ID: "track-1", Title: "Incoming", MetadataPath: filepath.Join(sourceRoot, "Artist", "incoming.json")}},
		Playlists:   []Playlist{{ID: "playlist-1", Name: "Incoming", TrackIDs: []string{"track-1"}}},
		Ratings:     map[string]int{"track-1": 5},
	}, LibraryManifestExportOptions{ExportedAt: time.Unix(100, 0).UTC()})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	target := LibraryManifestImportTarget{
		Tracks:    []LibraryManifestTrack{{ID: "track-1", Metadata: LibraryManifestTrackMetadata{Title: "Existing"}, Paths: LibraryManifestTrackPaths{Metadata: LibraryManifestPathReference{RelativePath: "Artist/incoming.json"}}}},
		Playlists: []LibraryManifestPlaylist{{ID: "playlist-1", Name: "Existing", TrackIDs: []string{"track-1"}}},
		Ratings:   []LibraryManifestRating{{TrackID: "track-1", Rating: 1}},
	}
	targetRoot := t.TempDir()
	plan, err := PlanLibraryManifestImport(manifest, target, LibraryManifestImportOptions{DryRun: true, TargetRoot: targetRoot})
	if err != nil {
		t.Fatalf("plan import: %v", err)
	}
	if !plan.DryRun || len(plan.Conflicts) < 3 {
		t.Fatalf("expected dry-run conflicts for track, playlist, and rating: %+v", plan)
	}
	if len(plan.PathResolutions) == 0 || plan.PathResolutions[0].ResolvedPath == "" {
		t.Fatalf("expected resolved path report: %+v", plan.PathResolutions)
	}
	unchanged, err := ApplyLibraryManifestImportPlan(plan, target)
	if err != nil {
		t.Fatalf("dry-run apply should not mutate or fail: %v", err)
	}
	if unchanged.Tracks[0].Metadata.Title != "Existing" || unchanged.Ratings[0].Rating != 1 {
		t.Fatalf("dry-run changed target: %+v", unchanged)
	}

	plan, err = PlanLibraryManifestImport(manifest, target, LibraryManifestImportOptions{TargetRoot: targetRoot, ConflictPolicy: LibraryManifestPreferIncoming})
	if err != nil {
		t.Fatalf("plan prefer incoming: %v", err)
	}
	updated, err := ApplyLibraryManifestImportPlan(plan, target)
	if err != nil {
		t.Fatalf("explicit prefer-incoming apply: %v", err)
	}
	if updated.Tracks[0].Metadata.Title != "Incoming" || updated.Ratings[0].Rating != 5 || updated.Playlists[0].Name != "Incoming" {
		t.Fatalf("explicit conflict policy was not applied: %+v", updated)
	}

	plan, err = PlanLibraryManifestImport(manifest, target, LibraryManifestImportOptions{TargetRoot: targetRoot})
	if err != nil {
		t.Fatalf("plan report-only: %v", err)
	}
	if _, err := ApplyLibraryManifestImportPlan(plan, target); !errors.Is(err, ErrLibraryManifestConflicts) {
		t.Fatalf("report-only apply error = %v, want unresolved conflicts", err)
	}
}

func TestLibraryManifestExternalPathIsReportedWithoutLeakingIt(t *testing.T) {
	manifest, err := BuildLibraryManifest(LibraryManifestExportSource{
		LibraryRoot: "/library",
		Tracks:      []TrackRecord{{ID: "external", MetadataPath: "/private/secret-location/metadata.json"}},
	}, LibraryManifestExportOptions{})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if !manifest.Tracks[0].Paths.Metadata.External || manifest.Tracks[0].Paths.Metadata.PathDigest == "" {
		t.Fatalf("expected external path digest: %+v", manifest.Tracks[0].Paths.Metadata)
	}
	data, err := MarshalLibraryManifest(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if strings.Contains(string(data), "/private/secret-location") {
		t.Fatal("manifest leaked external absolute path")
	}
	plan, err := PlanLibraryManifestImport(manifest, LibraryManifestImportTarget{}, LibraryManifestImportOptions{TargetRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("plan external path: %v", err)
	}
	if len(plan.Conflicts) == 0 || plan.Conflicts[0].Kind != "path_remap_required" {
		t.Fatalf("expected external path conflict: %+v", plan.Conflicts)
	}
}

func TestDecodeLibraryManifestReader(t *testing.T) {
	manifest, err := BuildLibraryManifest(LibraryManifestExportSource{}, LibraryManifestExportOptions{ExportedAt: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatalf("build empty manifest: %v", err)
	}
	data, err := MarshalLibraryManifest(manifest)
	if err != nil {
		t.Fatalf("marshal empty manifest: %v", err)
	}
	decoded, err := DecodeLibraryManifest(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("decode reader: %v", err)
	}
	if decoded.Format != LibraryManifestFormat || decoded.SchemaVersion != LibraryManifestSchemaVersion {
		t.Fatalf("decoded header = %+v", decoded)
	}
}
