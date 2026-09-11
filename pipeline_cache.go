package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"melodex/internal/songstore"
)

// Bump this when cache entries or their key contracts change. Older entries
// are intentionally discarded because a cache hit must never bypass a changed
// importer, decoder, prompt, or external-tool contract.
const pipelineCacheSchemaVersion = 2
const pipelineCacheKeyVersion = 2

type pipelineCacheFile struct {
	Version     int                                      `json:"version"`
	Downloads   map[string]pipelineDownloadCacheEntry    `json:"downloads,omitempty"`
	MusicBrainz map[string]pipelineMusicBrainzCacheEntry `json:"musicBrainz,omitempty"`
	AI          map[string]pipelineAICacheEntry          `json:"ai,omitempty"`
	Lyrics      map[string]pipelineLyricsCacheEntry      `json:"lyrics,omitempty"`
	Artwork     map[string]pipelineArtworkCacheEntry     `json:"artwork,omitempty"`
	UpdatedAt   time.Time                                `json:"updatedAt"`
}

type pipelineDownloadCacheEntry struct {
	InfoJSON   ytDLPInfo `json:"infoJson"`
	AudioPath  string    `json:"audioPath,omitempty"`
	VideoPath  string    `json:"videoPath,omitempty"`
	CachedAt   time.Time `json:"cachedAt"`
	SourceURL  string    `json:"sourceUrl,omitempty"`
	SourceKeys []string  `json:"sourceKeys,omitempty"`
	// Artifact hashes make a cache hit prove that the staged file is still the
	// file that was recorded. Older entries without a hash remain readable but
	// are validated using the conservative regular-file/non-empty checks below.
	AudioHash   string `json:"audioHash,omitempty"`
	VideoHash   string `json:"videoHash,omitempty"`
	ToolKey     string `json:"toolKey,omitempty"`
	SettingsKey string `json:"settingsKey,omitempty"`
}

type pipelineMusicBrainzCacheEntry struct {
	Metadata songstore.MetadataResult `json:"metadata"`
	Detail   string                   `json:"detail"`
	Matched  bool                     `json:"matched"`
	CachedAt time.Time                `json:"cachedAt"`
}

type pipelineAICacheEntry struct {
	Metadata songstore.MetadataResult `json:"metadata"`
	Outcome  aiOutcome                `json:"outcome"`
	CachedAt time.Time                `json:"cachedAt"`
}

type pipelineLyricsCacheEntry struct {
	Lyrics   songstore.LyricsResult    `json:"lyrics"`
	Metadata *songstore.MetadataResult `json:"metadata,omitempty"`
	Detail   string                    `json:"detail"`
	CachedAt time.Time                 `json:"cachedAt"`
}

type pipelineArtworkCacheEntry struct {
	Artwork      songstore.SongArtwork `json:"artwork"`
	CachedAt     time.Time             `json:"cachedAt"`
	ArtifactHash string                `json:"artifactHash,omitempty"`
}

type PipelineCache struct {
	mu   sync.Mutex
	path string
	file pipelineCacheFile
}

func loadPipelineCache(path string) (*PipelineCache, error) {
	cache := &PipelineCache{
		path: path,
		file: pipelineCacheFile{
			Version:     pipelineCacheSchemaVersion,
			Downloads:   map[string]pipelineDownloadCacheEntry{},
			MusicBrainz: map[string]pipelineMusicBrainzCacheEntry{},
			AI:          map[string]pipelineAICacheEntry{},
			Lyrics:      map[string]pipelineLyricsCacheEntry{},
			Artwork:     map[string]pipelineArtworkCacheEntry{},
		},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cache, nil
		}
		return nil, err
	}
	// Pipeline cache entries are disposable. Unknown additive fields are
	// intentionally ignored, while any non-current version is discarded below
	// rather than reused under an incompatible importer contract.
	if err := decodeCacheJSON(data, &cache.file, "pipeline cache"); err != nil {
		return nil, err
	}
	if cache.file.Version != 0 && cache.file.Version != pipelineCacheSchemaVersion {
		// The cache is disposable. Starting empty is safer than reusing an entry
		// whose stage identity was produced by an older implementation.
		cache.file = pipelineCacheFile{
			Version:     pipelineCacheSchemaVersion,
			Downloads:   map[string]pipelineDownloadCacheEntry{},
			MusicBrainz: map[string]pipelineMusicBrainzCacheEntry{},
			AI:          map[string]pipelineAICacheEntry{},
			Lyrics:      map[string]pipelineLyricsCacheEntry{},
			Artwork:     map[string]pipelineArtworkCacheEntry{},
		}
	}
	cache.normalizeLocked()
	return cache, nil
}

func (c *PipelineCache) normalizeLocked() {
	if c == nil {
		return
	}
	if c.file.Version == 0 {
		c.file.Version = pipelineCacheSchemaVersion
	}
	if c.file.Downloads == nil {
		c.file.Downloads = map[string]pipelineDownloadCacheEntry{}
	}
	if c.file.MusicBrainz == nil {
		c.file.MusicBrainz = map[string]pipelineMusicBrainzCacheEntry{}
	}
	if c.file.AI == nil {
		c.file.AI = map[string]pipelineAICacheEntry{}
	}
	if c.file.Lyrics == nil {
		c.file.Lyrics = map[string]pipelineLyricsCacheEntry{}
	}
	if c.file.Artwork == nil {
		c.file.Artwork = map[string]pipelineArtworkCacheEntry{}
	}
}

func (c *PipelineCache) saveLocked() error {
	if c == nil {
		return nil
	}
	c.file.Version = pipelineCacheSchemaVersion
	c.file.UpdatedAt = time.Now().UTC()
	if err := ensureDir(filepath.Dir(c.path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.file, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(c.path, data, 0o600)
}

func (c *PipelineCache) downloadEntry(keys []string, needVideo bool) (pipelineDownloadCacheEntry, bool) {
	if c == nil {
		return pipelineDownloadCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		entry, ok := c.file.Downloads[key]
		if !ok {
			continue
		}
		if !downloadCacheEntryUsable(entry, needVideo) {
			delete(c.file.Downloads, key)
			_ = c.saveLocked()
			continue
		}
		return entry, true
	}
	return pipelineDownloadCacheEntry{}, false
}

func downloadCacheEntryUsable(entry pipelineDownloadCacheEntry, needVideo bool) bool {
	if needVideo {
		if cacheArtifactUsable(entry.VideoPath, entry.VideoHash) {
			return true
		}
		return false
	}
	if cacheArtifactUsable(entry.AudioPath, entry.AudioHash) {
		return true
	}
	if cacheArtifactUsable(entry.VideoPath, entry.VideoHash) {
		return true
	}
	return false
}

func cacheArtifactUsable(path, expectedHash string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return false
	}
	expectedHash = strings.TrimSpace(expectedHash)
	if expectedHash == "" {
		return true
	}
	actual, err := fileSHA256(path)
	return err == nil && strings.EqualFold(actual, expectedHash)
}

func (c *PipelineCache) putDownload(keys []string, entry pipelineDownloadCacheEntry) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry.CachedAt = time.Now().UTC()
	entry.SourceKeys = uniqueStrings(keys)
	for _, key := range entry.SourceKeys {
		c.file.Downloads[key] = entry
	}
	return c.saveLocked()
}

func (c *PipelineCache) musicBrainzEntry(key string) (pipelineMusicBrainzCacheEntry, bool) {
	if c == nil {
		return pipelineMusicBrainzCacheEntry{}, false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return pipelineMusicBrainzCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry, ok := c.file.MusicBrainz[key]
	return entry, ok
}

func (c *PipelineCache) putMusicBrainz(key string, entry pipelineMusicBrainzCacheEntry) error {
	if c == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry.CachedAt = time.Now().UTC()
	c.file.MusicBrainz[key] = entry
	return c.saveLocked()
}

func (c *PipelineCache) aiEntry(key string) (pipelineAICacheEntry, bool) {
	if c == nil {
		return pipelineAICacheEntry{}, false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return pipelineAICacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry, ok := c.file.AI[key]
	return entry, ok
}

func (c *PipelineCache) putAI(key string, entry pipelineAICacheEntry) error {
	if c == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry.CachedAt = time.Now().UTC()
	c.file.AI[key] = entry
	return c.saveLocked()
}

func (c *PipelineCache) lyricsEntry(key string) (pipelineLyricsCacheEntry, bool) {
	if c == nil {
		return pipelineLyricsCacheEntry{}, false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return pipelineLyricsCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry, ok := c.file.Lyrics[key]
	return entry, ok
}

func (c *PipelineCache) putLyrics(key string, entry pipelineLyricsCacheEntry) error {
	if c == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry.CachedAt = time.Now().UTC()
	c.file.Lyrics[key] = entry
	return c.saveLocked()
}

func (c *PipelineCache) artworkEntry(key string) (pipelineArtworkCacheEntry, bool) {
	if c == nil {
		return pipelineArtworkCacheEntry{}, false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return pipelineArtworkCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry, ok := c.file.Artwork[key]
	return entry, ok
}

func (c *PipelineCache) putArtwork(key string, entry pipelineArtworkCacheEntry) error {
	if c == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeLocked()
	entry.CachedAt = time.Now().UTC()
	c.file.Artwork[key] = entry
	return c.saveLocked()
}

func pipelineCacheHash(parts ...any) string {
	data, err := json.Marshal(parts)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func pipelineCacheArtifactHash(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	hash, err := fileSHA256(path)
	if err != nil {
		return ""
	}
	return hash
}

func pipelineCacheKey(prefix string, parts ...any) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "stage"
	}
	hashParts := make([]any, 0, len(parts)+1)
	hashParts = append(hashParts, pipelineCacheKeyVersion)
	hashParts = append(hashParts, parts...)
	key := pipelineCacheHash(hashParts...)
	if key == "" {
		return ""
	}
	return prefix + ":" + key
}

func pipelineCacheKeysForURL(raw string) []string {
	keys := canonicalURLDuplicateKeys(raw)
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.HasPrefix(key, "path:") {
			continue
		}
		filtered = append(filtered, pipelineCacheKey("download", key))
	}
	return uniqueDuplicateStrings(filtered)
}
