package main

import (
	"errors"
	"strings"
)

// ScanLibraryHealth returns a deterministic read-only report over the current
// catalog. It deliberately does not apply any proposed fix or rewrite files.
func (a *App) ScanLibraryHealth() (LibraryHealthReport, error) {
	if a == nil {
		return LibraryHealthReport{}, errors.New("app is unavailable")
	}
	a.mu.Lock()
	tracks := append([]TrackRecord(nil), a.catalog.Tracks...)
	a.mu.Unlock()
	report := scanLibraryHealth(tracks)
	logEvent("library_health_scanned", "track_count", len(tracks), "issue_count", len(report.Issues))
	return report, nil
}

// FindDuplicateCandidates returns explainable, non-destructive suggestions for
// one catalog track. The caller must still ask the user whether to keep,
// reprocess, or import anything; this method never mutates the catalog.
func (a *App) FindDuplicateCandidates(trackID string, threshold float64) ([]DuplicateMatch, error) {
	if a == nil {
		return nil, errors.New("app is unavailable")
	}
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return nil, errors.New("track id is required")
	}
	a.mu.Lock()
	tracks := append([]TrackRecord(nil), a.catalog.Tracks...)
	a.mu.Unlock()
	var target TrackRecord
	found := false
	for _, track := range tracks {
		if track.ID == trackID {
			target = track
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("track not found")
	}
	matches := findDuplicateCandidates(tracks, target, threshold)
	logEvent("duplicate_candidates_found", "track_id", trackID, "match_count", len(matches))
	return matches, nil
}

// EvaluateSmartPlaylist previews a local smart-playlist definition against a
// catalog snapshot. Listening context is currently empty because persisted
// listening statistics are not yet part of the product schema; callers can
// still use all metadata and file-health predicates offline.
func (a *App) EvaluateSmartPlaylist(definition SmartPlaylistDefinition) (SmartPlaylistEvaluation, error) {
	if a == nil {
		return SmartPlaylistEvaluation{}, errors.New("app is unavailable")
	}
	a.mu.Lock()
	tracks := append([]TrackRecord(nil), a.catalog.Tracks...)
	a.mu.Unlock()
	evaluation, err := EvaluateSmartPlaylist(definition, tracks, nil)
	if err != nil {
		logEvent("smart_playlist_evaluation_failed", "playlist_id", definition.ID, "error", err.Error())
		return SmartPlaylistEvaluation{}, err
	}
	logEvent("smart_playlist_evaluated", "playlist_id", definition.ID, "match_count", evaluation.TotalMatches)
	return evaluation, nil
}

// BuildLibraryManifest previews the metadata-only backup payload for the
// current local catalog. It does not write or upload anything; file export and
// remote sync can build on this contract after their confirmation/security
// flows are selected.
func (a *App) BuildLibraryManifest() (LibraryManifest, error) {
	if a == nil {
		return LibraryManifest{}, errors.New("app is unavailable")
	}
	state := a.GetState()
	manifest, err := BuildLibraryManifest(LibraryManifestSourceFromAppState(state), LibraryManifestExportOptions{})
	if err != nil {
		logEvent("library_manifest_build_failed", "error", err.Error())
		return LibraryManifest{}, err
	}
	logEvent("library_manifest_built", "track_count", len(manifest.Tracks), "playlist_count", len(manifest.Playlists))
	return manifest, nil
}
