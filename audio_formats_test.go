package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAudioFormatDetectionAndDirectSupport(t *testing.T) {
	t.Parallel()

	if got := audioFormatFromBytes([]byte("RIFF\x00\x00\x00\x00WAVEfmt ")); got != audioFormatWAV {
		t.Fatalf("expected wav detection, got %q", got)
	}
	if got := audioFormatFromBytes([]byte("fLaC\x00\x00\x00\x22")); got != audioFormatFLAC {
		t.Fatalf("expected flac detection, got %q", got)
	}
	if got := audioFormatFromBytes([]byte("OggS\x00\x02\x00\x00")); got != audioFormatOGG {
		t.Fatalf("expected ogg detection, got %q", got)
	}
	if got := audioFormatFromName("track.mp3"); got != audioFormatMP3 {
		t.Fatalf("expected mp3 extension detection, got %q", got)
	}
	if got := audioFormatFromPath("/music/track.m4a"); got != audioFormatM4A {
		t.Fatalf("expected m4a path detection, got %q", got)
	}
	if got := mediaSourceFormatFromPath("/music/video.mp4"); got != "mp4" {
		t.Fatalf("expected mp4 media source detection, got %q", got)
	}
	if !supportedDirectAudioFormat("wav") || !supportedDirectAudioFormat("flac") || !supportedDirectAudioFormat("ogg") || !supportedDirectAudioFormat("mp3") {
		t.Fatalf("expected common direct audio formats to be supported")
	}
	if supportedDirectAudioFormat("m4a") {
		t.Fatalf("expected m4a to require transcode")
	}
}

func TestAudioFormatFromFileDetectsHeader(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "song.bin")
	if err := os.WriteFile(path, []byte("RIFF\x00\x00\x00\x00WAVEfmt "), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := audioFormatFromFile(path); got != audioFormatWAV {
		t.Fatalf("expected wav detection from file, got %q", got)
	}
}

func TestLocalAudioImportFormatInventory(t *testing.T) {
	cases := []struct {
		name          string
		extension     string
		sourceFormat  string
		directDecoder bool
	}{
		{name: "mp3", extension: ".mp3", sourceFormat: audioFormatMP3, directDecoder: true},
		{name: "m4a", extension: ".m4a", sourceFormat: audioFormatM4A},
		{name: "aac", extension: ".aac", sourceFormat: audioFormatAAC},
		{name: "flac", extension: ".flac", sourceFormat: audioFormatFLAC, directDecoder: true},
		{name: "wav", extension: ".wav", sourceFormat: audioFormatWAV, directDecoder: true},
		{name: "ogg", extension: ".ogg", sourceFormat: audioFormatOGG, directDecoder: true},
		{name: "opus", extension: ".opus", sourceFormat: audioFormatOPUS},
		{name: "webm", extension: ".webm", sourceFormat: audioFormatWEBM},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("music", "track"+tc.extension)
			if !supportedLocalImportFile(path) {
				t.Fatalf("%s is missing from the local import inventory", tc.extension)
			}
			if got := mediaSourceFormatFromPath(path); got != tc.sourceFormat {
				t.Fatalf("source format = %q, want %q", got, tc.sourceFormat)
			}
			if got := supportedDirectAudioFormat(tc.sourceFormat); got != tc.directDecoder {
				t.Fatalf("direct decoder support = %v, want %v", got, tc.directDecoder)
			}
		})
	}

	for _, extension := range []string{".txt", ".jpg", ".mkv", ".bin"} {
		if supportedLocalImportFile("music/track" + extension) {
			t.Fatalf("unsupported extension %s was accepted by the local import inventory", extension)
		}
	}
}

func TestPrepareLocalAudioSourceNormalizationPolicy(t *testing.T) {
	root := t.TempDir()
	stageDir := filepath.Join(root, "stage")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll stage: %v", err)
	}

	mp3Source := filepath.Join(root, "direct.mp3")
	mp3Data := []byte("not a decoded mp3, but sufficient for the copy policy")
	if err := os.WriteFile(mp3Source, mp3Data, 0o644); err != nil {
		t.Fatalf("WriteFile mp3 source: %v", err)
	}
	prepared, err := (&App{}).prepareLocalAudioSource(context.Background(), mp3Source, stageDir, "")
	if err != nil {
		t.Fatalf("MP3 preparation without ffmpeg: %v", err)
	}
	if prepared.Format != audioFormatMP3 || prepared.SourceFormat != audioFormatMP3 {
		t.Fatalf("MP3 preparation formats = %#v", prepared)
	}
	if got, err := os.ReadFile(prepared.Path); err != nil {
		t.Fatalf("ReadFile prepared MP3: %v", err)
	} else if string(got) != string(mp3Data) {
		t.Fatalf("direct MP3 preparation changed source bytes")
	}

	for _, extension := range []string{".m4a", ".aac", ".flac", ".wav", ".ogg", ".opus", ".webm"} {
		source := filepath.Join(root, "needs-ffmpeg"+extension)
		if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
			t.Fatalf("WriteFile %s source: %v", extension, err)
		}
		_, err := (&App{}).prepareLocalAudioSource(context.Background(), source, stageDir, "")
		if err == nil || !strings.Contains(err.Error(), "configure ffmpeg to transcode it to MP3") {
			t.Fatalf("preparing %s without ffmpeg returned %v; want explicit ffmpeg guidance", extension, err)
		}
	}
}

func TestLocalAudioNormalizationProducesDecodableMP3(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skipf("skipping real audio normalization integration test: ffmpeg is unavailable: %v", err)
	}

	root := t.TempDir()
	stageDir := filepath.Join(root, "stage")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll stage: %v", err)
	}

	cases := []struct {
		name       string
		extension  string
		sourceRate int
		codecArgs  []string
	}{
		{name: "mp3", extension: ".mp3", sourceRate: 44100, codecArgs: []string{"-codec:a", "libmp3lame", "-b:a", "96k"}},
		{name: "wav", extension: ".wav", sourceRate: 22050, codecArgs: []string{"-codec:a", "pcm_s16le", "-f", "wav"}},
		{name: "flac", extension: ".flac", sourceRate: 48000, codecArgs: []string{"-codec:a", "flac"}},
		{name: "ogg", extension: ".ogg", sourceRate: 32000, codecArgs: []string{"-strict", "-2", "-codec:a", "vorbis", "-q:a", "4", "-f", "ogg"}},
		{name: "m4a", extension: ".m4a", sourceRate: 44100, codecArgs: []string{"-codec:a", "aac", "-b:a", "96k", "-f", "ipod"}},
		{name: "aac", extension: ".aac", sourceRate: 22050, codecArgs: []string{"-codec:a", "aac", "-b:a", "96k", "-f", "adts"}},
		{name: "opus", extension: ".opus", sourceRate: 48000, codecArgs: []string{"-codec:a", "libopus", "-b:a", "64k", "-f", "ogg"}},
		{name: "webm", extension: ".webm", sourceRate: 24000, codecArgs: []string{"-codec:a", "libopus", "-b:a", "64k", "-f", "webm"}},
	}

	app := &App{settings: storedSettings{PublicSettings: PublicSettings{KeepOriginalAudio: true}}}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(root, fmt.Sprintf("fixture-%02d-%s%s", index, tc.name, tc.extension))
			generateRealAudioFixture(t, ffmpegPath, source, tc.sourceRate, tc.codecArgs)

			prepared, err := app.prepareLocalAudioSource(context.Background(), source, stageDir, ffmpegPath)
			if err != nil {
				t.Fatalf("prepareLocalAudioSource(%s): %v", tc.name, err)
			}
			if prepared.Format != audioFormatMP3 {
				t.Fatalf("normalized format = %q, want %q", prepared.Format, audioFormatMP3)
			}
			if prepared.SourceFormat != tc.name {
				t.Fatalf("source format = %q, want %q", prepared.SourceFormat, tc.name)
			}
			if filepath.Ext(prepared.Path) != ".mp3" {
				t.Fatalf("normalized path = %q, want .mp3 output", prepared.Path)
			}
			if info, err := os.Stat(prepared.Path); err != nil {
				t.Fatalf("stat normalized MP3: %v", err)
			} else if info.Size() == 0 {
				t.Fatalf("normalized MP3 is empty: %s", prepared.Path)
			}
			if tc.name != audioFormatMP3 {
				if prepared.OriginalStagePath == "" {
					t.Fatalf("expected retained original for %s", tc.name)
				}
				if _, err := os.Stat(prepared.OriginalStagePath); err != nil {
					t.Fatalf("stat retained %s original: %v", tc.name, err)
				}
			}

			file, streamer, format, err := openAudioDecoder(prepared.Path)
			if err != nil {
				t.Fatalf("openAudioDecoder(%s): %v", prepared.Path, err)
			}
			defer file.Close()
			defer streamer.Close()
			if format.SampleRate <= 0 {
				t.Fatalf("decoded sample rate = %d, want positive rate", format.SampleRate)
			}
			if streamer.Len() <= 0 {
				t.Fatalf("decoded stream length = %d, want positive length", streamer.Len())
			}

			buffer := make([][2]float64, 1024)
			decoded := 0
			for {
				n, ok := streamer.Stream(buffer)
				decoded += n
				if !ok {
					break
				}
				if n == 0 {
					t.Fatalf("decoder stalled after %d samples", decoded)
				}
			}
			if err := streamer.Err(); err != nil {
				t.Fatalf("decode normalized %s MP3: %v", tc.name, err)
			}
			if decoded == 0 {
				t.Fatalf("decoded normalized %s MP3 produced no samples", tc.name)
			}
		})
	}
}

func generateRealAudioFixture(t *testing.T, ffmpegPath, outputPath string, sampleRate int, codecArgs []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-nostdin",
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=0.25",
		"-ac", "2",
		"-ar", strconv.Itoa(sampleRate),
	}
	args = append(args, codecArgs...)
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stdout, stderr, err := runCommandCapture(ctx, cmd)
	if err != nil {
		output := strings.TrimSpace(string(append(stdout, stderr...)))
		t.Fatalf("generate fixture %s: %v: %s", filepath.Base(outputPath), err, output)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("generated fixture %s is missing: %v", filepath.Base(outputPath), err)
	}
}

func TestAudioFormatDetectionRejectsInvalidHeader(t *testing.T) {
	if got := audioFormatFromBytes([]byte("not an audio container")); got != "" {
		t.Fatalf("invalid audio header detected as %q", got)
	}
}
