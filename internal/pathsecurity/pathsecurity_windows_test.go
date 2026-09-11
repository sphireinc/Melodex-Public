//go:build windows

package pathsecurity

import (
	"os"
	"path/filepath"
	"testing"
)

// Windows CI must prove that symlink behavior was exercised. The portable
// tests intentionally skip when a platform cannot create symlinks; this
// Windows-only test makes that limitation an explicit failure for the hosted
// Windows security job instead of a silent pass.
func TestWindowsSymlinkPolicyRequiresSymlinkSupport(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("inside"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	allowedLink := filepath.Join(root, "allowed-link.txt")
	if err := os.Symlink(target, allowedLink); err != nil {
		t.Fatalf("Windows symlink policy test could not create an allowed symlink; hosted CI must run with symlink support: %v", err)
	}
	if _, err := ValidatePathWithinRoots(allowedLink, []string{root}, false); err != nil {
		t.Fatalf("allowed Windows symlink was rejected: %v", err)
	}

	outsideRoot := t.TempDir()
	outside := filepath.Join(outsideRoot, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	escapingLink := filepath.Join(root, "escaping-link.txt")
	if err := os.Symlink(outside, escapingLink); err != nil {
		t.Fatalf("create escaping symlink: %v", err)
	}
	if _, err := ValidatePathWithinRoots(escapingLink, []string{root}, false); err == nil {
		t.Fatal("expected Windows symlink escaping the allowed root to be rejected")
	}
}
