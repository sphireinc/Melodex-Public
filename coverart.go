package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"melodex/internal/songstore"
)

const musicBrainzUserAgent = "Melodex/0.1 (https://melodex.local)"

type musicBrainzArtworkSearchResponse struct {
	ReleaseGroups []musicBrainzArtworkReleaseGroup `json:"release-groups"`
	Releases      []musicBrainzArtworkRelease      `json:"releases"`
}

type musicBrainzArtworkReleaseGroup struct {
	ID    string `json:"id"`
	Score int    `json:"score"`
	Title string `json:"title"`
}

type musicBrainzArtworkRelease struct {
	ID    string `json:"id"`
	Score int    `json:"score"`
	Title string `json:"title"`
}

func (a *App) maybeFetchArtwork(ctx context.Context, metadata songstore.SongMetadata, plan songstore.SongStoragePlan) songstore.SongArtwork {
	if !metadataSupportsArtworkLookup(metadata) {
		log.Printf("cover art skipped for %s - %s: insufficient metadata", metadata.Artist, metadata.Album)
		logEvent("cover_art_skipped", "artist", metadata.Artist, "album", metadata.Album, "reason", "insufficient_metadata")
		return songstore.SongArtwork{}
	}
	if pathExists(plan.ArtworkPath) {
		log.Printf("cover art reused for %s - %s: %s", metadata.Artist, metadata.Album, plan.ArtworkPath)
		logEvent("cover_art_reused", "artist", metadata.Artist, "album", metadata.Album, "artwork_path", plan.ArtworkPath)
		return songstore.SongArtwork{
			Filename: filepath.Base(plan.ArtworkPath),
			Path:     plan.ArtworkPath,
			Source:   "coverartarchive",
		}
	}

	log.Printf("cover art lookup started for %s - %s", metadata.Artist, metadata.Album)
	logEvent("cover_art_lookup_started", "artist", metadata.Artist, "album", metadata.Album, "title", metadata.Title)
	art, err := lookupCoverArtArchive(ctx, metadata.Artist, metadata.Album, metadata.Title)
	if err != nil {
		log.Printf("cover art lookup failed for %s - %s: %v", metadata.Artist, metadata.Album, err)
		logEvent("cover_art_lookup_failed", "artist", metadata.Artist, "album", metadata.Album, "error", err.Error())
		return songstore.SongArtwork{}
	}
	if art.URL == "" {
		log.Printf("cover art not found for %s - %s", metadata.Artist, metadata.Album)
		logEvent("cover_art_not_found", "artist", metadata.Artist, "album", metadata.Album)
		return songstore.SongArtwork{}
	}
	log.Printf("cover art download started for %s - %s: %s", metadata.Artist, metadata.Album, art.URL)
	logEvent("cover_art_download_started", "artist", metadata.Artist, "album", metadata.Album, "url", art.URL)
	data, err := downloadArtwork(ctx, art.URL)
	if err != nil {
		log.Printf("cover art download failed for %s - %s - %s: %v", metadata.Artist, metadata.Album, art.URL, err)
		logEvent("cover_art_download_failed", "artist", metadata.Artist, "album", metadata.Album, "url", art.URL, "error", err.Error())
		return songstore.SongArtwork{}
	}
	if err := songstore.WriteBinaryFile(plan.ArtworkPath, data); err != nil {
		log.Printf("cover art write failed for %s - %s: %v", metadata.Artist, metadata.Album, err)
		logEvent("cover_art_write_failed", "artist", metadata.Artist, "album", metadata.Album, "artwork_path", plan.ArtworkPath, "error", err.Error())
		return songstore.SongArtwork{}
	}
	log.Printf("cover art written for %s - %s: %s", metadata.Artist, metadata.Album, plan.ArtworkPath)
	logEvent("cover_art_written", "artist", metadata.Artist, "album", metadata.Album, "artwork_path", plan.ArtworkPath)
	art.Path = plan.ArtworkPath
	art.Filename = filepath.Base(plan.ArtworkPath)
	return art
}

func lookupCoverArtArchive(ctx context.Context, artist, album, title string) (songstore.SongArtwork, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	queries := buildCoverArtQueries(artist, album, title)
	for _, query := range queries {
		if art, ok, err := searchCoverArtCandidate(ctx, client, "release-group", query); err != nil {
			return songstore.SongArtwork{}, err
		} else if ok {
			return art, nil
		}
		if art, ok, err := searchCoverArtCandidate(ctx, client, "release", query); err != nil {
			return songstore.SongArtwork{}, err
		} else if ok {
			return art, nil
		}
	}
	return songstore.SongArtwork{}, nil
}

func buildCoverArtQueries(artist, album, title string) []string {
	artist = strings.TrimSpace(artist)
	album = strings.TrimSpace(album)
	title = strings.TrimSpace(title)
	candidate := album
	if candidate == "" {
		candidate = title
	}
	queries := make([]string, 0, 2)
	if artist != "" && candidate != "" {
		queries = append(queries, fmt.Sprintf(`artist:"%s" AND releasegroup:"%s"`, escapeMusicBrainzQuery(artist), escapeMusicBrainzQuery(candidate)))
		queries = append(queries, fmt.Sprintf(`artist:"%s" AND release:"%s"`, escapeMusicBrainzQuery(artist), escapeMusicBrainzQuery(candidate)))
	}
	if title != "" && title != candidate && artist != "" {
		queries = append(queries, fmt.Sprintf(`artist:"%s" AND releasegroup:"%s"`, escapeMusicBrainzQuery(artist), escapeMusicBrainzQuery(title)))
	}
	return uniqueStrings(queries)
}

func searchCoverArtCandidate(ctx context.Context, client *http.Client, searchType, query string) (songstore.SongArtwork, bool, error) {
	if strings.TrimSpace(query) == "" {
		return songstore.SongArtwork{}, false, nil
	}
	endpoint := fmt.Sprintf("https://musicbrainz.org/ws/2/%s/?fmt=json&limit=1&query=%s", searchType, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return songstore.SongArtwork{}, false, err
	}
	req.Header.Set("User-Agent", musicBrainzUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return songstore.SongArtwork{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return songstore.SongArtwork{}, false, nil
	}

	var payload musicBrainzArtworkSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return songstore.SongArtwork{}, false, err
	}

	if searchType == "release-group" {
		for _, result := range payload.ReleaseGroups {
			if strings.TrimSpace(result.ID) == "" {
				continue
			}
			if art, ok := downloadCoverArtCandidate(ctx, client, "release-group", result.ID); ok {
				return art, true, nil
			}
		}
	}
	for _, result := range payload.Releases {
		if strings.TrimSpace(result.ID) == "" {
			continue
		}
		if art, ok := downloadCoverArtCandidate(ctx, client, "release", result.ID); ok {
			return art, true, nil
		}
	}
	return songstore.SongArtwork{}, false, nil
}

func downloadCoverArtCandidate(ctx context.Context, client *http.Client, kind, mbid string) (songstore.SongArtwork, bool) {
	endpoints := []string{
		fmt.Sprintf("https://coverartarchive.org/%s/%s/front-500", kind, mbid),
		fmt.Sprintf("https://coverartarchive.org/%s/%s/front", kind, mbid),
	}
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return songstore.SongArtwork{}, false
		}
		req.Header.Set("User-Agent", musicBrainzUserAgent)
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			if _, err := io.ReadAll(resp.Body); err != nil {
				return
			}
		}()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		return songstore.SongArtwork{
			Source:         "coverartarchive",
			URL:            endpoint,
			ReleaseID:      releaseIDForKind(kind, mbid),
			ReleaseGroupID: releaseGroupIDForKind(kind, mbid),
			FetchedAt:      time.Now().UTC(),
		}, true
	}
	return songstore.SongArtwork{}, false
}

func downloadArtwork(ctx context.Context, artworkURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artworkURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", musicBrainzUserAgent)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover art archive returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func escapeMusicBrainzQuery(value string) string {
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.TrimSpace(value)
	return value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func releaseIDForKind(kind, mbid string) string {
	if kind == "release" {
		return mbid
	}
	return ""
}

func releaseGroupIDForKind(kind, mbid string) string {
	if kind == "release-group" {
		return mbid
	}
	return ""
}

func metadataSupportsArtworkLookup(metadata songstore.SongMetadata) bool {
	if strings.TrimSpace(metadata.Artist) == "" {
		return false
	}
	if strings.TrimSpace(metadata.Album) != "" {
		return true
	}
	return strings.TrimSpace(metadata.Title) != ""
}
