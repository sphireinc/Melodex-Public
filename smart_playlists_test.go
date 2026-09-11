package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func smartPlaylistPredicate(field SmartPlaylistField, operator SmartPlaylistOperator, value string) SmartPlaylistRule {
	return SmartPlaylistRule{Predicate: &SmartPlaylistPredicate{Field: field, Operator: operator, Value: value}}
}

func smartPlaylistInPredicate(field SmartPlaylistField, values ...string) SmartPlaylistRule {
	return SmartPlaylistRule{Predicate: &SmartPlaylistPredicate{Field: field, Operator: SmartPlaylistOperatorIn, Values: values}}
}

func smartPlaylistTrack(id, artist, album, title, genre, year string, createdAt time.Time) TrackRecord {
	return TrackRecord{ID: id, Artist: artist, Album: album, Title: title, Genre: genre, Year: year, CreatedAt: createdAt}
}

func TestSmartPlaylistSerializationRoundTripAndVersion(t *testing.T) {
	definition := SmartPlaylistDefinition{
		ID:          "smart-alt-90s",
		Name:        "Alternative in the 90s",
		Description: "A local catalog rule",
		Rule: SmartPlaylistRule{All: []SmartPlaylistRule{
			smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorContains, "alternative"),
			smartPlaylistPredicate(SmartPlaylistFieldDecade, SmartPlaylistOperatorEquals, "1990"),
		}},
		Sort: SmartPlaylistSort{Field: SmartPlaylistSortYear, Descending: true},
	}

	data, err := MarshalSmartPlaylist(definition)
	if err != nil {
		t.Fatalf("marshal smart playlist: %v", err)
	}
	if !bytes.Contains(data, []byte(`"schemaVersion": 1`)) {
		t.Fatalf("serialized definition did not include schema version: %s", data)
	}
	decoded, err := UnmarshalSmartPlaylist(data)
	if err != nil {
		t.Fatalf("unmarshal smart playlist: %v", err)
	}
	if decoded.SchemaVersion != SmartPlaylistSchemaVersion {
		t.Fatalf("schema version = %d, want %d", decoded.SchemaVersion, SmartPlaylistSchemaVersion)
	}
	if decoded.ID != definition.ID || decoded.Name != definition.Name || decoded.Sort != definition.Sort {
		t.Fatalf("round trip changed definition: %#v", decoded)
	}

	var normalized map[string]any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("serialized JSON is invalid: %v", err)
	}
	if _, ok := normalized["rule"]; !ok {
		t.Fatal("serialized definition omitted rule")
	}

	unknownField := bytes.Replace(data, []byte(`"name": "Alternative in the 90s"`), []byte(`"name": "Alternative in the 90s", "future": true`), 1)
	if _, err := UnmarshalSmartPlaylist(unknownField); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
	unsupported := bytes.Replace(data, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 99`), 1)
	if _, err := UnmarshalSmartPlaylist(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported smart playlist schema version") {
		t.Fatalf("expected unsupported version rejection, got %v", err)
	}
}

func TestSmartPlaylistValidationRejectsAmbiguousAndMalformedRules(t *testing.T) {
	cases := []struct {
		name string
		def  SmartPlaylistDefinition
		want string
	}{
		{
			name: "missing identity",
			def:  SmartPlaylistDefinition{Rule: smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorEquals, "rock")},
			want: "id is required",
		},
		{
			name: "two branches",
			def: SmartPlaylistDefinition{ID: "x", Name: "x", Rule: SmartPlaylistRule{
				All:       []SmartPlaylistRule{smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorEquals, "rock")},
				Predicate: &SmartPlaylistPredicate{Field: SmartPlaylistFieldArtist, Operator: SmartPlaylistOperatorEquals, Value: "artist"},
			}},
			want: "exactly one",
		},
		{
			name: "invalid decade",
			def:  SmartPlaylistDefinition{ID: "x", Name: "x", Rule: smartPlaylistPredicate(SmartPlaylistFieldDecade, SmartPlaylistOperatorEquals, "1995")},
			want: "first year of a decade",
		},
		{
			name: "wrong operator shape",
			def: SmartPlaylistDefinition{ID: "x", Name: "x", Rule: SmartPlaylistRule{Predicate: &SmartPlaylistPredicate{
				Field: SmartPlaylistFieldGenre, Operator: SmartPlaylistOperatorIn, Value: "rock",
			}}},
			want: "requires values",
		},
		{
			name: "invalid date",
			def:  SmartPlaylistDefinition{ID: "x", Name: "x", Rule: smartPlaylistPredicate(SmartPlaylistFieldRecentlyPlayed, SmartPlaylistOperatorAfter, "yesterday")},
			want: "RFC3339",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.def.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %v, want text %q", err, tc.want)
			}
		})
	}
}

func TestSmartPlaylistMatchesDerivedFieldsAndExplainsFailure(t *testing.T) {
	createdAt := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	lastPlayed := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	duration := 240
	track := smartPlaylistTrack("track-1", "The Example", "Signals", "Signal Fire", "Alternative rock", "1997", createdAt)
	track.LyricsIncluded = true
	track.HasTimedLyrics = true
	track.VideoPath = "/library/signal-fire.mp4"
	track.DurationSeconds = &duration
	context := SmartPlaylistTrackContext{
		LastPlayedAt:            &lastPlayed,
		PlayCount:               4,
		Moods:                   []string{"focused", "energetic"},
		DuplicateCandidate:      true,
		VersionKey:              "studio",
		AlbumTrackCount:         7,
		ExpectedAlbumTrackCount: 10,
	}

	rule := SmartPlaylistRule{All: []SmartPlaylistRule{
		smartPlaylistPredicate(SmartPlaylistFieldDecade, SmartPlaylistOperatorEquals, "1990"),
		smartPlaylistInPredicate(SmartPlaylistFieldMood, "focused", "calm"),
		smartPlaylistPredicate(SmartPlaylistFieldConfidence, SmartPlaylistOperatorGreaterOrEqual, "medium"),
		smartPlaylistPredicate(SmartPlaylistFieldDuplicateCandidate, SmartPlaylistOperatorIsTrue, ""),
		smartPlaylistPredicate(SmartPlaylistFieldIncompleteAlbum, SmartPlaylistOperatorIsTrue, ""),
		smartPlaylistPredicate(SmartPlaylistFieldHasVideo, SmartPlaylistOperatorIsTrue, ""),
		smartPlaylistPredicate(SmartPlaylistFieldDurationSeconds, SmartPlaylistOperatorGreaterOrEqual, "180"),
	}}
	track.Confidence = "high"
	explanation, err := rule.Explain(track, context)
	if err != nil {
		t.Fatalf("Explain matching rule: %v", err)
	}
	if !explanation.Matched || len(explanation.Children) != 7 {
		t.Fatalf("matched = %v with %d children, want true with 7", explanation.Matched, len(explanation.Children))
	}
	for _, child := range explanation.Children {
		if !child.Matched {
			t.Fatalf("expected every child to match: %#v", child)
		}
		if child.Reason == "" || child.Actual == "" {
			t.Fatalf("expected explainable child: %#v", child)
		}
	}

	failed, err := smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorContains, "jazz").Explain(track, context)
	if err != nil {
		t.Fatalf("Explain non-matching rule: %v", err)
	}
	if failed.Matched || !strings.Contains(strings.ToLower(failed.Reason), "actual alternative rock") {
		t.Fatalf("unexpected failure explanation: %#v", failed)
	}

	neverPlayed, err := smartPlaylistPredicate(SmartPlaylistFieldNeverPlayed, SmartPlaylistOperatorIsTrue, "").Match(track, context)
	if err != nil {
		t.Fatalf("Match never-played rule: %v", err)
	}
	if neverPlayed {
		t.Fatal("track with play history matched never-played rule")
	}
}

func TestSmartPlaylistAnyAndNotGroups(t *testing.T) {
	track := smartPlaylistTrack("track", "Artist", "Album", "Title", "Rock", "2001", time.Time{})
	anyRule := SmartPlaylistRule{Any: []SmartPlaylistRule{
		smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorEquals, "jazz"),
		smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorEquals, "rock"),
	}}
	matched, err := anyRule.Match(track, SmartPlaylistTrackContext{})
	if err != nil || !matched {
		t.Fatalf("any rule matched = %v, err = %v; want true", matched, err)
	}
	notRule := SmartPlaylistRule{Not: &SmartPlaylistRule{Predicate: &SmartPlaylistPredicate{
		Field: SmartPlaylistFieldGenre, Operator: SmartPlaylistOperatorEquals, Value: "jazz",
	}}}
	matched, err = notRule.Match(track, SmartPlaylistTrackContext{})
	if err != nil || !matched {
		t.Fatalf("not rule matched = %v, err = %v; want true", matched, err)
	}
}

func TestSmartPlaylistEvaluationSortsStablyLimitsAndBuildsPlaylist(t *testing.T) {
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tracks := []TrackRecord{
		smartPlaylistTrack("b", "Artist", "Album", "Same", "Rock", "1991", created),
		smartPlaylistTrack("a", "Artist", "Album", "Same", "Rock", "1990", created),
		smartPlaylistTrack("c", "Artist", "Album", "Other", "Rock", "1992", created),
		smartPlaylistTrack("d", "Artist", "Album", "No", "Jazz", "1992", created),
	}
	definition := SmartPlaylistDefinition{
		ID:    "rock-playlist",
		Name:  "Rock",
		Rule:  smartPlaylistPredicate(SmartPlaylistFieldGenre, SmartPlaylistOperatorEquals, "rock"),
		Sort:  SmartPlaylistSort{Field: SmartPlaylistSortTitle},
		Limit: 2,
	}
	result, err := EvaluateSmartPlaylist(definition, tracks, nil)
	if err != nil {
		t.Fatalf("EvaluateSmartPlaylist: %v", err)
	}
	if result.TotalMatches != 3 || len(result.Matches) != 2 {
		t.Fatalf("total/matched count = %d/%d, want 3/2", result.TotalMatches, len(result.Matches))
	}
	if got, want := result.Playlist.TrackIDs, []string{"c", "b"}; !equalStrings(got, want) {
		t.Fatalf("playlist IDs = %#v, want %#v", got, want)
	}
	if result.Matches[0].Explanation.Reason == "" {
		t.Fatal("expected evaluation match to retain explanation")
	}

	// Equal title values retain the source order b before a. This is a stable
	// tie, not an accidental ID-based reorder.
	allMatches := result.Matches
	if allMatches[1].Track.ID != "b" {
		t.Fatalf("stable tie order changed: %#v", allMatches)
	}

	var sorted []SmartPlaylistMatch
	for _, track := range tracks[:3] {
		sorted = append(sorted, SmartPlaylistMatch{Track: track})
	}
	if err := SortSmartPlaylistMatches(sorted, SmartPlaylistSort{Field: SmartPlaylistSortYear, Descending: true}); err != nil {
		t.Fatalf("SortSmartPlaylistMatches: %v", err)
	}
	if got, want := []string{sorted[0].Track.ID, sorted[1].Track.ID, sorted[2].Track.ID}, []string{"c", "b", "a"}; !equalStrings(got, want) {
		t.Fatalf("descending year order = %#v, want %#v", got, want)
	}
}

func TestSmartPlaylistMissingMetadataAndDateRules(t *testing.T) {
	created := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	track := smartPlaylistTrack("missing", "Artist", "", "Title", "", "", created)
	if !SmartPlaylistMetadataMissing(track) {
		t.Fatal("expected incomplete track metadata to be detected")
	}
	missing, err := smartPlaylistPredicate(SmartPlaylistFieldMissingMetadata, SmartPlaylistOperatorIsTrue, "").Match(track, SmartPlaylistTrackContext{})
	if err != nil {
		t.Fatalf("missing metadata match: %v", err)
	}
	if !missing {
		t.Fatal("expected missing metadata rule to match")
	}

	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	recent, err := smartPlaylistPredicate(SmartPlaylistFieldRecentlyAdded, SmartPlaylistOperatorAfter, cutoff.Format(time.RFC3339)).Match(track, SmartPlaylistTrackContext{})
	if err != nil {
		t.Fatalf("recently-added match: %v", err)
	}
	if !recent {
		t.Fatal("expected recently-added rule to match")
	}

	never, err := smartPlaylistPredicate(SmartPlaylistFieldNeverPlayed, SmartPlaylistOperatorIsTrue, "").Match(track, SmartPlaylistTrackContext{})
	if err != nil {
		t.Fatalf("never-played match: %v", err)
	}
	if !never {
		t.Fatal("expected absent history to be treated as never played")
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
