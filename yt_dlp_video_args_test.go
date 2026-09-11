package main

import (
	"slices"
	"strings"
	"testing"
)

func TestYTDLPVideoArgsPreferBrowserCompatibleFormats(t *testing.T) {
	args := ytDLPVideoArgs("out/%(title)s.%(ext)s")
	joined := strings.Join(args, " ")

	if !slices.Contains(args, "--check-formats") {
		t.Fatalf("expected video args to check format availability")
	}
	if !strings.Contains(joined, "--merge-output-format mp4") {
		t.Fatalf("expected video args to merge into mp4, got %q", joined)
	}
	if !strings.Contains(joined, "vcodec^=avc1") {
		t.Fatalf("expected video args to prefer H.264 video, got %q", joined)
	}
	if !strings.Contains(joined, "acodec^=mp4a") {
		t.Fatalf("expected video args to prefer AAC audio, got %q", joined)
	}
	if strings.Contains(joined, "bv*+ba/b") {
		t.Fatalf("expected video args not to use unconstrained best-video selector, got %q", joined)
	}
}
