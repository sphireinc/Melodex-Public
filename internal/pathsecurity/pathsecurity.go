// Package pathsecurity contains platform-independent filesystem boundary checks.
// It intentionally has no Wails or CGO dependencies so the same path policy can
// be tested on every supported desktop runner.
package pathsecurity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePathWithinRoots resolves path and verifies that it is a regular file
// or an allowed directory contained by one of the supplied roots. Symlink
// targets are evaluated before the boundary comparison so a link cannot escape
// an approved root.
func ValidatePathWithinRoots(path string, allowedRoots []string, allowDirectory bool) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() && !allowDirectory {
		return "", errors.New("path must be a file")
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", errors.New("path must be a regular file or directory")
	}

	for _, root := range allowedRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		rootResolved, rootErr := filepath.EvalSymlinks(root)
		if rootErr != nil {
			rootResolved = root
		}
		rootResolved, err = filepath.Abs(rootResolved)
		if err != nil {
			continue
		}
		if resolved == rootResolved || strings.HasPrefix(resolved, rootResolved+string(os.PathSeparator)) {
			return filepath.Clean(resolved), nil
		}
	}

	return "", fmt.Errorf("path %s is outside allowed roots", resolved)
}
