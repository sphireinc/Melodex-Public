package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"melodex/internal/songstore"
)

func TestPipelineCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "song.mp3")
	videoPath := filepath.Join(dir, "video.mp4")
	artworkPath := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(artworkPath, []byte("art"), 0o644); err != nil {
		t.Fatalf("write artwork: %v", err)
	}

	cachePath := filepath.Join(dir, "pipeline-cache.json")
	cache, err := loadPipelineCache(cachePath)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}

	downloadKey := "exact:https://example.com/watch?v=abc123"
	if err := cache.putDownload([]string{downloadKey}, pipelineDownloadCacheEntry{
		InfoJSON: ytDLPInfo{
			ID:         "abc123",
			Title:      "Track",
			WebpageURL: "https://example.com/watch?v=abc123",
		},
		AudioPath: audioPath,
		VideoPath: videoPath,
		SourceURL: "https://example.com/watch?v=abc123",
	}); err != nil {
		t.Fatalf("put download: %v", err)
	}

	reloaded, err := loadPipelineCache(cachePath)
	if err != nil {
		t.Fatalf("reload cache: %v", err)
	}
	download, ok := reloaded.downloadEntry([]string{downloadKey}, false)
	if !ok {
		t.Fatalf("expected download cache hit")
	}
	if download.AudioPath != audioPath {
		t.Fatalf("unexpected audio path: %s", download.AudioPath)
	}
	downloadVideo, ok := reloaded.downloadEntry([]string{downloadKey}, true)
	if !ok {
		t.Fatalf("expected video cache hit")
	}
	if downloadVideo.VideoPath != videoPath {
		t.Fatalf("unexpected video path: %s", downloadVideo.VideoPath)
	}

	mbKey := "musicbrainz:test"
	mbMetadata := songstore.MetadataResult{Title: "Track", Artist: "Artist", Album: "Album", Confidence: "high"}
	if err := reloaded.putMusicBrainz(mbKey, pipelineMusicBrainzCacheEntry{
		Metadata: mbMetadata,
		Detail:   "matched",
		Matched:  true,
	}); err != nil {
		t.Fatalf("put musicbrainz: %v", err)
	}
	if got, ok := reloaded.musicBrainzEntry(mbKey); !ok || got.Detail != "matched" {
		t.Fatalf("musicbrainz cache miss or wrong detail")
	}

	lyricsKey := "lrclib:test"
	lyrics := songstore.LyricsResult{
		Text:       "lyrics",
		Source:     "lrclib",
		Confidence: "high",
		IsComplete: true,
	}
	metadataCopy := mbMetadata
	if err := reloaded.putLyrics(lyricsKey, pipelineLyricsCacheEntry{
		Lyrics:   lyrics,
		Metadata: &metadataCopy,
		Detail:   "matched",
	}); err != nil {
		t.Fatalf("put lyrics: %v", err)
	}
	if got, ok := reloaded.lyricsEntry(lyricsKey); !ok || got.Lyrics.Text != "lyrics" {
		t.Fatalf("lyrics cache miss or wrong lyrics")
	}

	artKey := "artwork:test"
	artwork := songstore.SongArtwork{Path: artworkPath, Filename: "cover.jpg", Source: "coverartarchive", FetchedAt: time.Now().UTC()}
	if err := reloaded.putArtwork(artKey, pipelineArtworkCacheEntry{Artwork: artwork}); err != nil {
		t.Fatalf("put artwork: %v", err)
	}
	if got, ok := reloaded.artworkEntry(artKey); !ok || got.Artwork.Path != artworkPath {
		t.Fatalf("artwork cache miss or wrong path")
	}
}

func TestPipelineCacheDownloadKeysIgnoreWatchPath(t *testing.T) {
	first := pipelineCacheDownloadKeys("https://www.youtube.com/watch?v=fiAT6ri2mqg")
	second := pipelineCacheDownloadKeys("https://www.youtube.com/watch?v=y7sUoP3Rmx8")
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("expected download keys for watch urls")
	}
	for _, key := range first {
		if strings.HasPrefix(key, "path:") {
			t.Fatalf("unexpected path key in download cache keys: %q", key)
		}
	}
	if strings.Join(first, "\u0001") == strings.Join(second, "\u0001") {
		t.Fatalf("expected distinct download keys for distinct video ids")
	}
}

func TestPipelineCacheInvalidatesOlderSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline-cache.json")
	legacy := `{"version":1,"downloads":{"legacy":{"audioPath":"/tmp/legacy.mp3"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy cache: %v", err)
	}
	cache, err := loadPipelineCache(path)
	if err != nil {
		t.Fatalf("load legacy cache: %v", err)
	}
	if _, ok := cache.downloadEntry([]string{"legacy"}, false); ok {
		t.Fatal("expected legacy cache entry to be invalidated")
	}
	if cache.file.Version != pipelineCacheSchemaVersion {
		t.Fatalf("expected cache schema %d, got %d", pipelineCacheSchemaVersion, cache.file.Version)
	}
}

func TestPipelineCacheRejectsEmptyDirectoriesAndChangedArtifacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	cache := &PipelineCache{path: filepath.Join(dir, "cache.json")}
	key := "download:test"
	entry := pipelineDownloadCacheEntry{
		AudioPath: path,
		AudioHash: pipelineCacheArtifactHash(path),
	}
	if err := cache.putDownload([]string{key}, entry); err != nil {
		t.Fatalf("put download: %v", err)
	}
	if _, ok := cache.downloadEntry([]string{key}, false); !ok {
		t.Fatal("expected hashed regular artifact to be usable")
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatalf("rewrite artifact: %v", err)
	}
	if _, ok := cache.downloadEntry([]string{key}, false); ok {
		t.Fatal("expected changed artifact to invalidate cache entry")
	}

	dirEntry := pipelineDownloadCacheEntry{AudioPath: dir}
	if downloadCacheEntryUsable(dirEntry, false) {
		t.Fatal("expected directory to be rejected as a cached artifact")
	}
}
