package main

import "testing"

func TestFindDuplicateCandidatesUsesExplainableEvidence(t *testing.T) {
	duration := 214
	target := TrackRecord{ID: "target", Title: "The Song!", Artist: "The Artist", Album: "The Album", DurationSeconds: &duration}
	near := TrackRecord{ID: "near", Title: "the song", Artist: "the artist", Album: "the album", DurationSeconds: intPtr(216)}
	falsePositive := TrackRecord{ID: "different", Title: "The Song", Artist: "Other Artist", Album: "Other Album", DurationSeconds: intPtr(600)}

	matches := findDuplicateCandidates([]TrackRecord{target, near, falsePositive}, target, 0.72)
	if len(matches) != 1 || matches[0].TrackID != "near" {
		t.Fatalf("expected only near duplicate, got %#v", matches)
	}
	if len(matches[0].Reasons) < 3 {
		t.Fatalf("expected explainable reasons, got %#v", matches[0].Reasons)
	}
	if summary := duplicateReasonSummary(matches[0]); summary == "" {
		t.Fatal("expected duplicate reason summary")
	}
}

func TestFindDuplicateCandidatesTreatsMatchingAudioHashAsCertainSuggestion(t *testing.T) {
	target := TrackRecord{ID: "target", Hash: "abc123"}
	candidate := TrackRecord{ID: "candidate", Hash: "ABC123"}
	matches := findDuplicateCandidates([]TrackRecord{candidate}, target, 0.99)
	if len(matches) != 1 || matches[0].Score != 1 {
		t.Fatalf("expected hash match at score 1, got %#v", matches)
	}
}

func intPtr(value int) *int { return &value }
