package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const recentLogBytesLimit = 256 * 1024

type recentLogBuffer struct {
	mu   sync.Mutex
	data []byte
	max  int
}

func newRecentLogBuffer(max int) *recentLogBuffer {
	return &recentLogBuffer{max: max}
}

func (b *recentLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.max <= 0 {
		return len(p), nil
	}
	if len(p) >= b.max {
		b.data = append(b.data[:0], p[len(p)-b.max:]...)
		return len(p), nil
	}
	if len(b.data)+len(p) > b.max {
		excess := len(b.data) + len(p) - b.max
		if excess >= len(b.data) {
			b.data = b.data[:0]
		} else {
			b.data = append(b.data[:0], b.data[excess:]...)
		}
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *recentLogBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

var recentLogs = newRecentLogBuffer(recentLogBytesLimit)

// CorrelationID ties together bounded diagnostic records for one operation.
// It is deliberately opaque so logs never need to expose a path, URL, or token
// as an identifier.
type CorrelationID string

var correlationSequence uint64

func newCorrelationID() CorrelationID {
	sequence := atomic.AddUint64(&correlationSequence, 1)
	return CorrelationID(fmt.Sprintf("corr-%s-%x", time.Now().UTC().Format("20060102150405.000000000"), sequence))
}

func configureLogging() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[melodex] ")
	log.SetOutput(io.MultiWriter(os.Stderr, recentLogs))

	if !envDebugEnabled(os.Getenv("MELODEX_DEBUG")) {
		return
	}

	logPath := strings.TrimSpace(os.Getenv("MELODEX_LOG_FILE"))
	if logPath == "" {
		log.Printf("debug logging enabled; writing to stderr")
		return
	}

	log.Printf("debug logging enabled; shell output is tee'd to %s", logPath)
}

func closeLogging() {
	// No-op. The shell launcher owns the log file tee.
}

func currentLogPath() string {
	return strings.TrimSpace(os.Getenv("MELODEX_LOG_FILE"))
}

func envDebugEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "debug":
		return true
	default:
		return false
	}
}

func logEvent(event string, kv ...any) {
	if !envDebugEnabled(os.Getenv("MELODEX_DEBUG")) {
		return
	}
	event = sanitizeLogFieldName(event)
	if event == "" {
		event = "unknown"
	}
	fields := []string{"event=" + event}
	for i := 0; i+1 < len(kv); i += 2 {
		key := sanitizeLogFieldName(fmt.Sprint(kv[i]))
		if key == "" {
			continue
		}
		fields = append(fields, key+"="+strconv.Quote(sanitizeLogFieldValue(key, strings.TrimSpace(fmt.Sprint(kv[i+1])))))
	}
	if len(kv)%2 != 0 {
		fields = append(fields, "missing_value="+strconv.Quote(sanitizeLogFieldValue("missing_value", strings.TrimSpace(fmt.Sprint(kv[len(kv)-1])))))
	}
	log.Print(strings.Join(fields, " "))
}

func sanitizeLogFieldValue(key, value string) string {
	if isSensitiveLogKey(key) {
		return "<redacted>"
	}
	if strings.Contains(value, "://") {
		return redactURLString(value)
	}
	return value
}

func isSensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(key, "api_key"),
		strings.Contains(key, "apikey"),
		strings.Contains(key, "cookie"),
		strings.Contains(key, "secret"),
		strings.Contains(key, "password"),
		strings.Contains(key, "auth"),
		strings.Contains(key, "token"):
		return true
	default:
		return false
	}
}

func redactURLString(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" {
		return raw
	}
	query := parsed.Query()
	for _, name := range []string{"token", "access_token", "refresh_token", "api_key", "apikey", "key", "auth", "signature", "sig"} {
		if query.Has(name) {
			query.Set(name, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) > 0 && strings.EqualFold(parts[0], "media") && len(parts) > 1 {
		parts[len(parts)-1] = "<redacted>"
		parsed.Path = "/" + strings.Join(parts, "/")
	}
	return parsed.String()
}

func sanitizeLogFieldName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"/", "_",
		"\\", "_",
		":", "_",
	)
	return replacer.Replace(value)
}
