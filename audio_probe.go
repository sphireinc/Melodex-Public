package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"melodex/internal/songstore"
)

const audioProbeTimeout = 20 * time.Second

type audioProbeResult struct {
	Codec      string
	Bitrate    *int
	SampleRate *int
	Channels   *int
	Fallback   bool
}

type ffprobeDocument struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
		BitRate    string `json:"bit_rate"`
	} `json:"streams"`
}

func findFFProbePath(ffmpegPath string) string {
	names := []string{"ffprobe"}
	if runtime.GOOS == "windows" {
		names = append(names, "ffprobe.exe")
	}
	if directory := strings.TrimSpace(filepath.Dir(ffmpegPath)); directory != "." && directory != "" {
		for _, name := range names {
			candidate := filepath.Join(directory, name)
			if resolved, err := validateExecutablePath(candidate); err == nil {
				return resolved
			}
		}
	}
	for _, name := range names {
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved
		}
	}
	return ""
}

func parseAudioProbeJSON(data []byte) (audioProbeResult, error) {
	var document ffprobeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return audioProbeResult{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	for _, stream := range document.Streams {
		if strings.TrimSpace(stream.CodecType) != "audio" {
			continue
		}
		result := audioProbeResult{Codec: strings.TrimSpace(stream.CodecName)}
		if sampleRate := parsePositiveProbeInt(stream.SampleRate); sampleRate != nil {
			result.SampleRate = sampleRate
		}
		if stream.Channels > 0 {
			channels := stream.Channels
			result.Channels = &channels
		}
		if bitrate := parsePositiveProbeInt(stream.BitRate); bitrate != nil {
			result.Bitrate = bitrate
		}
		return result, nil
	}
	return audioProbeResult{}, fmt.Errorf("ffprobe found no usable audio stream")
}

func parsePositiveProbeInt(value string) *int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return nil
	}
	return &parsed
}

func probeAudioWithDecoder(path string) (audioProbeResult, error) {
	file, streamer, format, err := openAudioDecoder(path)
	if err != nil {
		return audioProbeResult{}, fmt.Errorf("validate audio with decoder: %w", err)
	}
	defer file.Close()
	defer streamer.Close()
	if format.SampleRate <= 0 || format.NumChannels <= 0 {
		return audioProbeResult{}, fmt.Errorf("decoded audio has invalid format: sample rate %d, channels %d", format.SampleRate, format.NumChannels)
	}
	sampleRate := int(format.SampleRate)
	channels := format.NumChannels
	return audioProbeResult{
		Codec:      audioFormatFromPath(path),
		SampleRate: &sampleRate,
		Channels:   &channels,
		Fallback:   true,
	}, nil
}

func probeAudioFile(path, ffmpegPath string) (audioProbeResult, error) {
	if strings.TrimSpace(path) == "" {
		return audioProbeResult{}, fmt.Errorf("audio path is required")
	}
	probePath := findFFProbePath(ffmpegPath)
	if probePath == "" {
		result, err := probeAudioWithDecoder(path)
		if err == nil {
			logEvent("audio_probe_fallback", "audio_name", filepath.Base(path), "reason", "ffprobe_unavailable")
		}
		return result, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), audioProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, probePath,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type,codec_name,sample_rate,channels,bit_rate",
		"-of", "json",
		path,
	)
	stdout, stderr, err := runCommandCapture(ctx, cmd)
	if err != nil {
		detail := strings.TrimSpace(redactSensitiveText(string(stderr)))
		if detail == "" {
			detail = strings.TrimSpace(redactSensitiveText(string(stdout)))
		}
		if detail == "" {
			return audioProbeResult{}, fmt.Errorf("ffprobe failed: %w", err)
		}
		return audioProbeResult{}, fmt.Errorf("ffprobe failed: %w: %s", err, detail)
	}
	result, err := parseAudioProbeJSON(stdout)
	if err != nil {
		return audioProbeResult{}, err
	}
	return result, nil
}

func applyAudioProbe(metadata *songstore.SongMetadata, probe audioProbeResult) {
	if metadata == nil {
		return
	}
	if codec := strings.TrimSpace(probe.Codec); codec != "" {
		metadata.Audio.Codec = codec
	}
	if probe.Bitrate != nil {
		metadata.Audio.Bitrate = probe.Bitrate
	}
	if probe.SampleRate != nil {
		metadata.Audio.SampleRate = probe.SampleRate
	}
	if probe.Channels != nil {
		metadata.Audio.Channels = probe.Channels
	}
}
