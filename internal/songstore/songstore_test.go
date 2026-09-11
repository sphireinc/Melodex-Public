package songstore

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildSongStoragePlan(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	track := 1
	metadata := SongMetadata{
		Title:       "Song Title",
		Artist:      "Artist Name",
		Album:       "Album Name",
		TrackNumber: &track,
	}

	plan, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	if got, want := plan.ArtistDir, filepath.Join(absRoot, "Artist Name"); got != want {
		t.Fatalf("ArtistDir mismatch: got %q want %q", got, want)
	}
	if got, want := plan.AlbumDir, filepath.Join(absRoot, "Artist Name", "Album Name"); got != want {
		t.Fatalf("AlbumDir mismatch: got %q want %q", got, want)
	}
	if got, want := plan.BaseName, "01 - Song Title"; got != want {
		t.Fatalf("BaseName mismatch: got %q want %q", got, want)
	}
	if got, want := plan.AudioPath, filepath.Join(absRoot, "Artist Name", "Album Name", "01 - Song Title.mp3"); got != want {
		t.Fatalf("AudioPath mismatch: got %q want %q", got, want)
	}
	if got, want := plan.LyricsPath, filepath.Join(absRoot, "Artist Name", "Album Name", "01 - Song Title.txt"); got != want {
		t.Fatalf("LyricsPath mismatch: got %q want %q", got, want)
	}
	if got, want := plan.LRCPath, filepath.Join(absRoot, "Artist Name", "Album Name", "01 - Song Title.lrc"); got != want {
		t.Fatalf("LRCPath mismatch: got %q want %q", got, want)
	}
	if got, want := plan.MetadataPath, filepath.Join(absRoot, "Artist Name", "Album Name", "01 - Song Title.metadata.json"); got != want {
		t.Fatalf("MetadataPath mismatch: got %q want %q", got, want)
	}
}

func TestBuildSongStoragePlanUsesAudioFormat(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	metadata := SongMetadata{
		Title:  "Song Title",
		Artist: "Artist Name",
		Album:  "Album Name",
		Audio: SongAudio{
			Format: "flac",
		},
	}

	plan, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}

	if !strings.HasSuffix(plan.AudioPath, ".flac") {
		t.Fatalf("expected flac audio path, got %q", plan.AudioPath)
	}
}

func TestBuildSongStoragePlanFallbacksAndSanitization(t *testing.T) {
	t.Parallel()

	year := 2024
	track := 7
	cases := []struct {
		name          string
		metadata      SongMetadata
		wantArtistDir string
		wantAlbumDir  string
		wantBaseName  string
	}{
		{
			name: "missing artist",
			metadata: SongMetadata{
				Title:       "Hello",
				Album:       "Album",
				TrackNumber: &track,
			},
			wantArtistDir: "Unknown Artist",
			wantAlbumDir:  "Album",
			wantBaseName:  "07 - Hello",
		},
		{
			name: "missing album",
			metadata: SongMetadata{
				Title:       "Hello",
				Artist:      "Artist",
				TrackNumber: &track,
			},
			wantArtistDir: "Artist",
			wantAlbumDir:  "Unknown Album",
			wantBaseName:  "07 - Hello",
		},
		{
			name: "missing track number",
			metadata: SongMetadata{
				Title:  "Hello",
				Artist: "Artist",
				Album:  "Album",
				Year:   &year,
			},
			wantArtistDir: "Artist",
			wantAlbumDir:  "Album",
			wantBaseName:  "Hello",
		},
		{
			name: "unsafe characters",
			metadata: SongMetadata{
				Title:       `Song <>:"/\|?* Name.`,
				Artist:      `AUX`,
				Album:       `CON.`,
				TrackNumber: &track,
			},
			wantArtistDir: "AUX_",
			wantAlbumDir:  "CON_",
			wantBaseName:  "07 - Song - Name",
		},
	}

	root := t.TempDir()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan, err := BuildSongStoragePlan(root, tc.metadata)
			if err != nil {
				t.Fatalf("BuildSongStoragePlan returned error: %v", err)
			}

			if base := filepath.Base(plan.ArtistDir); base != tc.wantArtistDir {
				t.Fatalf("artist dir base mismatch: got %q want %q", base, tc.wantArtistDir)
			}
			if base := filepath.Base(plan.AlbumDir); base != tc.wantAlbumDir {
				t.Fatalf("album dir base mismatch: got %q want %q", base, tc.wantAlbumDir)
			}
			if plan.BaseName != tc.wantBaseName {
				t.Fatalf("base name mismatch: got %q want %q", plan.BaseName, tc.wantBaseName)
			}
			if strings.ContainsAny(filepath.Base(plan.ArtistDir), `<>:"/\|?*`) {
				t.Fatalf("artist dir contains unsafe characters: %q", filepath.Base(plan.ArtistDir))
			}
			if strings.ContainsAny(filepath.Base(plan.AlbumDir), `<>:"/\|?*`) {
				t.Fatalf("album dir contains unsafe characters: %q", filepath.Base(plan.AlbumDir))
			}
			if strings.ContainsAny(plan.BaseName, `<>:"/\|?*`) {
				t.Fatalf("base name contains unsafe characters: %q", plan.BaseName)
			}
			if strings.HasSuffix(filepath.Base(plan.ArtistDir), ".") || strings.HasSuffix(filepath.Base(plan.ArtistDir), " ") {
				t.Fatalf("artist dir has unsafe suffix: %q", filepath.Base(plan.ArtistDir))
			}
			if strings.HasSuffix(filepath.Base(plan.AlbumDir), ".") || strings.HasSuffix(filepath.Base(plan.AlbumDir), " ") {
				t.Fatalf("album dir has unsafe suffix: %q", filepath.Base(plan.AlbumDir))
			}
			if strings.HasSuffix(plan.BaseName, ".") || strings.HasSuffix(plan.BaseName, " ") {
				t.Fatalf("base name has unsafe suffix: %q", plan.BaseName)
			}
		})
	}
}

func TestBuildSongStoragePlanCollisionHandling(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	metadata := SongMetadata{
		Title:  "Collision Song",
		Artist: "Artist",
		Album:  "Album",
	}

	first, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(first.MetadataPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(first.MetadataPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	second, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}

	if got, want := second.BaseName, "Collision Song (2)"; got != want {
		t.Fatalf("collision suffix mismatch: got %q want %q", got, want)
	}
	if !strings.HasSuffix(second.AudioPath, "Collision Song (2).mp3") {
		t.Fatalf("audio path did not use shared suffix: %q", second.AudioPath)
	}
	if !strings.HasSuffix(second.LyricsPath, "Collision Song (2).txt") {
		t.Fatalf("lyrics path did not use shared suffix: %q", second.LyricsPath)
	}
	if !strings.HasSuffix(second.LRCPath, "Collision Song (2).lrc") {
		t.Fatalf("lrc path did not use shared suffix: %q", second.LRCPath)
	}
	if !strings.HasSuffix(second.MetadataPath, "Collision Song (2).metadata.json") {
		t.Fatalf("metadata path did not use shared suffix: %q", second.MetadataPath)
	}
}

func TestWriteSongFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	track := 3
	year := 2025
	duration := 210
	bitrate := 320
	sampleRate := 44100
	channels := 2
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	metadata := SongMetadata{
		Title:                "Song Title",
		Artist:               "Artist Name",
		Album:                "Album Name",
		TrackNumber:          &track,
		Year:                 &year,
		Genre:                "pop",
		DurationSeconds:      &duration,
		Confidence:           "high",
		MetadataConfidence:   "high",
		EnrichmentConfidence: "medium",
		ReleaseType:          "album",
		ISRC:                 "US-ABC-25-00001",
		ArtistLinks: MetadataLinks{
			OfficialWebsite: "https://artist.example",
			Spotify:         "https://open.spotify.com/artist/example",
		},
		AlbumLinks: MetadataLinks{
			Spotify:     "https://open.spotify.com/album/example",
			Wikipedia:   "https://wikipedia.org/wiki/Album",
			MusicBrainz: "https://musicbrainz.org/release/example",
		},
		SongLinks: MetadataLinks{
			Spotify:      "https://open.spotify.com/track/example",
			YouTube:      "https://youtube.com/watch?v=example",
			Genius:       "https://genius.com/example",
			Wikipedia:    "https://wikipedia.org/wiki/Song",
			YouTubeMusic: "https://music.youtube.com/watch?v=example",
		},
		ArtistTrivia:  []string{"Artist formed in 1995."},
		AlbumTrivia:   []string{"Album reached platinum status."},
		SongTrivia:    []string{"Song was released as a single."},
		SongMeaning:   "A concise summary of the song's theme.",
		Tidbits:       []string{"Recorded at a well-known studio."},
		Sources:       []string{"https://musicbrainz.org"},
		MetadataNotes: "verified",
		Source: SongSource{
			URL:          "https://youtube.com/watch?v=abc123",
			Provider:     "youtube",
			VideoID:      "abc123",
			VideoTitle:   "Song Title (Official Video)",
			Channel:      "Artist Name",
			DownloadedAt: now,
		},
		Audio: SongAudio{
			Format:       "mp3",
			Codec:        "mp3",
			SourceFormat: "mp3",
			Bitrate:      &bitrate,
			SampleRate:   &sampleRate,
			Channels:     &channels,
		},
		AI: SongAI{
			Provider:    "openai",
			Model:       "gpt-4.1-mini",
			GeneratedAt: now,
			Context: SongAIContext{
				FileName:        "Song Title.mp3",
				SourceURL:       "https://youtube.com/watch?v=abc123",
				LibraryRoot:     root,
				TargetAudioPath: filepath.Join(root, "Artist Name", "Album Name", "Song Title.mp3"),
			},
		},
	}

	plan, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}

	audioSource := filepath.Join(root, "source.mp3")
	if err := os.WriteFile(audioSource, []byte("mp3-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile audio source: %v", err)
	}

	lyrics := LyricsResult{
		Text:              "Line one\nLine two",
		TimingGranularity: "line",
		Confidence:        "high",
		Source:            "provided_input",
		SourceLoc:         "memory",
		IsComplete:        true,
		Notes:             "verified",
		TimedLyrics: []TimedLyricLine{
			{StartSeconds: 1.23, Text: "Line one"},
			{StartSeconds: 4.56, Text: "Line two"},
		},
	}

	if err := WriteSongFiles(plan, audioSource, "", lyrics, metadata); err != nil {
		t.Fatalf("WriteSongFiles returned error: %v", err)
	}

	assertFileContents(t, plan.LyricsPath, "Line one\nLine two")
	assertFileContents(t, plan.LRCPath, "[00:01.23]Line one\n[00:04.56]Line two\n")
	assertFileExists(t, plan.AudioPath)
	assertFileExists(t, plan.MetadataPath)

	stored, err := ReadMetadataJSON(plan.MetadataPath)
	if err != nil {
		t.Fatalf("ReadMetadataJSON returned error: %v", err)
	}

	if stored.StorageDir != plan.AlbumDir {
		t.Fatalf("StorageDir mismatch: got %q want %q", stored.StorageDir, plan.AlbumDir)
	}
	if stored.MetadataPath != plan.MetadataPath {
		t.Fatalf("MetadataPath mismatch: got %q want %q", stored.MetadataPath, plan.MetadataPath)
	}
	if stored.Audio.Path != plan.AudioPath {
		t.Fatalf("Audio.Path mismatch: got %q want %q", stored.Audio.Path, plan.AudioPath)
	}
	if stored.Audio.SourceFormat != "mp3" {
		t.Fatalf("Audio.SourceFormat mismatch: got %q want %q", stored.Audio.SourceFormat, "mp3")
	}
	if stored.Audio.Codec != "mp3" {
		t.Fatalf("Audio.Codec mismatch: got %q want %q", stored.Audio.Codec, "mp3")
	}
	if stored.Lyrics.Path != plan.LyricsPath {
		t.Fatalf("Lyrics.Path mismatch: got %q want %q", stored.Lyrics.Path, plan.LyricsPath)
	}
	if stored.TimedLyrics.Path != plan.LRCPath {
		t.Fatalf("TimedLyrics.Path mismatch: got %q want %q", stored.TimedLyrics.Path, plan.LRCPath)
	}
	if !stored.Lyrics.HasLyrics {
		t.Fatalf("expected lyrics flag to be true")
	}
	if !stored.TimedLyrics.HasTimedLyrics {
		t.Fatalf("expected timed lyrics flag to be true")
	}
	if stored.MetadataConfidence != "high" {
		t.Fatalf("MetadataConfidence mismatch: got %q", stored.MetadataConfidence)
	}
	if stored.Confidence != "high" {
		t.Fatalf("Confidence mismatch: got %q", stored.Confidence)
	}
	if stored.EnrichmentConfidence != "medium" {
		t.Fatalf("EnrichmentConfidence mismatch: got %q", stored.EnrichmentConfidence)
	}
	if stored.ReleaseType != "album" {
		t.Fatalf("ReleaseType mismatch: got %q", stored.ReleaseType)
	}
	if stored.ISRC != "US-ABC-25-00001" {
		t.Fatalf("ISRC mismatch: got %q", stored.ISRC)
	}
	if stored.ArtistLinks.OfficialWebsite != "https://artist.example" {
		t.Fatalf("ArtistLinks.OfficialWebsite mismatch: got %q", stored.ArtistLinks.OfficialWebsite)
	}
	if stored.AlbumLinks.MusicBrainz != "https://musicbrainz.org/release/example" {
		t.Fatalf("AlbumLinks.MusicBrainz mismatch: got %q", stored.AlbumLinks.MusicBrainz)
	}
	if stored.SongLinks.Genius != "https://genius.com/example" {
		t.Fatalf("SongLinks.Genius mismatch: got %q", stored.SongLinks.Genius)
	}
	if len(stored.ArtistTrivia) != 1 || stored.ArtistTrivia[0] != "Artist formed in 1995." {
		t.Fatalf("ArtistTrivia mismatch: %#v", stored.ArtistTrivia)
	}
	if len(stored.Tidbits) != 1 || stored.Tidbits[0] != "Recorded at a well-known studio." {
		t.Fatalf("Tidbits mismatch: %#v", stored.Tidbits)
	}
	if len(stored.Sources) != 1 || stored.Sources[0] != "https://musicbrainz.org" {
		t.Fatalf("Sources mismatch: %#v", stored.Sources)
	}
	if stored.MetadataNotes != "verified" {
		t.Fatalf("MetadataNotes mismatch: got %q", stored.MetadataNotes)
	}
	if stored.Lyrics.SourceLoc != "memory" {
		t.Fatalf("Lyrics.SourceLoc mismatch: got %q want %q", stored.Lyrics.SourceLoc, "memory")
	}
	if stored.Audio.Bitrate == nil || *stored.Audio.Bitrate != bitrate {
		t.Fatalf("Audio.Bitrate mismatch: got %#v want %d", stored.Audio.Bitrate, bitrate)
	}
	if stored.Audio.SampleRate == nil || *stored.Audio.SampleRate != sampleRate {
		t.Fatalf("Audio.SampleRate mismatch: got %#v want %d", stored.Audio.SampleRate, sampleRate)
	}
	if stored.Audio.Channels == nil || *stored.Audio.Channels != channels {
		t.Fatalf("Audio.Channels mismatch: got %#v want %d", stored.Audio.Channels, channels)
	}
	if stored.AI.Context.SourceURL != "https://youtube.com/watch?v=abc123" {
		t.Fatalf("AI.Context.SourceURL mismatch: got %q", stored.AI.Context.SourceURL)
	}
}

func TestWriteSongFilesEmptySidecars(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	metadata := SongMetadata{
		Title:  "Untitled",
		Artist: "Artist",
		Album:  "Album",
	}

	plan, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}

	audioSource := filepath.Join(root, "source.mp3")
	if err := os.WriteFile(audioSource, []byte("audio"), 0o644); err != nil {
		t.Fatalf("WriteFile audio source: %v", err)
	}

	lyrics := LyricsResult{
		Confidence:        "low",
		Source:            "none",
		TimingGranularity: "none",
		Notes:             "Lyrics were unavailable.",
	}

	if err := WriteSongFiles(plan, audioSource, "", lyrics, metadata); err != nil {
		t.Fatalf("WriteSongFiles returned error: %v", err)
	}

	assertFileContents(t, plan.LyricsPath, "")
	assertFileContents(t, plan.LRCPath, "")

	stored, err := ReadMetadataJSON(plan.MetadataPath)
	if err != nil {
		t.Fatalf("ReadMetadataJSON returned error: %v", err)
	}
	if stored.Lyrics.HasLyrics {
		t.Fatalf("expected HasLyrics to be false")
	}
	if stored.TimedLyrics.HasTimedLyrics {
		t.Fatalf("expected HasTimedLyrics to be false")
	}
}

func TestEnsureAIStatusBackfillsLegacyMetadata(t *testing.T) {
	t.Parallel()

	metadata := SongMetadata{
		MetadataConfidence: "medium",
		MetadataNotes:      "Artist inferred from source title; album, year, genre, and track number not provided; duration from source metadata.",
		AI: SongAI{
			Provider:    "openai-compatible",
			Model:       "gpt-4.1-mini",
			GeneratedAt: time.Now().UTC(),
		},
	}

	normalized, changed := EnsureAIStatus(metadata)
	if !changed {
		t.Fatalf("expected status backfill to report change")
	}
	if normalized.AI.Status != "partial" {
		t.Fatalf("unexpected AI status: %q", normalized.AI.Status)
	}
	if normalized.AI.Message != "Metadata generated; lyrics unavailable." {
		t.Fatalf("unexpected AI message: %q", normalized.AI.Message)
	}
}

func TestWriteSongFilesIgnoresBlockTimingForLRC(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	metadata := SongMetadata{
		Title:  "Block Timing",
		Artist: "Artist",
		Album:  "Album",
	}

	plan, err := BuildSongStoragePlan(root, metadata)
	if err != nil {
		t.Fatalf("BuildSongStoragePlan returned error: %v", err)
	}

	audioSource := filepath.Join(root, "source.mp3")
	if err := os.WriteFile(audioSource, []byte("audio"), 0o644); err != nil {
		t.Fatalf("WriteFile audio source: %v", err)
	}

	lyrics := LyricsResult{
		Text:              "Block lyric",
		TimingGranularity: "block",
		Confidence:        "medium",
		Source:            "provided_input",
		IsComplete:        true,
		TimedLyrics: []TimedLyricLine{
			{StartSeconds: 12.34, Text: "Block lyric"},
		},
	}

	if err := WriteSongFiles(plan, audioSource, "", lyrics, metadata); err != nil {
		t.Fatalf("WriteSongFiles returned error: %v", err)
	}

	assertFileContents(t, plan.LRCPath, "")
}

func TestWriteSongFilesFailureBoundariesLeaveNoPartialArtifacts(t *testing.T) {
	cases := []struct {
		name       string
		targetPath func(SongStoragePlan, string) string
	}{
		{name: "audio", targetPath: func(plan SongStoragePlan, _ string) string { return plan.AudioPath }},
		{name: "original-audio", targetPath: func(_ SongStoragePlan, originalPath string) string { return originalPath }},
		{name: "plain-lyrics", targetPath: func(plan SongStoragePlan, _ string) string { return plan.LyricsPath }},
		{name: "timed-lyrics", targetPath: func(plan SongStoragePlan, _ string) string { return plan.LRCPath }},
		{name: "metadata", targetPath: func(plan SongStoragePlan, _ string) string { return plan.MetadataPath }},
	}

	for _, failureStage := range []struct {
		name  string
		stage atomicWriteStage
	}{
		{name: "write", stage: atomicWriteStageWrite},
		{name: "rename", stage: atomicWriteStageRename},
	} {
		for _, testCase := range cases {
			t.Run(failureStage.name+"/"+testCase.name, func(t *testing.T) {
				root := t.TempDir()
				metadata := SongMetadata{
					Title:  "Failure Boundary",
					Artist: "Artist",
					Album:  "Album",
					Audio: SongAudio{
						Format:           "mp3",
						SourceFormat:     "flac",
						OriginalFormat:   "flac",
						OriginalFilename: "Failure Boundary.source.flac",
					},
					AI: SongAI{
						Provider:    "test",
						Model:       "failure-boundary",
						GeneratedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
					},
				}
				plan, err := BuildSongStoragePlan(root, metadata)
				if err != nil {
					t.Fatalf("BuildSongStoragePlan returned error: %v", err)
				}
				originalPath := filepath.Join(plan.AlbumDir, plan.BaseName+".source.flac")
				metadata.Audio.OriginalPath = originalPath

				audioSource := filepath.Join(root, "source.mp3")
				originalSource := filepath.Join(root, "source.flac")
				if err := os.WriteFile(audioSource, []byte("normalized audio"), 0o644); err != nil {
					t.Fatalf("write audio source: %v", err)
				}
				if err := os.WriteFile(originalSource, []byte("original audio"), 0o644); err != nil {
					t.Fatalf("write original source: %v", err)
				}

				outputs := []string{plan.AudioPath, originalPath, plan.LyricsPath, plan.LRCPath, plan.MetadataPath}
				previous := []byte("previous durable artifact")
				for _, output := range outputs {
					if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
						t.Fatalf("create output directory: %v", err)
					}
					if err := os.WriteFile(output, previous, 0o644); err != nil {
						t.Fatalf("write previous output %s: %v", output, err)
					}
				}
				target := testCase.targetPath(plan, originalPath)
				restore := installAtomicWriteFailureHook(func(stage atomicWriteStage, path string) error {
					if stage == failureStage.stage && filepath.Clean(path) == filepath.Clean(target) {
						return errors.New("injected finalization " + failureStage.name + " failure")
					}
					return nil
				})
				t.Cleanup(restore)

				lyrics := LyricsResult{
					Text:              "line one",
					TimingGranularity: "line",
					TimedLyrics: []TimedLyricLine{
						{StartSeconds: 1.0, Text: "line one"},
					},
				}
				err = WriteSongFiles(plan, audioSource, originalSource, lyrics, metadata)
				if err == nil || !strings.Contains(err.Error(), "injected finalization") {
					t.Fatalf("expected injected %s failure, got %v", failureStage.name, err)
				}

				for _, output := range outputs {
					assertFileContents(t, output, string(previous))
				}
				for _, pattern := range []string{".*.tmp-*", ".*.backup-*"} {
					matches, err := filepath.Glob(filepath.Join(plan.AlbumDir, pattern))
					if err != nil {
						t.Fatalf("find finalization temporary files: %v", err)
					}
					if len(matches) != 0 {
						t.Fatalf("finalization failure left %s files: %v", pattern, matches)
					}
				}
			})
		}
	}
}

func TestLoadSongMetadataLegacyBundle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bundlePath := filepath.Join(root, "legacy.mldx")

	file, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("Create bundle: %v", err)
	}
	zw := zip.NewWriter(file)

	manifest := map[string]any{
		"artist":        "Legacy Artist",
		"album":         "Legacy Album",
		"title":         "Legacy Title",
		"genre":         "indie",
		"year":          "2020",
		"lyrics":        "legacy lyrics",
		"sourceKind":    "youtube",
		"sourceRef":     "https://youtube.com/watch?v=legacy",
		"originalName":  "Legacy Source",
		"audioEntry":    "Legacy.mp3",
		"metadataEntry": "metadata.json",
		"lyricsEntry":   "lyrics.txt",
		"hash":          "abc123",
		"createdAt":     time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	writer, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := writer.Write(manifestBytes); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close bundle file: %v", err)
	}

	metadata, err := LoadSongMetadata(bundlePath)
	if err != nil {
		t.Fatalf("LoadSongMetadata returned error: %v", err)
	}

	if metadata.Title != "Legacy Title" || metadata.Artist != "Legacy Artist" || metadata.Album != "Legacy Album" {
		t.Fatalf("legacy metadata mismatch: %#v", metadata)
	}
	if metadata.Source.Provider != "youtube" {
		t.Fatalf("legacy source provider mismatch: %q", metadata.Source.Provider)
	}
	if metadata.MetadataPath != bundlePath {
		t.Fatalf("MetadataPath mismatch: got %q want %q", metadata.MetadataPath, bundlePath)
	}
	if metadata.StorageDir != root {
		t.Fatalf("StorageDir mismatch: got %q want %q", metadata.StorageDir, root)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist at %q: %v", path, err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("contents mismatch for %q: got %q want %q", path, string(got), want)
	}
}
