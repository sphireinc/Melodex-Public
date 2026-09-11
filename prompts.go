package main

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:embed prompts/metadata_system_updated.md prompts/metadata_user_updated.md prompts/metadata_system_legacy.txt prompts/metadata_user_legacy.txt
var promptFS embed.FS

type PromptLibrary struct {
	files map[string]string
}

func loadPromptLibrary() (*PromptLibrary, error) {
	entries, err := fs.ReadDir(promptFS, "prompts")
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".txt" && ext != ".md" {
			continue
		}
		data, err := promptFS.ReadFile(filepath.Join("prompts", name))
		if err != nil {
			return nil, err
		}
		files[name] = string(data)
	}
	return &PromptLibrary{files: files}, nil
}

func (p *PromptLibrary) Info() []PromptInfo {
	names := make([]string, 0, len(p.files))
	for name := range p.files {
		names = append(names, name)
	}
	sort.Strings(names)
	infos := make([]PromptInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, PromptInfo{Name: name, Purpose: promptPurpose(name)})
	}
	return infos
}

func promptPurpose(name string) string {
	switch name {
	case "metadata_system_updated.md":
		return "Current metadata rules"
	case "metadata_user_updated.md":
		return "Current metadata input"
	case "metadata_system_legacy.txt":
		return "Legacy metadata rules"
	case "metadata_user_legacy.txt":
		return "Legacy metadata input"
	default:
		return "Embedded prompt"
	}
}

func (p *PromptLibrary) MetadataPrompt(templateData any) (string, string, error) {
	systemName, userName := "metadata_system_updated.md", "metadata_user_updated.md"
	system := strings.TrimSpace(firstPromptFile(p.files, systemName, "metadata_system_legacy.txt"))
	userTemplate := firstPromptFile(p.files, userName, "metadata_user_legacy.txt")
	if strings.TrimSpace(system) == "" || strings.TrimSpace(userTemplate) == "" {
		return "", "", fmt.Errorf("metadata prompt files are missing")
	}
	user, err := renderTemplate(userTemplate, templateData)
	if err != nil {
		return "", "", err
	}
	return system, strings.TrimSpace(user), nil
}

func (p *PromptLibrary) LyricsPrompt(templateData any) (string, string, error) {
	return "", "", fmt.Errorf("lyrics prompts are disabled; LRCLIB handles lyrics")
}

func renderTemplate(content string, data any) (string, error) {
	tpl, err := template.New("prompt").Delims("{{", "}}").Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse prompt: %w", err)
	}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return b.String(), nil
}

func firstPromptFile(files map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(files[name]); value != "" {
			return value
		}
	}
	return ""
}
