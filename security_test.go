package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateHTTPURL(t *testing.T) {
	url, err := validateHTTPURL("  https://example.com/watch?v=abc123&token=secret  ")
	if err != nil {
		t.Fatalf("validateHTTPURL: %v", err)
	}
	if !strings.HasPrefix(url, "https://example.com/watch") {
		t.Fatalf("unexpected normalized url %q", url)
	}
	if _, err := validateHTTPURL("file:///tmp/x"); err == nil {
		t.Fatalf("expected non-http scheme to fail")
	}
}

func TestValidateBrowserCookieSelector(t *testing.T) {
	if got, err := validateBrowserCookieSelector("chrome:Profile 1"); err != nil || got != "chrome:Profile 1" {
		t.Fatalf("validateBrowserCookieSelector: got %q err=%v", got, err)
	}
	if _, err := validateBrowserCookieSelector("chrome;rm -rf /"); err == nil {
		t.Fatalf("expected invalid selector to fail")
	}
}

func TestWriteAtomicFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")
	if err := writeAtomicFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("writeAtomicFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content %q", string(data))
	}
}

func TestRedactSensitiveText(t *testing.T) {
	input := "https://example.com/watch?v=abc&token=secret cookie: session=abc Authorization: Bearer xyz"
	output := redactSensitiveText(input)
	if strings.Contains(output, "secret") || strings.Contains(output, "session=abc") || strings.Contains(output, "Bearer xyz") {
		t.Fatalf("expected secrets to be redacted, got %q", output)
	}
	if !strings.Contains(output, "<redacted>") {
		t.Fatalf("expected redaction marker, got %q", output)
	}
}

func TestValidatePathWithinRootsRejectsTraversalAndDirectories(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write inside file: %v", err)
	}
	expectedInside, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatalf("resolve inside file: %v", err)
	}
	if got, err := validatePathWithinRoots(inside, []string{root}, false); err != nil || got != expectedInside {
		t.Fatalf("expected allowed file, got %q err=%v", got, err)
	}
	if _, err := validatePathWithinRoots(root, []string{root}, false); err == nil {
		t.Fatal("expected directory rejection for file-only validation")
	}
	if _, err := validatePathWithinRoots(filepath.Join(root, "..", filepath.Base(root), "inside.txt"), []string{root}, false); err != nil {
		t.Fatalf("clean traversal within root should remain allowed: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("no"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if _, err := validatePathWithinRoots(outside, []string{root}, false); err == nil {
		t.Fatal("expected outside path rejection")
	}
}

func TestValidatePathWithinRootsResolvesAllowedAndEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	allowedLink := filepath.Join(root, "allowed-link.txt")
	if err := os.Symlink(target, allowedLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got, err := validatePathWithinRoots(allowedLink, []string{root}, false); err != nil || got != resolvedTarget {
		t.Fatalf("allowed symlink resolved to %q err=%v, want %q", got, err, resolvedTarget)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	escapingLink := filepath.Join(root, "escaping-link.txt")
	if err := os.Symlink(outside, escapingLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := validatePathWithinRoots(escapingLink, []string{root}, false); err == nil {
		t.Fatal("expected symlink escaping the allowed root to be rejected")
	}
}
