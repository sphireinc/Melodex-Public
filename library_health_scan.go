package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// LibraryHealthIssue is a read-only, explainable finding. Scanning never
// writes metadata, moves files, or queues work; a later confirmation flow can
// decide whether a proposed fix is appropriate.
type LibraryHealthIssue struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	TrackID        string   `json:"trackId,omitempty"`
	RelatedTrackID string   `json:"relatedTrackId,omitempty"`
	Title          string   `json:"title,omitempty"`
	Severity       string   `json:"severity"`
	Confidence     string   `json:"confidence"`
	Evidence       []string `json:"evidence"`
	AffectedPaths  []string `json:"affectedPaths"`
	ProposedFix    string   `json:"proposedFix"`
}

type LibraryHealthReport struct {
	Version string               `json:"version"`
	Issues  []LibraryHealthIssue `json:"issues"`
}

const libraryHealthReportVersion = 1

// scanLibraryHealth computes findings from the supplied catalog snapshot. It
// intentionally accepts tracks rather than an App so callers can scan a copy
// of a catalog, test malformed records, and preview results without holding
// application locks or touching the filesystem.
func scanLibraryHealth(tracks []TrackRecord) LibraryHealthReport {
	ordered := append([]TrackRecord(nil), tracks...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})

	issues := make([]LibraryHealthIssue, 0)
	for _, track := range ordered {
		issues = append(issues, trackHealthIssues(track)...)
	}
	for i, left := range ordered {
		matches := findDuplicateCandidates(ordered[i+1:], left, 0.72)
		for _, match := range matches {
			issues = append(issues, duplicateHealthIssue(left, match))
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].ID == issues[j].ID {
			return issues[i].Kind < issues[j].Kind
		}
		return issues[i].ID < issues[j].ID
	})
	return LibraryHealthReport{Version: fmt.Sprint(libraryHealthReportVersion), Issues: issues}
}

func trackHealthIssues(track TrackRecord) []LibraryHealthIssue {
	issues := make([]LibraryHealthIssue, 0, 6)
	add := func(kind, severity, confidence, fix string, evidence, paths []string) {
		issues = append(issues, newLibraryHealthIssue(kind, track.ID, "", trackDisplayName(track), severity, confidence, evidence, paths, fix))
	}
	if track.AudioPath == "" || !pathExists(track.AudioPath) {
		evidence := []string{"catalog audio path is empty or the referenced file is not readable"}
		add("missing-audio", "error", "high", "Locate the original audio file or remove this stale catalog entry after confirmation.", evidence, nonEmptyPaths(track.AudioPath))
	}
	if track.MetadataPath == "" || !pathExists(track.MetadataPath) {
		add("missing-metadata", "error", "high", "Rebuild metadata from the retained audio file after reviewing the proposed fields.", []string{"catalog metadata sidecar is empty or missing"}, nonEmptyPaths(track.MetadataPath))
	}
	if strings.TrimSpace(track.ArtworkPath) == "" || !pathExists(track.ArtworkPath) {
		add("missing-artwork", "warning", "high", "Choose artwork locally or run artwork lookup, then preview before saving.", []string{"no readable artwork file is recorded"}, nonEmptyPaths(track.ArtworkPath))
	}
	if strings.TrimSpace(track.LyricsPath) == "" || !pathExists(track.LyricsPath) {
		add("missing-lyrics", "info", "high", "Run lyrics lookup or attach a lyrics sidecar only after reviewing the source and rights.", []string{"no readable lyrics sidecar is recorded"}, nonEmptyPaths(track.LyricsPath))
	}
	if strings.TrimSpace(track.LRCPath) == "" || !pathExists(track.LRCPath) {
		add("missing-timed-lyrics", "info", "high", "Add a reviewed timed-lyrics sidecar when one is available.", []string{"no readable timed-lyrics sidecar is recorded"}, nonEmptyPaths(track.LRCPath))
	}
	if isTrackUnprocessed(track) {
		add("low-metadata-confidence", "warning", confidenceForMetadata(track), "Preview normalized metadata and enrichment, then selectively apply the fields you trust.", []string{"metadata confidence is missing or below the processed threshold"}, nonEmptyPaths(track.MetadataPath))
	}
	return issues
}

func duplicateHealthIssue(track TrackRecord, match DuplicateMatch) LibraryHealthIssue {
	related := match.TrackID
	evidence := append([]string(nil), match.Reasons...)
	evidence = append(evidence, duplicateReasonSummary(match))
	paths := make([]string, 0, 2)
	if strings.TrimSpace(track.AudioPath) != "" {
		paths = append(paths, filepath.Clean(track.AudioPath))
	}
	return newLibraryHealthIssue("possible-duplicate", track.ID, related, trackDisplayName(track), "warning", duplicateConfidence(match.Score), evidence, paths, "Compare both tracks and choose keep, reprocess, or import anyway; do not delete automatically.")
}

func newLibraryHealthIssue(kind, trackID, relatedTrackID, title, severity, confidence string, evidence, paths []string, fix string) LibraryHealthIssue {
	evidence = sortedUniqueStrings(evidence)
	paths = sortedUniqueStrings(paths)
	identity := strings.Join([]string{kind, trackID, relatedTrackID, title, strings.Join(evidence, "\x1f"), strings.Join(paths, "\x1f")}, "\x1e")
	hash := sha256.Sum256([]byte(identity))
	return LibraryHealthIssue{
		ID:             "health-" + hex.EncodeToString(hash[:8]),
		Kind:           kind,
		TrackID:        trackID,
		RelatedTrackID: relatedTrackID,
		Title:          title,
		Severity:       severity,
		Confidence:     confidence,
		Evidence:       evidence,
		AffectedPaths:  paths,
		ProposedFix:    fix,
	}
}

func trackDisplayName(track TrackRecord) string {
	title := strings.TrimSpace(track.Title)
	artist := strings.TrimSpace(track.Artist)
	if title == "" {
		title = track.ID
	}
	if artist == "" {
		return title
	}
	return artist + " - " + title
}

func confidenceForMetadata(track TrackRecord) string {
	if strings.TrimSpace(track.MetadataConfidence) == "" {
		return "high"
	}
	return strings.ToLower(strings.TrimSpace(track.MetadataConfidence))
}

func duplicateConfidence(score float64) string {
	switch {
	case score >= 0.95:
		return "high"
	case score >= 0.82:
		return "medium"
	default:
		return "low"
	}
}

func nonEmptyPaths(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{filepath.Clean(value)}
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
