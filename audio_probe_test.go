package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

const testMP3Base64 = "SUQzBAAAAAAAI1RTU0UAAAAPAAADTGF2ZjYyLjEyLjEwMQAAAAAAAAAAAAAA//tQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAASW5mbwAAAA8AAAAFAAAE5ABVVVVVVVVVVVVVVVVVVVVVVVVVf39/f39/f39/f39/f39/f39/f3+qqqqqqqqqqqqqqqqqqqqqqqqqqtXV1dXV1dXV1dXV1dXV1dXV//////////////////////////8AAAAATGF2YzYyLjI4AAAAAAAAAAAAAAAAJAMGAAAAAAAABOQn25l5AAAAAAAAAAAAAAAAAAAAAP/7UGQAAAGPB1PVMCAMAAANIKAAAQuArTc5xQAAAAA0gwAAAAAAWLlbSAEsSzOM4EAEAaEx+7b9wfPggCAIawf1AgCByXB8P4gBCDju/o8/ynn+U9/QABmAAhhBhAAAYbIxiH1JGGL8AvQx+fjdxRCoAOlBx8TywIbKY/DhhsOOd11tgGgCfACiCBu/AWECDaDb/iCArAUhVBt/8RJEPhWIR7/+KpEPh8Qj0e/1A0JQkDQAAAEaB2MAAJUpUmAcAuMAIAkB4ypVLzIOEtMJAFP/+1JkDwnxwwbJ73gACAAADSDgAAEG3CEUDftiQAAANIAAAAQwOAITANAWMBMBAwFwCkxjZ3To/bFawx+b/QkOYgJGZnpukkYk49pv+elm+WPeYlgUhwbOZ6imaHhmrqYyKrUl+AOAP57pGu6vfi/96Uq/7PoqrgxEWMUJTJUQ0yZMDLCADFxEBYxZ8H0MC4AgTPkjABzSnDaTTOCV7S0OfldWS8n+v6LfT6/Re79aRBiYiZscG7xpiXDtHBX1wb/A7hiahPnCMhniMZmfmZPBi//7UmQtjPHFB8WLf9CQAAANIAAAAQa4IRQN+2JAAAA0gAAABAsteWVwTb9n3dv0f16f/Z/q12V1IAxAXMVJDIkc0mcMDCCDTFi0G8xWQIBMCvAhDMFzCgjTGTbyDPhl6y6z38v9HkArnWIGUBKVVzAcAzAIKjC4djrbkTltGzI0WzD8DzAEGzBUETBMEVM7YEMCmzZZ3f/Z/uT/7/oVAAA+uk1usHg1GokAAAphAcS8Cjx2V9FFrRE/XQXKqgLHLj5aDkFhYKqrbwAT0U83Z5ht//tSZEyI8XYIxYt/0JAAAA0gAAABBtAdKbXQACAAADSCgAAECsHAXQsV0lTf+uOo0+3m5Upf1/f//+HV3uuoAkJNRqNVaX///9+GdtfftrjkQzWyrfl////8Ud+Ny+KWMMt0tLWpqb0y5QMCUJA1aECIRlQaRJJMxQCtBQnGES9VpIrLqp3Ba7Dt5UUgiKWaBp4NHpblTpU7BqDVZ2iDSg6qTEFNRTMuMTAwqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqr/+1JkcAAD9THUbmsABAAADSDAAAAGMDECPYSAEAAANIOAAASqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg=="

func writeTestMP3(t *testing.T, path string) {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(testMP3Base64)
	if err != nil {
		t.Fatalf("decode test MP3: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write test MP3 %s: %v", filepath.Base(path), err)
	}
}

func TestParseAudioProbeJSON(t *testing.T) {
	result, err := parseAudioProbeJSON([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"mp3","sample_rate":"44100","channels":2,"bit_rate":"128000"}]}`))
	if err != nil {
		t.Fatalf("parseAudioProbeJSON: %v", err)
	}
	if result.Codec != "mp3" || result.SampleRate == nil || *result.SampleRate != 44100 || result.Channels == nil || *result.Channels != 2 || result.Bitrate == nil || *result.Bitrate != 128000 {
		t.Fatalf("unexpected probe result: %+v", result)
	}
}

func TestParseAudioProbeJSONRejectsMissingOrInvalidAudio(t *testing.T) {
	for name, input := range map[string]string{
		"missing stream": `{"streams":[{"codec_type":"video","codec_name":"h264"}]}`,
		"malformed":      `{"streams":`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseAudioProbeJSON([]byte(input)); err == nil {
				t.Fatal("expected probe parsing error")
			}
		})
	}
}

func TestProbeAudioFileValidatesCanonicalMP3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.mp3")
	writeTestMP3(t, path)
	result, err := probeAudioFile(path, "")
	if err != nil {
		t.Fatalf("probeAudioFile: %v", err)
	}
	if result.Codec != "mp3" || result.SampleRate == nil || *result.SampleRate <= 0 || result.Channels == nil || *result.Channels <= 0 {
		t.Fatalf("unexpected canonical MP3 probe result: %+v", result)
	}
}

func TestProbeAudioFileRejectsMalformedMP3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.mp3")
	if err := os.WriteFile(path, []byte("not an MP3"), 0o600); err != nil {
		t.Fatalf("write corrupt MP3: %v", err)
	}
	if _, err := probeAudioFile(path, ""); err == nil {
		t.Fatal("expected malformed MP3 to be rejected before finalization")
	}
}
