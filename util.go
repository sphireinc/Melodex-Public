package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var unsafeSegment = regexp.MustCompile(`[^a-zA-Z0-9 .,_-]+`)

func shortID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(b[:])
}

func slugify(in string) string {
	in = strings.TrimSpace(in)
	in = unsafeSegment.ReplaceAllString(in, "-")
	in = strings.ReplaceAll(in, " ", "-")
	in = strings.Trim(in, "-_.")
	if in == "" {
		return "untitled"
	}
	return strings.ToLower(in)
}

func cleanDisplayName(in string) string {
	in = strings.TrimSpace(in)
	in = strings.Join(strings.Fields(in), " ")
	if in == "" {
		return "Unknown"
	}
	return in
}

func safeJoin(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = cleanDisplayName(part)
		part = strings.ReplaceAll(part, string(filepath.Separator), "-")
		cleaned = append(cleaned, part)
	}
	return filepath.Join(cleaned...)
}

func uniqueNonEmpty(values ...string) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return strings.Join(out, " ")
}

type localImportTarget struct {
	Path       string
	SourceRoot string
}

func collectLocalImportFiles(paths []string) ([]localImportTarget, error) {
	seen := map[string]struct{}{}
	out := make([]localImportTarget, 0)
	for _, input := range paths {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			root := filepath.Clean(input)
			if err := filepath.WalkDir(input, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					base := filepath.Base(path)
					if base != "." && strings.HasPrefix(base, ".") {
						return filepath.SkipDir
					}
					return nil
				}
				if !supportedLocalImportFile(entry.Name()) {
					return nil
				}
				cleaned := filepath.Clean(path)
				if _, ok := seen[cleaned]; ok {
					return nil
				}
				seen[cleaned] = struct{}{}
				out = append(out, localImportTarget{Path: cleaned, SourceRoot: root})
				return nil
			}); err != nil {
				return nil, err
			}
			continue
		}
		if !supportedLocalImportFile(info.Name()) {
			continue
		}
		cleaned := filepath.Clean(input)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, localImportTarget{Path: cleaned, SourceRoot: filepath.Dir(cleaned)})
	}
	if len(out) == 0 {
		return nil, errors.New("no supported audio files found")
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func supportedLocalImportFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".opus", ".webm":
		return true
	default:
		return false
	}
}
