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

func StateDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "state", "mebot")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func ProjectFilePath(project string) string {
	return filepath.Join(StateDir(), fmt.Sprintf("%s.md", project))
}

func DiscoverProjects() []string {
	dir := StateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	var projects []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			name := strings.TrimSuffix(entry.Name(), ".md")
			projects = append(projects, name)
		}
	}
	return projects
}

func SerializeToMarkdown(articles []*Article) string {
	var sb strings.Builder

	sb.WriteString("# Full Text\n\n")
	for _, art := range articles {
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", strings.TrimSpace(art.Title), strings.TrimSpace(art.FullText)))
	}

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

func ParseMarkdownRollup(content string) ([]*Article, error) {
	// Strip UTF-8 BOM and normalize Windows CR+LF to LF
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")
	content = strings.ReplaceAll(content, "\r\n", "\n")

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
			if _, exists := fullTexts[currentTitle]; !exists {
				titleOrder = append(titleOrder, currentTitle)
			}
			fullTexts[currentTitle] = body
		} else if currentSection == inSummary {
			summaries[currentTitle] = body
		}
		currentBody.Reset()
		currentTitle = ""
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		// Flexible header checks
		if lower == "# full text" || lower == "# full text:" || lower == "# full texts" {
			flush()
			currentSection = inFullText
			continue
		} else if lower == "# summary" || lower == "# summary:" || lower == "# summaries" {
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

	// Fallback: If it's a single article markdown without rollup headers
	if len(titleOrder) == 0 {
		trimmedContent := strings.TrimSpace(content)
		if trimmedContent != "" {
			lines := strings.SplitN(trimmedContent, "\n", 2)
			title := strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
			body := ""
			if len(lines) > 1 {
				body = strings.TrimSpace(lines[1])
			}
			return []*Article{{
				Title:    title,
				FullText: body,
				Summary:  "",
				Selected: false,
			}}, nil
		}
		return nil, fmt.Errorf("markdown file is empty")
	}

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
	if project == "" {
		return nil
	}
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
