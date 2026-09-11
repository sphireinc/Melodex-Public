package main

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"melodex/internal/songstore"
)

func pipelineCachePromptKey(baseURL, model string, prompts map[string]string) string {
	return pipelineCacheKey("ai", baseURL, model, prompts)
}

func pipelineCacheMetadataKey(input EnrichmentInput, metadata songstore.MetadataResult) string {
	return pipelineCacheKey("musicbrainz",
		input.SourceURL,
		input.SourceRef,
		input.FileName,
		input.SourceTitle,
		input.SourceUploader,
		input.SourceChannel,
		metadata.Title,
		metadata.Artist,
		metadata.Album,
		intHint(metadata.TrackNumber),
		intHint(metadata.Year),
		metadata.Genre,
		intHint(metadata.DurationSeconds),
	)
}

func pipelineCacheLyricsKey(input EnrichmentInput, metadata songstore.MetadataResult) string {
	return pipelineCacheKey("lrclib",
		input.SourceURL,
		input.SourceRef,
		input.FileName,
		metadata.Title,
		metadata.Artist,
		metadata.Album,
		intHint(metadata.DurationSeconds),
	)
}

func pipelineCacheArtworkKey(metadata songstore.SongMetadata) string {
	return pipelineCacheKey("artwork",
		metadata.Artist,
		metadata.Album,
		metadata.Title,
		metadata.ReleaseType,
		metadata.ISRC,
		intHint(metadata.Year),
	)
}

func pipelineCacheDownloadKeys(sourceURL string) []string {
	return pipelineCacheKeysForURL(sourceURL)
}

func (a *App) cachedDownloadForURL(sourceURL string, needVideo bool) (pipelineDownloadCacheEntry, bool) {
	if a == nil || a.pipelineCache == nil {
		return pipelineDownloadCacheEntry{}, false
	}
	keys := pipelineCacheDownloadKeys(sourceURL)
	if len(keys) == 0 {
		return pipelineDownloadCacheEntry{}, false
	}
	entry, ok := a.pipelineCache.downloadEntry(keys, needVideo)
	if ok {
		logEvent("pipeline_cache_hit", "stage", "download", "source_url", sourceURL, "need_video", needVideo)
	}
	return entry, ok
}

func (a *App) rememberDownloadForURL(sourceURL string, info ytDLPInfo, audioPath, videoPath string) {
	if a == nil || a.pipelineCache == nil {
		return
	}
	keys := pipelineCacheDownloadKeys(sourceURL)
	if len(keys) == 0 {
		return
	}
	entry := pipelineDownloadCacheEntry{
		InfoJSON:    info,
		AudioPath:   strings.TrimSpace(audioPath),
		VideoPath:   strings.TrimSpace(videoPath),
		SourceURL:   firstNonEmpty(info.WebpageURL, sourceURL),
		AudioHash:   pipelineCacheArtifactHash(audioPath),
		VideoHash:   pipelineCacheArtifactHash(videoPath),
		ToolKey:     pipelineCacheKey("tools", a.settings.YTDLPPath, a.settings.FFmpegPath),
		SettingsKey: pipelineCacheKey("download-settings", a.settings.DownloadMusicVideo, a.settings.KeepOriginalAudio),
	}
	if entry.AudioPath == "" && entry.VideoPath == "" {
		return
	}
	if err := a.pipelineCache.putDownload(keys, entry); err != nil {
		log.Printf("pipeline cache store failed for download %s: %v", sourceURL, err)
		logEvent("pipeline_cache_store_failed", "stage", "download", "source_url", sourceURL, "error", err.Error())
	}
}

func (a *App) cachedMusicBrainzLookup(ctx context.Context, input EnrichmentInput, metadata songstore.MetadataResult) (songstore.MetadataResult, string, bool) {
	if a == nil || a.pipelineCache == nil {
		return fetchMetadataFromMusicBrainz(ctx, input, metadata)
	}
	key := pipelineCacheMetadataKey(input, metadata)
	if entry, ok := a.pipelineCache.musicBrainzEntry(key); ok {
		logEvent("pipeline_cache_hit", "stage", "musicbrainz", "source_url", input.SourceURL, "file_name", input.FileName)
		return entry.Metadata, entry.Detail, entry.Matched
	}
	updated, detail, matched := fetchMetadataFromMusicBrainz(ctx, input, metadata)
	if strings.TrimSpace(detail) != "" {
		if err := a.pipelineCache.putMusicBrainz(key, pipelineMusicBrainzCacheEntry{
			Metadata: updated,
			Detail:   detail,
			Matched:  matched,
		}); err != nil {
			log.Printf("pipeline cache store failed for musicbrainz %s: %v", input.FileName, err)
			logEvent("pipeline_cache_store_failed", "stage", "musicbrainz", "file_name", input.FileName, "error", err.Error())
		}
	}
	return updated, detail, matched
}

func (a *App) cachedAIMetadata(ctx context.Context, input EnrichmentInput, metadata songstore.MetadataResult) (songstore.MetadataResult, aiOutcome, bool) {
	if a == nil || a.promptLibrary == nil || a.ai == nil || !a.ai.available() {
		return metadata, aiOutcome{
			Ran:     false,
			Status:  "skipped",
			Message: "AI unavailable; using MusicBrainz metadata.",
		}, false
	}
	aiInput := input.WithResolvedMetadata(metadata, input.LibraryRoot, input.FileName)
	systemPrompt, userPrompt, err := a.promptLibrary.MetadataPrompt(aiInput.PromptMap())
	if err != nil {
		return metadata, aiOutcome{
			Ran:     false,
			Status:  "skipped",
			Message: "AI enrichment skipped because the prompt set is unavailable.",
		}, false
	}
	key := pipelineCachePromptKey(a.ai.baseURL, a.ai.model, map[string]string{
		"system": systemPrompt,
		"user":   userPrompt,
	})
	if a.pipelineCache != nil {
		if entry, ok := a.pipelineCache.aiEntry(key); ok {
			logEvent("pipeline_cache_hit", "stage", "ai_metadata", "source_url", input.SourceURL, "file_name", input.FileName)
			return entry.Metadata, entry.Outcome, true
		}
	}
	aiMetadata, err := a.ai.enrichMetadata(ctx, a.promptLibrary, aiInput)
	if err != nil {
		return metadata, aiOutcome{
			Ran:     true,
			Status:  "failed",
			Message: err.Error(),
		}, false
	}
	merged := mergeDeterministicMetadata(metadata, aiMetadata)
	success := metadataHasMeaningfulData(merged)
	outcome := aiOutcome{
		Ran:     true,
		Success: success,
		Status:  "success",
		Message: "Metadata discovered.",
	}
	outcome.Status, outcome.Message = buildMetadataAIStatus(outcome.Success, nil, merged)
	if outcome.Status != "failed" && a.pipelineCache != nil {
		if err := a.pipelineCache.putAI(key, pipelineAICacheEntry{
			Metadata: merged,
			Outcome:  outcome,
		}); err != nil {
			log.Printf("pipeline cache store failed for ai metadata %s: %v", input.FileName, err)
			logEvent("pipeline_cache_store_failed", "stage", "ai_metadata", "file_name", input.FileName, "error", err.Error())
		}
	}
	return merged, outcome, false
}

func (a *App) cachedLyricsLookup(ctx context.Context, baseURL string, input EnrichmentInput, metadata songstore.MetadataResult) (songstore.LyricsResult, string, *songstore.MetadataResult) {
	if a == nil || a.pipelineCache == nil {
		return fetchLyricsFromLRCLIB(ctx, baseURL, nil, input, metadata)
	}
	key := pipelineCacheLyricsKey(input, metadata)
	if entry, ok := a.pipelineCache.lyricsEntry(key); ok {
		logEvent("pipeline_cache_hit", "stage", "lrclib", "source_url", input.SourceURL, "file_name", input.FileName)
		if entry.Metadata != nil {
			copyMetadata := *entry.Metadata
			return entry.Lyrics, entry.Detail, &copyMetadata
		}
		return entry.Lyrics, entry.Detail, nil
	}
	lyrics, detail, updated := fetchLyricsFromLRCLIB(ctx, baseURL, nil, input, metadata)
	if strings.TrimSpace(detail) != "" {
		var cachedMetadata *songstore.MetadataResult
		if updated != nil {
			copyMetadata := *updated
			cachedMetadata = &copyMetadata
		}
		if err := a.pipelineCache.putLyrics(key, pipelineLyricsCacheEntry{
			Lyrics:   lyrics,
			Metadata: cachedMetadata,
			Detail:   detail,
		}); err != nil {
			log.Printf("pipeline cache store failed for lrclib %s: %v", input.FileName, err)
			logEvent("pipeline_cache_store_failed", "stage", "lrclib", "file_name", input.FileName, "error", err.Error())
		}
	}
	return lyrics, detail, updated
}

func (a *App) cachedArtworkLookup(ctx context.Context, metadata songstore.SongMetadata, plan songstore.SongStoragePlan) songstore.SongArtwork {
	if a == nil || a.pipelineCache == nil {
		return a.maybeFetchArtwork(ctx, metadata, plan)
	}
	key := pipelineCacheArtworkKey(metadata)
	if entry, ok := a.pipelineCache.artworkEntry(key); ok {
		artwork := entry.Artwork
		if cacheArtifactUsable(artwork.Path, entry.ArtifactHash) {
			if filepath.Clean(artwork.Path) != filepath.Clean(plan.ArtworkPath) && strings.TrimSpace(plan.ArtworkPath) != "" {
				if err := copyFile(artwork.Path, plan.ArtworkPath); err == nil {
					artwork.Path = plan.ArtworkPath
					artwork.Filename = filepath.Base(plan.ArtworkPath)
				}
			}
			logEvent("pipeline_cache_hit", "stage", "artwork", "artist", metadata.Artist, "album", metadata.Album)
			return artwork
		}
	}
	artwork := a.maybeFetchArtwork(ctx, metadata, plan)
	if artwork.Path != "" {
		if err := a.pipelineCache.putArtwork(key, pipelineArtworkCacheEntry{Artwork: artwork, ArtifactHash: pipelineCacheArtifactHash(artwork.Path)}); err != nil {
			log.Printf("pipeline cache store failed for artwork %s - %s: %v", metadata.Artist, metadata.Album, err)
			logEvent("pipeline_cache_store_failed", "stage", "artwork", "artist", metadata.Artist, "album", metadata.Album, "error", err.Error())
		}
	}
	return artwork
}
