package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Article struct {
	ID       string
	Title    string
	FullText string
	Summary  string
	Selected bool
}

// StateDir returns ~/.local/state/mebot
func StateDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "state", "mebot")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func ProjectFilePath(project string) string {
	return filepath.Join(StateDir(), fmt.Sprintf("%s.md", project))
}

// SerializeToMarkdown formats articles into the # Full Text and # Summary format
func SerializeToMarkdown(articles []*Article) string {
	var sb strings.Builder

	// 1. Full Text Section
	sb.WriteString("# Full Text\n\n")
	for _, art := range articles {
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", strings.TrimSpace(art.Title), strings.TrimSpace(art.FullText)))
	}

	// 2. Summary Section
	sb.WriteString("# Summary\n\n")
	for _, art := range articles {
		summary := strings.TrimSpace(art.Summary)
		if summary == "" {
			summary = "_No summary provided._"
		}
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", strings.TrimSpace(art.Title), summary))
	}

	return sb.String()
}

// ParseMarkdownRollup parses a markdown file with # Full Text and # Summary sections
func ParseMarkdownRollup(content string) ([]*Article, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	type sectionType int
	const (
		none sectionType = iota
		inFullText
		inSummary
	)

	currentSection := none
	currentTitle := ""
	var currentBody strings.Builder

	fullTexts := make(map[string]string)
	summaries := make(map[string]string)
	var titleOrder []string

	flush := func() {
		if currentTitle == "" {
			return
		}
		body := strings.TrimSpace(currentBody.String())
		if currentSection == inFullText {
			fullTexts[currentTitle] = body
			titleOrder = append(titleOrder, currentTitle)
		} else if currentSection == inSummary {
			summaries[currentTitle] = body
		}
		currentBody.Reset()
		currentTitle = ""
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "# Full Text" {
			flush()
			currentSection = inFullText
			continue
		} else if trimmed == "# Summary" {
			flush()
			currentSection = inSummary
			continue
		}

		if strings.HasPrefix(trimmed, "## ") {
			flush()
			currentTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			continue
		}

		if currentTitle != "" {
			currentBody.WriteString(line + "\n")
		}
	}
	flush()

	var articles []*Article
	for _, title := range titleOrder {
		articles = append(articles, &Article{
			Title:    title,
			FullText: fullTexts[title],
			Summary:  summaries[title],
			Selected: false,
		})
	}

	return articles, nil
}

func SaveProjectState(project string, articles []*Article) error {
	md := SerializeToMarkdown(articles)
	return os.WriteFile(ProjectFilePath(project), []byte(md), 0644)
}

func LoadProjectState(project string) ([]*Article, error) {
	data, err := os.ReadFile(ProjectFilePath(project))
	if err != nil {
		return nil, err
	}
	return ParseMarkdownRollup(string(data))
}

func RemoveProjectState(project string) error {
	return os.Remove(ProjectFilePath(project))
}
