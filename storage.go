package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

var migratedStateDirs sync.Map

func StateDir() string {
	var base string
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		base = xdg
	} else {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "mebot")
	_ = os.MkdirAll(dir, 0755)

	// Automatic recovery: rescue any files written to ~/.local/share/mebot.
	// Run it once per state directory, not on every file save.
	if _, loaded := migratedStateDirs.LoadOrStore(dir, true); !loaded {
		migrateShareFolder(dir)
	}

	return dir
}

// migrateShareFolder moves the files of the pre-XDG location into dir. A
// rename across filesystems fails with EXDEV, so its fallback copies the
// file before removing the original.
func migrateShareFolder(dir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	shareDir := filepath.Join(home, ".local", "share", "mebot")
	entries, err := os.ReadDir(shareDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(shareDir, e.Name())
		dst := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			continue
		}
		if err := os.Rename(src, dst); err == nil {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := writeFileAtomic(dst, data, 0644); err == nil {
			_ = os.Remove(src)
		}
	}
}

func DefaultExportDir() string {
	home, _ := os.UserHomeDir()
	docs := filepath.Join(home, "Documents")
	_ = os.MkdirAll(docs, 0755)
	return docs
}

func ProjectFilePath(projectName string) string {
	return filepath.Join(StateDir(), projectName+".md")
}

// ValidateProjectName reports why name cannot be used as a project file
// name. A name with a slash would write outside the state directory, and a
// control character would break the settings file and the export name.
func ValidateProjectName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return errors.New("Enter a project name.")
	case name == "." || name == "..":
		return errors.New("That project name is reserved.")
	case strings.ContainsAny(name, `/\`):
		return errors.New("A project name cannot contain a slash.")
	case strings.ContainsRune(name, 0):
		return errors.New("A project name cannot contain a null character.")
	case strings.ContainsFunc(name, unicode.IsControl):
		return errors.New("A project name cannot contain control characters.")
	}
	return nil
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

	fullTextIdx := findHeaderLine(content, "# Full Text")
	summaryIdx := findHeaderLine(content, "# Summary")

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

	fullTexts := parseMarkdownSections(fullTextPart)
	summaries := parseMarkdownSections(summaryPart)

	// Pair the summaries with the articles in order. A project may hold two
	// articles with the same title, and a title-keyed map would collapse them
	// into one entry, losing one article's text on the next load.
	summaryQueues := make(map[string][]string, len(summaries))
	for _, s := range summaries {
		summaryQueues[s.Title] = append(summaryQueues[s.Title], s.Body)
	}

	articles := make([]*Article, 0, len(fullTexts))
	for _, s := range fullTexts {
		summary := ""
		if queue := summaryQueues[s.Title]; len(queue) > 0 {
			summary = queue[0]
			summaryQueues[s.Title] = queue[1:]
		}
		articles = append(articles, &Article{
			Title:    s.Title,
			FullText: s.Body,
			Summary:  summary,
		})
	}

	return articles, nil
}

// markdownSection is one "## Title" section of a rollup.
type markdownSection struct {
	Title string
	Body  string
}

// findHeaderLine returns the index of the header text on the line that holds
// it, or -1. A section header is its own line in the format this app writes,
// so text such as "the # Summary section" inside an article body does not
// split it.
func findHeaderLine(content, header string) int {
	for start := 0; start <= len(content); {
		end := strings.IndexByte(content[start:], '\n')
		if end < 0 {
			end = len(content)
		} else {
			end += start
		}
		line := content[start:end]
		if strings.TrimSpace(line) == header {
			return start + strings.Index(line, header)
		}
		if end == len(content) {
			break
		}
		start = end + 1
	}
	return -1
}

// parseMarkdownSections returns the "## " sections of part in order,
// keeping repeated titles instead of overwriting them.
func parseMarkdownSections(part string) []markdownSection {
	var sections []markdownSection
	var title string
	var body []string

	flush := func() {
		if title != "" {
			sections = append(sections, markdownSection{
				Title: title,
				Body:  strings.TrimSpace(strings.Join(body, "\n")),
			})
		}
		body = nil
	}

	for _, line := range strings.Split(part, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			title = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		} else if title != "" {
			body = append(body, line)
		}
	}
	flush()
	return sections
}

func SaveProjectState(projectName string, articles []*Article) error {
	md := SerializeToMarkdown(articles)
	return writeFileAtomic(ProjectFilePath(projectName), []byte(md), 0644)
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

// writeFileAtomic replaces path with data in one step, so a crash or a full
// disk while saving cannot truncate the project file the app is keeping.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if tmp != "" {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	tmp = ""

	// Rename is atomic, but a power loss right after it can still drop the
	// new directory entry until the directory itself is flushed.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
