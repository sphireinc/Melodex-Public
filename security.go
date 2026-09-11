package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"melodex/internal/pathsecurity"
)

var browserSelectorPattern = regexp.MustCompile(`^[A-Za-z0-9._+\- ]+(?::[A-Za-z0-9._+\- ]+)?$`)
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)
var browserCookieAliases = map[string]string{
	"google chrome":    "chrome",
	"chrome":           "chrome",
	"chromium":         "chromium",
	"apple safari":     "safari",
	"safari":           "safari",
	"microsoft edge":   "edge",
	"edge":             "edge",
	"mozilla firefox":  "firefox",
	"firefox":          "firefox",
	"samsung internet": "samsung internet",
	"opera":            "opera",
	"brave":            "brave",
	"uc browser":       "uc browser",
	"vivaldi":          "vivaldi",
	"yandex browser":   "yandex browser",
	"whale":            "whale",
}

func validateHTTPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported url scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("url host is missing")
	}
	return parsed.String(), nil
}

func validateExecutablePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}
	info, err := os.Stat(trimmed)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("path %q is a directory", trimmed)
		}
		return filepath.Clean(trimmed), nil
	}
	if resolved, lookErr := exec.LookPath(trimmed); lookErr == nil {
		return filepath.Clean(resolved), nil
	}
	return "", err
}

func validateCookieFilePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("cookie file path is empty")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("cookie file path %q is a directory", trimmed)
	}
	return filepath.Clean(trimmed), nil
}

func validateBrowserCookieSelector(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("browser selector is empty")
	}
	if len(trimmed) > 128 {
		return "", errors.New("browser selector is too long")
	}
	if !browserSelectorPattern.MatchString(trimmed) {
		return "", fmt.Errorf("browser selector %q contains unsupported characters", trimmed)
	}
	return normalizeBrowserCookieSelector(trimmed), nil
}

func normalizeBrowserCookieSelector(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parts := strings.SplitN(trimmed, ":", 2)
	base := strings.ToLower(strings.TrimSpace(parts[0]))
	normalizedBase := browserCookieAliases[base]
	if normalizedBase == "" {
		normalizedBase = strings.TrimSpace(parts[0])
	}
	if len(parts) == 1 {
		return normalizedBase
	}
	profile := strings.TrimSpace(parts[1])
	if profile == "" {
		return normalizedBase
	}
	return normalizedBase + ":" + profile
}

func validateMediaReadPath(path string, allowedRoots []string) (string, error) {
	return validatePathWithinRoots(path, allowedRoots, false)
}

func validatePathWithinRoots(path string, allowedRoots []string, allowDirectory bool) (string, error) {
	return pathsecurity.ValidatePathWithinRoots(path, allowedRoots, allowDirectory)
}

func redactSensitiveText(raw string) string {
	if raw == "" {
		return raw
	}
	out := urlPattern.ReplaceAllStringFunc(raw, redactURLString)
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "authorization:"),
			strings.Contains(lower, "cookie:"),
			strings.Contains(lower, "set-cookie:"),
			strings.Contains(lower, "x-api-key:"):
			if idx := strings.Index(line, ":"); idx >= 0 {
				lines[i] = line[:idx+1] + " <redacted>"
			}
		}
	}
	return strings.Join(lines, "\n")
}
