package main

import (
	"fmt"
	"testing"

	"melodex/internal/songstore"
)

func TestBuildCoverArtQueriesPrefersAlbumThenTitle(t *testing.T) {
	queries := buildCoverArtQueries("Lacuna Coil", "Karmacode", "Enjoy the Silence")
	if len(queries) == 0 {
		t.Fatalf("expected queries")
	}
	if queries[0] != `artist:"Lacuna Coil" AND releasegroup:"Karmacode"` {
		t.Fatalf("unexpected first query: %q", queries[0])
	}
}

func TestBuildMetadataAIStatus(t *testing.T) {
	status, message := buildMetadataAIStatus(true, nil, songstore.MetadataResult{Confidence: "high"})
	if status != "success" || message != "Metadata discovered." {
		t.Fatalf("unexpected success outcome: %q %q", status, message)
	}

	status, message = buildMetadataAIStatus(false, nil, songstore.MetadataResult{Confidence: "low"})
	if status != "partial" || message != "Metadata discovery returned incomplete fields." {
		t.Fatalf("unexpected partial outcome: %q %q", status, message)
	}

	status, message = buildMetadataAIStatus(false, fmt.Errorf("boom"), songstore.MetadataResult{})
	if status != "failed" || message != "boom" {
		t.Fatalf("unexpected failed outcome: %q %q", status, message)
	}
}
