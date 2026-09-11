package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxRecordedCommandOutputBytes = 32 * 1024

type commandFailureRecord struct {
	Timestamp     time.Time     `json:"timestamp"`
	CorrelationID CorrelationID `json:"correlationId"`
	Command       string        `json:"command"`
	JobID         string        `json:"jobId,omitempty"`
	Error         string        `json:"error,omitempty"`
	Stdout        string        `json:"stdout,omitempty"`
	Stderr        string        `json:"stderr,omitempty"`
}

type recentCommandFailureBuffer struct {
	mu   sync.Mutex
	data []commandFailureRecord
	max  int
}

func newRecentCommandFailureBuffer(max int) *recentCommandFailureBuffer {
	return &recentCommandFailureBuffer{max: max}
}

func (b *recentCommandFailureBuffer) add(record commandFailureRecord) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.max <= 0 {
		return
	}
	if len(b.data) >= b.max {
		copy(b.data, b.data[1:])
		b.data = b.data[:b.max-1]
	}
	b.data = append(b.data, record)
}

func (b *recentCommandFailureBuffer) snapshot() []commandFailureRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]commandFailureRecord(nil), b.data...)
}

var recentCommandFailures = newRecentCommandFailureBuffer(32)

func commandLabel(cmd *exec.Cmd) string {
	if cmd == nil {
		return "command"
	}
	if trimmed := strings.TrimSpace(cmd.Path); trimmed != "" {
		return filepath.Base(trimmed)
	}
	if len(cmd.Args) > 0 {
		return filepath.Base(strings.TrimSpace(cmd.Args[0]))
	}
	return "command"
}

func trimRecordedCommandOutput(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	text := strings.TrimSpace(redactSensitiveText(string(raw)))
	if text == "" {
		return ""
	}
	if len(text) <= maxRecordedCommandOutputBytes {
		return text
	}
	return text[len(text)-maxRecordedCommandOutputBytes:]
}

func recordCommandFailure(cmd *exec.Cmd, jobID string, err error, stdout, stderr []byte) {
	recordCommandFailureWithCorrelation(cmd, jobID, newCorrelationID(), err, stdout, stderr)
}

func recordCommandFailureWithCorrelation(cmd *exec.Cmd, jobID string, correlationID CorrelationID, err error, stdout, stderr []byte) {
	if err == nil && len(stdout) == 0 && len(stderr) == 0 {
		return
	}
	if correlationID == "" {
		correlationID = newCorrelationID()
	}
	recentCommandFailures.add(commandFailureRecord{
		Timestamp:     time.Now().UTC(),
		CorrelationID: correlationID,
		Command:       commandLabel(cmd),
		JobID:         strings.TrimSpace(jobID),
		Error: func() string {
			if err == nil {
				return ""
			}
			return err.Error()
		}(),
		Stdout: trimRecordedCommandOutput(stdout),
		Stderr: trimRecordedCommandOutput(stderr),
	})
}

func recentCommandFailuresSnapshot() []commandFailureRecord {
	return recentCommandFailures.snapshot()
}
