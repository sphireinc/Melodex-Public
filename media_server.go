package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxDataURLBytes = 8 * 1024 * 1024
const mediaTokenTTL = 24 * time.Hour

type mediaServer struct {
	mu           sync.RWMutex
	server       *http.Server
	listener     net.Listener
	baseURL      string
	allowedRoots []string
	entries      map[string]mediaEntry
	paths        map[string]string
}

type mediaEntry struct {
	path      string
	name      string
	mimeType  string
	modTime   time.Time
	expiresAt time.Time
}

type mediaResponseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *mediaResponseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *mediaResponseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += int64(n)
	return n, err
}

func (r *mediaResponseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

type mediaURLProbe struct {
	reachable     bool
	statusCode    int
	contentLength int64
	contentType   string
}

// mediaURLMetadata is the shared contract used when a local media URL is
// created. The public MediaURLInfo method exposes the same fields through the
// MediaFileInfo Wails model, while MediaURL remains a compatibility wrapper.
type mediaURLMetadata struct {
	URL       string
	MimeType  string
	Size      int64
	ExpiresAt time.Time
}

func newMediaServer(roots []string) *mediaServer {
	allowed := make([]string, 0, len(roots))
	for _, root := range roots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			allowed = append(allowed, normalizeMediaRoot(trimmed))
		}
	}
	return &mediaServer{
		allowedRoots: allowed,
		entries:      map[string]mediaEntry{},
		paths:        map[string]string{},
	}
}

func (m *mediaServer) setAllowedRoots(roots []string) {
	allowed := make([]string, 0, len(roots))
	for _, root := range roots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			allowed = append(allowed, normalizeMediaRoot(trimmed))
		}
	}
	m.mu.Lock()
	cleared := len(m.entries)
	m.allowedRoots = allowed
	m.entries = map[string]mediaEntry{}
	m.paths = map[string]string{}
	m.mu.Unlock()
	if cleared > 0 {
		logEvent("media_server_tokens_invalidated", "count", cleared)
	}
}

func normalizeMediaRoot(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(trimmed)
}

func normalizeMediaPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func pathWithinRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func mediaTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".m4v":
		return "video/x-m4v"
	case ".ogv", ".ogg":
		return "video/ogg"
	default:
		if mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); mimeType != "" {
			return mimeType
		}
		return "application/octet-stream"
	}
}

func supportedMediaPath(path string) bool {
	mimeType := mediaTypeForPath(path)
	return strings.HasPrefix(mimeType, "audio/") ||
		strings.HasPrefix(mimeType, "video/") ||
		strings.HasPrefix(mimeType, "image/")
}

func (m *mediaServer) start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	m.listener = listener
	m.baseURL = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	m.server = &http.Server{
		Handler:           m,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := m.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("media server stopped unexpectedly: %v", err)
		}
	}()
	return nil
}

func (m *mediaServer) shutdown(ctx context.Context) error {
	m.mu.Lock()
	server := m.server
	m.server = nil
	m.listener = nil
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (m *mediaServer) URLFor(path string) (string, string, error) {
	metadata, err := m.URLForMetadata(path)
	if err != nil {
		return "", "", err
	}
	return metadata.URL, metadata.MimeType, nil
}

func (m *mediaServer) URLForMetadata(path string) (mediaURLMetadata, error) {
	resolved, err := validateMediaReadPath(path, m.allowedRootsSnapshot())
	if err != nil {
		return mediaURLMetadata{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return mediaURLMetadata{}, err
	}
	if info.IsDir() {
		return mediaURLMetadata{}, errors.New("media path must be a file")
	}

	mimeType := mediaTypeForPath(resolved)
	if !supportedMediaPath(resolved) {
		return mediaURLMetadata{}, fmt.Errorf("unsupported media type for %s", filepath.Base(resolved))
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.baseURL == "" {
		return mediaURLMetadata{}, errors.New("media server is not ready")
	}
	if token, ok := m.paths[resolved]; ok {
		if entry, ok := m.entries[token]; ok && entry.path == resolved && now.Before(entry.expiresAt) {
			return mediaURLMetadata{
				URL:       fmt.Sprintf("%s/media/%s", m.baseURL, token),
				MimeType:  mimeType,
				Size:      info.Size(),
				ExpiresAt: entry.expiresAt,
			}, nil
		}
		delete(m.entries, token)
		delete(m.paths, resolved)
	}
	token := shortID()
	for {
		if _, exists := m.entries[token]; !exists {
			break
		}
		token = shortID()
	}
	m.entries[token] = mediaEntry{
		path:      resolved,
		name:      filepath.Base(resolved),
		mimeType:  mimeType,
		modTime:   info.ModTime(),
		expiresAt: now.Add(mediaTokenTTL),
	}
	m.paths[resolved] = token
	return mediaURLMetadata{
		URL:       fmt.Sprintf("%s/media/%s", m.baseURL, token),
		MimeType:  mimeType,
		Size:      info.Size(),
		ExpiresAt: now.Add(mediaTokenTTL),
	}, nil
}

func (m *mediaServer) allowedRootsSnapshot() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.allowedRoots...)
}

func (m *mediaServer) ReadDataURL(path string) (string, error) {
	resolved, err := validateMediaReadPath(path, m.allowedRootsSnapshot())
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.Size() > maxDataURLBytes {
		return "", fmt.Errorf("file too large for data url: %s (%d bytes)", resolved, info.Size())
	}
	if !isSmallAssetDataURLPath(resolved) {
		return "", fmt.Errorf("data urls are limited to image assets: %s", filepath.Base(resolved))
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	if len(data) > maxDataURLBytes {
		return "", fmt.Errorf("file too large for data url: %s (%d bytes)", resolved, len(data))
	}
	mimeType := mediaTypeForPath(resolved)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}

func isSmallAssetDataURLPath(path string) bool {
	return strings.HasPrefix(mediaTypeForPath(path), "image/")
}

func (m *mediaServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	correlationID := newCorrelationID()
	recorder := &mediaResponseRecorder{ResponseWriter: w}
	logEvent(
		"media_server_request",
		"correlation_id", correlationID,
		"method", r.Method,
		"route", mediaRouteForLog(r.URL.Path),
		"range", r.Header.Get("Range") != "",
	)
	defer func() {
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		logEvent(
			"media_server_response",
			"correlation_id", correlationID,
			"method", r.Method,
			"route", mediaRouteForLog(r.URL.Path),
			"status", status,
			"bytes", recorder.bytes,
			"range", r.Header.Get("Range") != "",
			"content_type", recorder.Header().Get("Content-Type"),
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}()
	m.serveHTTP(recorder, r)
}

func mediaRouteForLog(path string) string {
	if strings.HasPrefix(path, "/media/") {
		return "/media/<redacted>"
	}
	if path == "" {
		return "/"
	}
	return path
}

func (m *mediaServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Accept, Origin, Content-Type")
	w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range, Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/media/") {
		http.NotFound(w, r)
		return
	}
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/media/"), "/")
	if token == "" {
		http.NotFound(w, r)
		return
	}
	m.mu.RLock()
	entry, ok := m.entries[token]
	m.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !entry.expiresAt.IsZero() && !time.Now().UTC().Before(entry.expiresAt) {
		http.Error(w, "media token expired", http.StatusNotFound)
		return
	}
	resolved, err := validateMediaReadPath(entry.path, m.allowedRootsSnapshot())
	if err != nil || resolved != entry.path || !supportedMediaPath(resolved) {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.Error(w, "unable to open media file", http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "unable to stat media file", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", entry.mimeType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", entry.name))
	http.ServeContent(w, r, entry.name, info.ModTime(), file)
}

func probeMediaURL(url string) (mediaURLProbe, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return mediaURLProbe{}, err
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return mediaURLProbe{}, err
	}
	defer resp.Body.Close()
	return mediaURLProbe{
		reachable:     resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices,
		statusCode:    resp.StatusCode,
		contentLength: resp.ContentLength,
		contentType:   resp.Header.Get("Content-Type"),
	}, nil
}

func (a *App) ensureMediaServer() error {
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	if a.mediaServer != nil {
		return nil
	}
	server := newMediaServer([]string{
		a.info.LibraryRoot,
		a.info.AppDataDir,
		a.info.IncomingDir,
		a.info.CacheDir,
	})
	if err := server.start(); err != nil {
		return err
	}
	a.mediaServer = server
	return nil
}

func (a *App) MediaURL(path string) (string, error) {
	info, err := a.MediaURLInfo(path)
	if err != nil {
		return "", err
	}
	return info.URL, nil
}

// MediaURLInfo is the typed URL-creation contract. MediaURL is retained as a
// compatibility method for older clients, but all new callers can receive
// the URL, backend MIME type, file size, and token expiry together.
func (a *App) MediaURLInfo(path string) (MediaFileInfo, error) {
	if err := a.ensureMediaServer(); err != nil {
		logEvent("media_url_info_failed", "path", path, "error", err.Error())
		return MediaFileInfo{}, err
	}
	a.mediaMu.Lock()
	server := a.mediaServer
	a.mediaMu.Unlock()
	if server == nil {
		return MediaFileInfo{}, errors.New("media server unavailable")
	}
	metadata, err := server.URLForMetadata(path)
	if err != nil {
		logEvent("media_url_info_failed", "path", path, "error", err.Error())
		return MediaFileInfo{}, err
	}
	logEvent("media_url_created", "path", path, "url", metadata.URL, "mime_type", metadata.MimeType, "content_length", metadata.Size, "expires_at", metadata.ExpiresAt)
	return MediaFileInfo{
		URL:           metadata.URL,
		MimeType:      metadata.MimeType,
		ContentLength: metadata.Size,
		ExpiresAt:     metadata.ExpiresAt,
	}, nil
}

func (a *App) MediaInfo(path string) (MediaFileInfo, error) {
	if err := a.ensureMediaServer(); err != nil {
		logEvent("media_info_failed", "path", path, "error", err.Error())
		return MediaFileInfo{}, err
	}
	a.mediaMu.Lock()
	server := a.mediaServer
	a.mediaMu.Unlock()
	if server == nil {
		return MediaFileInfo{}, errors.New("media server unavailable")
	}
	info, err := a.MediaURLInfo(path)
	if err != nil {
		logEvent("media_info_failed", "path", path, "error", err.Error())
		return MediaFileInfo{}, err
	}
	probe, probeErr := probeMediaURL(info.URL)
	if probeErr != nil {
		logEvent("media_info_probe_failed", "path", path, "url", info.URL, "mime_type", info.MimeType, "error", probeErr.Error())
	} else {
		logEvent(
			"media_info_probe_completed",
			"path", path,
			"url", info.URL,
			"mime_type", info.MimeType,
			"status", probe.statusCode,
			"reachable", probe.reachable,
			"content_length", probe.contentLength,
			"content_type", probe.contentType,
		)
	}
	if probeErr == nil && probe.contentLength >= 0 && probe.contentLength != info.ContentLength {
		logEvent("media_info_size_mismatch", "path", path, "url", info.URL, "metadata_size", info.ContentLength, "response_size", probe.contentLength)
	}
	info.Reachable = probe.reachable
	info.StatusCode = probe.statusCode
	logEvent("media_info_created", "path", path, "url", info.URL, "mime_type", info.MimeType, "reachable", info.Reachable, "status", info.StatusCode, "content_length", info.ContentLength)
	return info, nil
}

func (a *App) ServiceShutdown() error {
	a.mediaMu.Lock()
	server := a.mediaServer
	a.mediaServer = nil
	a.mediaMu.Unlock()
	if server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.shutdown(shutdownCtx); err != nil {
		log.Printf("media server shutdown failed: %v", err)
		return err
	}
	return nil
}
