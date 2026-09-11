package main

import (
	"os"
	"path/filepath"
	"strings"

	"melodex/internal/songstore"
)

type localImportExistingAssets struct {
	MetadataPath   string
	Metadata       songstore.SongMetadata
	HasMetadata    bool
	Lyrics         songstore.LyricsResult
	PlainLyrics    string
	HasPlainLyrics bool
	TimedLyrics    []songstore.TimedLyricLine
	HasTimedLyrics bool
}

func loadLocalImportExistingAssets(source string) (localImportExistingAssets, error) {
	assets := localImportExistingAssets{}
	source = strings.TrimSpace(source)
	if source == "" {
		return assets, nil
	}

	metadataPath := localMetadataSidecarPath(source)
	if pathExists(metadataPath) {
		metadata, err := songstore.ReadMetadataJSON(metadataPath)
		if err != nil {
			return assets, err
		}
		if metadataHasMeaningfulData(metadataResultFromSongMetadata(metadata)) {
			assets.MetadataPath = metadataPath
			assets.Metadata = metadata
			assets.HasMetadata = true
		}
	}

	if plainLyrics, ok, err := readOptionalTextFile(localPlainLyricsSidecarPath(source)); err != nil {
		return assets, err
	} else if ok {
		assets.PlainLyrics = plainLyrics
		assets.HasPlainLyrics = true
	}

	if timedLyricsText, ok, err := readOptionalTextFile(localTimedLyricsSidecarPath(source)); err != nil {
		return assets, err
	} else if ok {
		assets.TimedLyrics = parseLRCLIBSyncedLyrics(timedLyricsText)
		assets.HasTimedLyrics = len(assets.TimedLyrics) > 0
		if !assets.HasPlainLyrics {
			assets.PlainLyrics = plainLyricsFromSyncedLyrics(timedLyricsText)
			assets.HasPlainLyrics = strings.TrimSpace(assets.PlainLyrics) != ""
		}
	}

	assets.Lyrics = songstore.LyricsResult{
		Text:              assets.PlainLyrics,
		TimedLyrics:       append([]songstore.TimedLyricLine(nil), assets.TimedLyrics...),
		TimingGranularity: lyricGranularityForLines(assets.TimedLyrics),
		Confidence:        "high",
		Source:            "local-sidecar",
		SourceLoc:         source,
		IsComplete:        strings.TrimSpace(assets.PlainLyrics) != "" || len(assets.TimedLyrics) > 0,
		Notes:             "Loaded from existing local lyrics sidecar(s).",
	}

	return assets, nil
}

func localMetadataSidecarPath(source string) string {
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return filepath.Join(filepath.Dir(source), base+".metadata.json")
}

func localPlainLyricsSidecarPath(source string) string {
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return filepath.Join(filepath.Dir(source), base+".txt")
}

func localTimedLyricsSidecarPath(source string) string {
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return filepath.Join(filepath.Dir(source), base+".lrc")
}

func readOptionalTextFile(path string) (string, bool, error) {
	if strings.TrimSpace(path) == "" || !pathExists(path) {
		return "", false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	text := string(data)
	if strings.TrimSpace(text) == "" {
		return "", false, nil
	}
	return text, true, nil
}

func metadataResultFromSongMetadata(metadata songstore.SongMetadata) songstore.MetadataResult {
	return songstore.MetadataResult{
		Title:                metadata.Title,
		Artist:               metadata.Artist,
		Album:                metadata.Album,
		TrackNumber:          metadata.TrackNumber,
		Year:                 metadata.Year,
		Genre:                metadata.Genre,
		DurationSeconds:      metadata.DurationSeconds,
		Confidence:           metadata.Confidence,
		MetadataConfidence:   metadata.MetadataConfidence,
		EnrichmentConfidence: metadata.EnrichmentConfidence,
		ReleaseType:          metadata.ReleaseType,
		ISRC:                 metadata.ISRC,
		ArtistLinks:          metadata.ArtistLinks,
		AlbumLinks:           metadata.AlbumLinks,
		SongLinks:            metadata.SongLinks,
		ArtistTrivia:         append([]string(nil), metadata.ArtistTrivia...),
		AlbumTrivia:          append([]string(nil), metadata.AlbumTrivia...),
		SongTrivia:           append([]string(nil), metadata.SongTrivia...),
		SongMeaning:          metadata.SongMeaning,
		Tidbits:              append([]string(nil), metadata.Tidbits...),
		Sources:              append([]string(nil), metadata.Sources...),
		Notes:                metadata.MetadataNotes,
	}
}

func mergeLocalLyricsResults(existing, fetched songstore.LyricsResult, needPlain, needTimed bool) songstore.LyricsResult {
	merged := existing
	if needPlain && strings.TrimSpace(merged.Text) == "" {
		merged.Text = fetched.Text
	}
	if needTimed && len(merged.TimedLyrics) == 0 {
		merged.TimedLyrics = append([]songstore.TimedLyricLine(nil), fetched.TimedLyrics...)
		merged.TimingGranularity = fetched.TimingGranularity
	}
	if merged.Confidence == "" {
		merged.Confidence = fetched.Confidence
	}
	if merged.Source == "" {
		merged.Source = fetched.Source
	} else if fetched.Source != "" && !strings.Contains(strings.ToLower(merged.Source), strings.ToLower(fetched.Source)) {
		merged.Source = strings.TrimSpace(merged.Source + ", " + fetched.Source)
	}
	if merged.SourceLoc == "" {
		merged.SourceLoc = fetched.SourceLoc
	}
	if merged.Notes == "" {
		merged.Notes = fetched.Notes
	} else if fetched.Notes != "" && !strings.Contains(strings.ToLower(merged.Notes), strings.ToLower(fetched.Notes)) {
		merged.Notes = strings.TrimSpace(merged.Notes + " " + fetched.Notes)
	}
	merged.IsComplete = strings.TrimSpace(merged.Text) != "" || len(merged.TimedLyrics) > 0
	if merged.TimingGranularity == "" && len(merged.TimedLyrics) > 0 {
		merged.TimingGranularity = lyricGranularityForLines(merged.TimedLyrics)
	}
	return merged
}

func plainLyricsFromTimedLines(lines []songstore.TimedLyricLine) string {
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSpace(line.Text))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
