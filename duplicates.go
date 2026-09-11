package main

import (
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

type URLImportDuplicateInfo struct {
	Exists       bool   `json:"exists"`
	TrackID      string `json:"trackId,omitempty"`
	JobID        string `json:"jobId,omitempty"`
	MetadataPath string `json:"metadataPath,omitempty"`
	Title        string `json:"title,omitempty"`
	Artist       string `json:"artist,omitempty"`
	Album        string `json:"album,omitempty"`
	SourceURL    string `json:"sourceUrl,omitempty"`
	VideoURL     string `json:"videoUrl,omitempty"`
	MatchReason  string `json:"matchReason,omitempty"`
}

// DuplicateMatch is a non-destructive suggestion produced from local track
// metadata. A caller must still ask the user whether to reprocess, import
// anyway, or replace anything; this helper never mutates or deletes files.
type DuplicateMatch struct {
	TrackID string   `json:"trackId"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

func findDuplicateCandidates(tracks []TrackRecord, target TrackRecord, threshold float64) []DuplicateMatch {
	if threshold <= 0 {
		threshold = 0.72
	}
	if threshold > 1 {
		threshold = 1
	}
	matches := make([]DuplicateMatch, 0)
	for _, candidate := range tracks {
		if candidate.ID == "" || candidate.ID == target.ID {
			continue
		}
		score, reasons := duplicateScore(target, candidate)
		if score < threshold {
			continue
		}
		matches = append(matches, DuplicateMatch{TrackID: candidate.ID, Score: score, Reasons: reasons})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].TrackID < matches[j].TrackID
		}
		return matches[i].Score > matches[j].Score
	})
	return matches
}

func duplicateScore(left, right TrackRecord) (float64, []string) {
	title := normalizedDuplicateValue(left.Title) != "" && normalizedDuplicateValue(left.Title) == normalizedDuplicateValue(right.Title)
	artist := normalizedDuplicateValue(left.Artist) != "" && normalizedDuplicateValue(left.Artist) == normalizedDuplicateValue(right.Artist)
	album := normalizedDuplicateValue(left.Album) != "" && normalizedDuplicateValue(left.Album) == normalizedDuplicateValue(right.Album)
	duration := durationMatches(left.DurationSeconds, right.DurationSeconds)
	hash := strings.TrimSpace(left.Hash) != "" && strings.EqualFold(strings.TrimSpace(left.Hash), strings.TrimSpace(right.Hash))

	if hash {
		return 1, []string{"audio hash matches"}
	}
	points := 0.0
	reasons := make([]string, 0, 4)
	if title {
		points += 0.42
		reasons = append(reasons, "title matches")
	}
	if artist {
		points += 0.30
		reasons = append(reasons, "artist matches")
	}
	if album {
		points += 0.16
		reasons = append(reasons, "album matches")
	}
	if duration {
		points += 0.12
		reasons = append(reasons, "duration is within five seconds")
	}
	return points, reasons
}

func normalizedDuplicateValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case builder.Len() > 0:
			builder.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func durationMatches(left, right *int) bool {
	if left == nil || right == nil || *left <= 0 || *right <= 0 {
		return false
	}
	return absInt(*left-*right) <= 5
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func duplicateReasonSummary(match DuplicateMatch) string {
	if len(match.Reasons) == 0 {
		return "possible duplicate (no matching evidence recorded)"
	}
	return "possible duplicate: " + strings.Join(match.Reasons, ", ") + " (score " + strconv.FormatFloat(match.Score, 'f', 2, 64) + ")"
}

func canonicalURLDuplicateKeys(raw string) []string {
	normalized, err := validateHTTPURL(raw)
	if err != nil {
		return nil
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil
	}
	keys := []string{}
	if exact := strings.ToLower(normalized); exact != "" {
		keys = append(keys, "exact:"+exact)
	}
	if videoID := findVideoIDFromURL(parsed); videoID != "" {
		keys = append(keys, "youtube:video:"+strings.ToLower(videoID))
	}
	if playlistID := parsed.Query().Get("list"); playlistID != "" {
		keys = append(keys, "youtube:playlist:"+strings.ToLower(strings.TrimSpace(playlistID)))
	}
	return uniqueDuplicateStrings(keys)
}

func findVideoIDFromURL(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	host := strings.ToLower(parsed.Host)
	switch {
	case strings.Contains(host, "youtu.be"):
		id := strings.Trim(strings.TrimSpace(parsed.Path), "/")
		if idx := strings.Index(id, "/"); idx >= 0 {
			id = id[:idx]
		}
		return id
	case strings.Contains(host, "youtube.com"):
		if id := strings.TrimSpace(parsed.Query().Get("v")); id != "" {
			return id
		}
		if strings.HasPrefix(strings.ToLower(parsed.Path), "/shorts/") {
			return path.Base(parsed.Path)
		}
	}
	return ""
}

func uniqueDuplicateStrings(values []string) []string {
	seen := map[string]struct{}{}
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
	return out
}
