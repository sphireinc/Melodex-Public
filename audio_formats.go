package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	audioFormatMP3  = "mp3"
	audioFormatWAV  = "wav"
	audioFormatFLAC = "flac"
	audioFormatOGG  = "ogg"
	audioFormatM4A  = "m4a"
	audioFormatAAC  = "aac"
	audioFormatOPUS = "opus"
	audioFormatWEBM = "webm"
)

func audioFormatFromPath(path string) string {
	if format := normalizeAudioFormat(filepath.Ext(path)); format != "" {
		return format
	}
	return audioFormatFromFile(path)
}

func mediaSourceFormatFromPath(path string) string {
	ext := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")))
	switch ext {
	case audioFormatMP3, audioFormatWAV, audioFormatFLAC, audioFormatOGG, audioFormatM4A, audioFormatAAC, audioFormatOPUS, audioFormatWEBM:
		return ext
	case "mp4", "mkv", "mov", "m4v":
		return ext
	}
	return audioFormatFromFile(path)
}

func audioFormatFromName(name string) string {
	return normalizeAudioFormat(filepath.Ext(name))
}

func normalizeAudioFormat(value string) string {
	value = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(value, ".")))
	switch value {
	case audioFormatMP3, audioFormatWAV, audioFormatFLAC, audioFormatOGG:
		return value
	case "oga":
		return audioFormatOGG
	case "wave":
		return audioFormatWAV
	case "fla":
		return audioFormatFLAC
	case audioFormatM4A, audioFormatAAC, audioFormatOPUS, audioFormatWEBM:
		return value
	default:
		return ""
	}
}

func audioFormatFromFile(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	header := make([]byte, 512)
	n, _ := file.Read(header)
	return audioFormatFromBytes(header[:n])
}

func audioFormatFromBytes(data []byte) string {
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return audioFormatWAV
	}
	if len(data) >= 4 && string(data[0:4]) == "fLaC" {
		return audioFormatFLAC
	}
	if len(data) >= 4 && string(data[0:4]) == "OggS" {
		return audioFormatOGG
	}
	switch http.DetectContentType(data) {
	case "audio/mpeg":
		return audioFormatMP3
	case "audio/wav", "audio/x-wav":
		return audioFormatWAV
	case "audio/flac", "audio/x-flac":
		return audioFormatFLAC
	case "audio/ogg", "application/ogg":
		return audioFormatOGG
	default:
		return ""
	}
}

func supportedDirectAudioFormat(value string) bool {
	switch normalizeAudioFormat(value) {
	case audioFormatMP3, audioFormatWAV, audioFormatFLAC, audioFormatOGG:
		return true
	default:
		return false
	}
}
