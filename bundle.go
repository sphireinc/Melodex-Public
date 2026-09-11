package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const bundleManifestSchemaVersion = 1

func packageTrack(root string, metadata EnrichmentResult, sourceKind, sourceRef, originalPath string) (TrackRecord, BundleManifest, error) {
	info, err := os.Stat(originalPath)
	if err != nil {
		return TrackRecord{}, BundleManifest{}, err
	}
	artist := cleanDisplayName(metadata.Artist)
	if artist == "Unknown" {
		artist = "Unknown Artist"
	}
	album := cleanDisplayName(metadata.Album)
	if album == "Unknown" {
		album = "Singles"
	}
	title := cleanDisplayName(metadata.Title)
	if title == "Unknown" {
		title = cleanDisplayName(stringsTrimExt(filepath.Base(originalPath)))
	}
	if title == "Unknown" {
		title = "Untitled Track"
	}
	id := shortID()
	artistDir := sanitizePathPart(artist)
	albumDir := sanitizePathPart(album)
	bundleDir := filepath.Join(root, "Library", artistDir, albumDir)
	if err := ensureDir(bundleDir); err != nil {
		return TrackRecord{}, BundleManifest{}, err
	}
	bundleName := fmt.Sprintf("%s__%s.mldx", slugify(title), id)
	bundlePath := filepath.Join(bundleDir, bundleName)
	audioExt := filepath.Ext(info.Name())
	audioEntry := "audio" + audioExt
	if audioExt == "" {
		audioEntry = "audio.bin"
	}
	metadataEntry := "metadata.json"
	lyricsEntry := "lyrics.txt"
	hash, err := buildBundle(bundlePath, originalPath, audioEntry, metadataEntry, lyricsEntry, metadata, sourceKind, sourceRef, info.Name(), id)
	if err != nil {
		return TrackRecord{}, BundleManifest{}, err
	}
	record := TrackRecord{
		ID:             id,
		Artist:         artist,
		Album:          album,
		Title:          title,
		Genre:          metadata.Genre,
		Year:           metadata.Year,
		LyricsIncluded: metadata.Lyrics != "",
		BundlePath:     bundlePath,
		SourceKind:     sourceKind,
		SourceRef:      sourceRef,
		Hash:           hash,
		CreatedAt:      time.Now().UTC(),
	}
	manifest := BundleManifest{
		Version:        bundleManifestSchemaVersion,
		ID:             id,
		Artist:         artist,
		Album:          album,
		Title:          title,
		Genre:          metadata.Genre,
		Year:           metadata.Year,
		Lyrics:         metadata.Lyrics,
		SourceKind:     sourceKind,
		SourceRef:      sourceRef,
		OriginalName:   info.Name(),
		AudioEntry:     audioEntry,
		MetadataEntry:  metadataEntry,
		LyricsEntry:    lyricsEntry,
		Hash:           hash,
		CreatedAt:      time.Now().UTC(),
		EnrichmentNote: metadata.Notes,
	}
	return record, manifest, nil
}

func buildBundle(bundlePath, sourcePath, audioEntry, metadataEntry, lyricsEntry string, metadata EnrichmentResult, sourceKind, sourceRef, originalName, id string) (string, error) {
	in, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(bundlePath), 0o755); err != nil {
		return "", err
	}
	out, err := os.CreateTemp(filepath.Dir(bundlePath), "."+filepath.Base(bundlePath)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := out.Name()
	cleanup := func() {
		_ = out.Close()
		_ = os.Remove(tmpPath)
	}

	zipWriter := zip.NewWriter(out)

	audioWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
		Name:   audioEntry,
		Method: zip.Deflate,
	})
	if err != nil {
		cleanup()
		return "", err
	}
	hasher := sha256.New()
	if err := atomicWriteFailure(atomicWriteStageWrite, bundlePath); err != nil {
		cleanup()
		return "", err
	}
	if _, err := io.Copy(io.MultiWriter(audioWriter, hasher), in); err != nil {
		cleanup()
		return "", err
	}

	metaPayload, err := json.MarshalIndent(map[string]any{
		"artist":       metadata.Artist,
		"album":        metadata.Album,
		"title":        metadata.Title,
		"genre":        metadata.Genre,
		"year":         metadata.Year,
		"notes":        metadata.Notes,
		"sourceKind":   sourceKind,
		"sourceRef":    sourceRef,
		"originalName": originalName,
		"bundleId":     id,
	}, "", "  ")
	if err != nil {
		cleanup()
		return "", err
	}
	if err := writeZipText(zipWriter, metadataEntry, metaPayload); err != nil {
		cleanup()
		return "", err
	}
	if err := writeZipText(zipWriter, lyricsEntry, []byte(metadata.Lyrics)); err != nil {
		cleanup()
		return "", err
	}

	manifest := BundleManifest{
		Version:       bundleManifestSchemaVersion,
		ID:            id,
		Artist:        metadata.Artist,
		Album:         metadata.Album,
		Title:         metadata.Title,
		Genre:         metadata.Genre,
		Year:          metadata.Year,
		Lyrics:        metadata.Lyrics,
		SourceKind:    sourceKind,
		SourceRef:     sourceRef,
		OriginalName:  originalName,
		AudioEntry:    audioEntry,
		MetadataEntry: metadataEntry,
		LyricsEntry:   lyricsEntry,
		Hash:          hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:     time.Now().UTC(),
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		cleanup()
		return "", err
	}
	if err := writeZipText(zipWriter, "manifest.json", manifestBytes); err != nil {
		cleanup()
		return "", err
	}
	if err := zipWriter.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := out.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := commitAtomicTempFile(tmpPath, bundlePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return manifest.Hash, nil
}

func readManifest(file *os.File) (BundleManifest, error) {
	stat, err := file.Stat()
	if err != nil {
		return BundleManifest{}, err
	}
	reader, err := zip.NewReader(file, stat.Size())
	if err != nil {
		return BundleManifest{}, err
	}
	for _, entry := range reader.File {
		if entry.Name != "manifest.json" {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return BundleManifest{}, err
		}
		defer rc.Close()
		manifestBytes, err := io.ReadAll(rc)
		if err != nil {
			return BundleManifest{}, err
		}
		var manifest BundleManifest
		if err := decodePortableJSON(manifestBytes, &manifest, "bundle manifest"); err != nil {
			return BundleManifest{}, err
		}
		if err := validatePersistedSchemaVersion("bundle manifest", manifest.Version, bundleManifestSchemaVersion); err != nil {
			return BundleManifest{}, err
		}
		if manifest.Version == 0 {
			// Legacy bundles predate the explicit manifest version field.
			manifest.Version = bundleManifestSchemaVersion
		}
		return manifest, nil
	}
	return BundleManifest{}, fmt.Errorf("manifest.json not found in bundle")
}

func writeZipText(zipWriter *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func stringsTrimExt(name string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}

func sanitizePathPart(input string) string {
	input = cleanDisplayName(input)
	input = strings.ReplaceAll(input, "/", "-")
	input = strings.ReplaceAll(input, "\\", "-")
	input = strings.TrimSpace(input)
	if input == "" {
		return "Unknown"
	}
	return input
}
