package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaServerURLForValidatesRootsAndReusesTokens(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	mediaPath := filepath.Join(root, "Artist", "Album", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatalf("MkdirAll media dir: %v", err)
	}
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}
	artworkPath := filepath.Join(root, "Artist", "Album", "cover.png")
	if err := os.WriteFile(artworkPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("WriteFile artwork: %v", err)
	}

	server := newMediaServer([]string{root})
	server.baseURL = "http://127.0.0.1:12345"

	url1, mime1, err := server.URLFor(mediaPath)
	if err != nil {
		t.Fatalf("URLFor valid media: %v", err)
	}
	if mime1 != "video/mp4" {
		t.Fatalf("expected video/mp4 mime, got %q", mime1)
	}
	if !strings.Contains(url1, "/media/") {
		t.Fatalf("expected tokenized media URL, got %q", url1)
	}

	url2, mime2, err := server.URLFor(mediaPath)
	if err != nil {
		t.Fatalf("URLFor cached media: %v", err)
	}
	if url2 != url1 {
		t.Fatalf("expected stable token URL, got %q then %q", url1, url2)
	}
	if mime2 != mime1 {
		t.Fatalf("expected stable mime type, got %q then %q", mime1, mime2)
	}

	if _, _, err := server.URLFor(root); err == nil {
		t.Fatalf("expected directory path to be rejected")
	}

	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile outside: %v", err)
	}
	traversal := filepath.Join(root, "..", filepath.Base(outside))
	if _, _, err := server.URLFor(traversal); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}

	missing := filepath.Join(root, "missing.mp4")
	if _, _, err := server.URLFor(missing); err == nil {
		t.Fatalf("expected missing file to be rejected")
	}

	dataURL, err := server.ReadDataURL(artworkPath)
	if err != nil {
		t.Fatalf("ReadDataURL valid media: %v", err)
	}
	if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("expected base64 data url, got %q", dataURL)
	}
	if _, err := server.ReadDataURL(mediaPath); err == nil {
		t.Fatalf("expected video data URL to be rejected")
	}
}

func TestMediaServerURLForSupportsAllowedRootsAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(t.TempDir(), "AppData")
	incoming := filepath.Join(t.TempDir(), "Incoming")
	cache := filepath.Join(t.TempDir(), "Cache")
	for _, dir := range []string{root, appData, incoming, cache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
	}

	rootFile := filepath.Join(root, "root.mp4")
	appDataFile := filepath.Join(appData, "appdata.mp4")
	incomingFile := filepath.Join(incoming, "incoming.mp4")
	cacheFile := filepath.Join(cache, "cache.mp4")
	for _, path := range []string{rootFile, appDataFile, incomingFile, cacheFile} {
		if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}

	server := newMediaServer([]string{root, appData, incoming, cache})
	server.baseURL = "http://127.0.0.1:12345"
	for _, path := range []string{rootFile, appDataFile, incomingFile, cacheFile} {
		if _, _, err := server.URLFor(path); err != nil {
			t.Fatalf("URLFor allowed root file %s: %v", path, err)
		}
	}

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.mp4")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile outside: %v", err)
	}
	symlinkPath := filepath.Join(root, "linked.mp4")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := server.URLFor(symlinkPath); err == nil {
		t.Fatalf("expected symlink escape to be rejected")
	}

	largePath := filepath.Join(root, "large.png")
	if err := os.WriteFile(largePath, bytes.Repeat([]byte("a"), maxDataURLBytes+1), 0o644); err != nil {
		t.Fatalf("WriteFile large: %v", err)
	}
	if _, err := server.ReadDataURL(largePath); err == nil {
		t.Fatalf("expected large file to be rejected for data url")
	}
}

func TestMediaURLMetadataMatchesMediaInfoAndStreamHeaders(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	mediaPath := filepath.Join(root, "clip.webm")
	contents := []byte("0123456789")
	if err := os.WriteFile(mediaPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}

	app := &App{info: RootInfo{LibraryRoot: root}}
	defer func() {
		if err := app.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	}()

	urlInfo, err := app.MediaURLInfo(mediaPath)
	if err != nil {
		t.Fatalf("MediaURLInfo: %v", err)
	}
	if urlInfo.URL == "" || !strings.Contains(urlInfo.URL, "/media/") {
		t.Fatalf("expected opaque media URL, got %q", urlInfo.URL)
	}
	if urlInfo.MimeType != "video/webm" {
		t.Fatalf("expected video/webm, got %q", urlInfo.MimeType)
	}
	if urlInfo.ContentLength != int64(len(contents)) {
		t.Fatalf("expected content length %d, got %d", len(contents), urlInfo.ContentLength)
	}
	if !urlInfo.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("expected future token expiry, got %s", urlInfo.ExpiresAt)
	}

	full, err := app.MediaInfo(mediaPath)
	if err != nil {
		t.Fatalf("MediaInfo: %v", err)
	}
	if full.URL != urlInfo.URL || full.MimeType != urlInfo.MimeType || full.ContentLength != urlInfo.ContentLength || !full.ExpiresAt.Equal(urlInfo.ExpiresAt) {
		t.Fatalf("MediaURLInfo and MediaInfo metadata differ: url=%#v full=%#v", urlInfo, full)
	}
	if !full.Reachable || full.StatusCode != http.StatusOK {
		t.Fatalf("expected reachable media info, got %#v", full)
	}

	response, err := http.Get(urlInfo.URL)
	if err != nil {
		t.Fatalf("GET media URL: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); got != urlInfo.MimeType {
		t.Fatalf("GET content type = %q, want %q", got, urlInfo.MimeType)
	}
	if got := response.Header.Get("Content-Length"); got != "10" {
		t.Fatalf("GET content length = %q, want 10", got)
	}

	rangeRequest, err := http.NewRequest(http.MethodGet, urlInfo.URL, nil)
	if err != nil {
		t.Fatalf("new range request: %v", err)
	}
	rangeRequest.Header.Set("Range", "bytes=2-5")
	rangeResponse, err := http.DefaultClient.Do(rangeRequest)
	if err != nil {
		t.Fatalf("GET range: %v", err)
	}
	defer rangeResponse.Body.Close()
	if rangeResponse.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want %d", rangeResponse.StatusCode, http.StatusPartialContent)
	}
	if got := rangeResponse.Header.Get("Content-Type"); got != urlInfo.MimeType {
		t.Fatalf("range content type = %q, want %q", got, urlInfo.MimeType)
	}
	if got := rangeResponse.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("content range = %q, want bytes 2-5/10", got)
	}
}

func TestMediaServerInvalidatesTokensWhenAllowedRootsChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	mediaPath := filepath.Join(root, "clip.mp4")
	if err := os.WriteFile(mediaPath, []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}

	server := newMediaServer([]string{root})
	server.baseURL = "http://127.0.0.1:12345"
	mediaURL, _, err := server.URLFor(mediaPath)
	if err != nil {
		t.Fatalf("URLFor media: %v", err)
	}
	token := strings.TrimPrefix(mediaURL, "http://127.0.0.1:12345/media/")

	server.setAllowedRoots([]string{filepath.Join(t.TempDir(), "OtherMusic")})
	req := httptest.NewRequest(http.MethodGet, "/media/"+token, nil)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected old token to be invalidated, got status %d", res.Code)
	}
	if _, _, err := server.URLFor(mediaPath); err == nil {
		t.Fatalf("expected media path to be rejected after allowed roots change")
	}
}

func TestMediaServerServeHTTPSupportsGetHeadAndRange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	mediaPath := filepath.Join(root, "clip.mp4")
	if err := os.WriteFile(mediaPath, []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}

	server := newMediaServer([]string{root})
	server.baseURL = "http://127.0.0.1:12345"
	mediaURL, mimeType, err := server.URLFor(mediaPath)
	if err != nil {
		t.Fatalf("URLFor media: %v", err)
	}
	if mimeType != "video/mp4" {
		t.Fatalf("expected video/mp4 mime, got %q", mimeType)
	}
	token := strings.TrimPrefix(mediaURL, "http://127.0.0.1:12345/media/")

	headReq := httptest.NewRequest(http.MethodHead, "/media/"+token, nil)
	headRes := httptest.NewRecorder()
	server.ServeHTTP(headRes, headReq)
	if headRes.Code != http.StatusOK {
		t.Fatalf("expected HEAD status 200, got %d", headRes.Code)
	}
	if got := headRes.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("expected content type video/mp4, got %q", got)
	}
	if headRes.Body.Len() != 0 {
		t.Fatalf("expected empty body for HEAD, got %d bytes", headRes.Body.Len())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/media/"+token, nil)
	getRes := httptest.NewRecorder()
	server.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d", getRes.Code)
	}
	if got := getRes.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("expected Accept-Ranges bytes, got %q", got)
	}

	rangeReq := httptest.NewRequest(http.MethodGet, "/media/"+token, nil)
	rangeReq.Header.Set("Range", "bytes=0-3")
	rangeRes := httptest.NewRecorder()
	server.ServeHTTP(rangeRes, rangeReq)
	if rangeRes.Code != http.StatusPartialContent {
		t.Fatalf("expected partial content, got %d", rangeRes.Code)
	}
	if body := rangeRes.Body.String(); body == "" {
		t.Fatalf("expected partial range body")
	}
}

func TestMediaServerRejectsUnsupportedFilesAndRevalidatesTokens(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	textPath := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(textPath, []byte("not media"), 0o644); err != nil {
		t.Fatalf("WriteFile text: %v", err)
	}
	server := newMediaServer([]string{root})
	server.baseURL = "http://127.0.0.1:12345"
	if _, _, err := server.URLFor(textPath); err == nil {
		t.Fatal("expected unsupported file type to be rejected")
	}

	mediaPath := filepath.Join(root, "clip.mp4")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}
	mediaURL, _, err := server.URLFor(mediaPath)
	if err != nil {
		t.Fatalf("URLFor media: %v", err)
	}
	token := strings.TrimPrefix(mediaURL, server.baseURL+"/media/")
	if err := os.Remove(mediaPath); err != nil {
		t.Fatalf("remove media: %v", err)
	}
	if err := os.Symlink(textPath, mediaPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	res := httptest.NewRecorder()
	server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/media/"+token, nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected token revalidation to reject replacement symlink, got %d", res.Code)
	}
}

func TestMediaServerExpiresOpaqueTokens(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "clip.mp4")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}
	server := newMediaServer([]string{root})
	server.baseURL = "http://127.0.0.1:12345"
	mediaURL, _, err := server.URLFor(mediaPath)
	if err != nil {
		t.Fatalf("URLFor media: %v", err)
	}
	token := strings.TrimPrefix(mediaURL, server.baseURL+"/media/")
	server.mu.Lock()
	server.entries[token] = mediaEntry{path: mediaPath, name: "clip.mp4", mimeType: "video/mp4", expiresAt: time.Now().UTC().Add(-time.Minute)}
	server.mu.Unlock()
	res := httptest.NewRecorder()
	server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/media/"+token, nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected expired token to return 404, got %d", res.Code)
	}
}

func TestProbeMediaURLChecksReachability(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("IPv4 loopback unavailable: %v", err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("expected HEAD probe, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
	})}}
	server.Start()
	defer server.Close()

	probe, err := probeMediaURL(server.URL + "/media/test")
	if err != nil {
		t.Fatalf("probeMediaURL: %v", err)
	}
	if !probe.reachable {
		t.Fatalf("expected probe to be reachable")
	}
	if probe.statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", probe.statusCode)
	}
	if probe.contentType != "video/mp4" {
		t.Fatalf("expected content type video/mp4, got %q", probe.contentType)
	}
	if probe.contentLength != 10 {
		t.Fatalf("expected content length 10, got %d", probe.contentLength)
	}
}
