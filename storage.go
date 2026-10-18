package main

import (
	"os"
	"path/filepath"
	"strings"
)

func StateDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "state", "mebot")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func ProjectFilePath(projectName string) string {
	return filepath.Join(StateDir(), projectName+".md")
}

func DiscoverProjects() []string {
	dir := StateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var projects []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			name := strings.TrimSuffix(e.Name(), ".md")
			projects = append(projects, name)
		}
	}
	return projects
}

func SerializeToMarkdown(articles []*Article) string {
	var sb strings.Builder
	sb.WriteString("# Full Text\n\n")
	for _, art := range articles {
		sb.WriteString("## " + art.Title + "\n\n")
		sb.WriteString(strings.TrimSpace(art.FullText) + "\n\n")
	}

	sb.WriteString("# Summary\n\n")
	for _, art := range articles {
		sb.WriteString("## " + art.Title + "\n\n")
		if !IsSummaryEmpty(art.Summary) {
			sb.WriteString(strings.TrimSpace(art.Summary) + "\n\n")
		}
	}
	return sb.String()
}

func ParseMarkdownRollup(content string) ([]*Article, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	fullTextIdx := strings.Index(content, "# Full Text")
	summaryIdx := strings.Index(content, "# Summary")

	var fullTextPart string
	var summaryPart string

	if fullTextIdx != -1 && summaryIdx != -1 {
		if fullTextIdx < summaryIdx {
			fullTextPart = content[fullTextIdx+len("# Full Text") : summaryIdx]
			summaryPart = content[summaryIdx+len("# Summary"):]
		} else {
			summaryPart = content[summaryIdx+len("# Summary") : fullTextIdx]
			fullTextPart = content[fullTextIdx+len("# Full Text"):]
		}
	} else if fullTextIdx != -1 {
		fullTextPart = content[fullTextIdx+len("# Full Text"):]
	} else if summaryIdx != -1 {
		summaryPart = content[summaryIdx+len("# Summary"):]
	} else {
		fullTextPart = content
	}

	parseSections := func(part string) map[string]string {
		res := make(map[string]string)
		lines := strings.Split(part, "\n")
		var currentTitle string
		var currentBody []string

		flush := func() {
			if currentTitle != "" {
				res[currentTitle] = strings.TrimSpace(strings.Join(currentBody, "\n"))
			}
			currentBody = nil
		}

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") {
				flush()
				currentTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			} else if currentTitle != "" {
				currentBody = append(currentBody, line)
			}
		}
		flush()
		return res
	}

	fullTexts := parseSections(fullTextPart)
	summaries := parseSections(summaryPart)

	var articles []*Article
	lines := strings.Split(fullTextPart, "\n")
	seen := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if title != "" && !seen[title] {
				seen[title] = true
				articles = append(articles, &Article{
					Title:    title,
					FullText: fullTexts[title],
					Summary:  summaries[title],
				})
			}
		}
	}

	return articles, nil
}

func SaveProjectState(projectName string, articles []*Article) error {
	md := SerializeToMarkdown(articles)
	return os.WriteFile(ProjectFilePath(projectName), []byte(md), 0644)
}

func LoadProjectState(projectName string) ([]*Article, error) {
	data, err := os.ReadFile(ProjectFilePath(projectName))
	if err != nil {
		return nil, err
	}
	return ParseMarkdownRollup(string(data))
}

func RemoveProjectState(projectName string) error {
	return os.Remove(ProjectFilePath(projectName))
}
