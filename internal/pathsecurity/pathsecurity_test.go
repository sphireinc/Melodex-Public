package pathsecurity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePathWithinRootsRejectsTraversalDirectoriesAndOutsideFiles(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write inside file: %v", err)
	}

	expected, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatalf("resolve inside file: %v", err)
	}
	if got, err := ValidatePathWithinRoots(inside, []string{root}, false); err != nil || got != expected {
		t.Fatalf("expected allowed file, got %q err=%v want %q", got, err, expected)
	}
	if _, err := ValidatePathWithinRoots(root, []string{root}, false); err == nil {
		t.Fatal("expected directory rejection for file-only validation")
	}
	if _, err := ValidatePathWithinRoots(filepath.Join(root, "..", filepath.Base(root), "inside.txt"), []string{root}, false); err != nil {
		t.Fatalf("clean traversal within root should remain allowed: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if _, err := ValidatePathWithinRoots(outside, []string{root}, false); err == nil {
		t.Fatal("expected outside path rejection")
	}
}

func TestValidatePathWithinRootsResolvesAllowedAndRejectsEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	allowedLink := filepath.Join(root, "allowed-link.txt")
	if err := os.Symlink(target, allowedLink); err != nil {
		t.Skipf("symlink unavailable on this runner: %v", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got, err := ValidatePathWithinRoots(allowedLink, []string{root}, false); err != nil || got != resolvedTarget {
		t.Fatalf("allowed symlink resolved to %q err=%v want %q", got, err, resolvedTarget)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	escapingLink := filepath.Join(root, "escaping-link.txt")
	if err := os.Symlink(outside, escapingLink); err != nil {
		t.Skipf("symlink unavailable for escaping-link case: %v", err)
	}
	if _, err := ValidatePathWithinRoots(escapingLink, []string{root}, false); err == nil {
		t.Fatal("expected symlink escaping the allowed root to be rejected")
	}
}
