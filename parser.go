package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	trafilatura "github.com/markusmobius/go-trafilatura/v2"
)

// articlePruneSelector removes page furniture that the extractor otherwise
// mistakes for article text. AI2HTML is used by many news sites for
// infographics; its absolutely positioned labels come through as stray
// paragraphs. Printheadline is the print-edition blurb some sites add.
const articlePruneSelector = `.ai2html_export, [class*=ai2html], [class*=g-GFX], [class*=g-aiPointText], [class*=Printheadline], [class*=printheadline]`

var (
	// A leading "ET", "Updated ET" or "Oct. 24, 2026 10:00 am ET" is a
	// timestamp fragment, not the start of the article.
	leadingTimestamp = regexp.MustCompile(`(?i)^(?:updated\s+)?(?:[a-z]{3}\.?\s+\d{1,2},\s+\d{4}(?:\s+\d{1,2}:\d{2}\s*[ap]m)?\s+)?(?:et|pt|ct|mt|utc|gmt|bst|cet|cest|ist|jst|aest)$`)

	// Inline stock widgets leave text such as
	// "NVDA 2.39%increase; up pointing triangle" in the article.
	inlineTicker = regexp.MustCompile(`\s+[A-Z][A-Z0-9.]{0,9}\s+[+-]?\d+(?:\.\d+)?%(?:increase|decrease);\s*(?:up|down) pointing triangle`)

	copyrightLine = regexp.MustCompile(`(?im)^[ \t]*copyright\b`)
)

func ExtractArticleFromHTML(filePath string) (*Article, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	result, err := trafilatura.Extract(bytes.NewReader(data), trafilatura.Options{
		ExcludeComments: true,
		ExcludeTables:   false,
		PruneSelector:   articlePruneSelector,
	})
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(result.Metadata.Title)
	if title == "" {
		base := filepath.Base(filePath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return &Article{
		Title:    title,
		FullText: cleanExtractedText(result.ContentText),
		Summary:  "",
		Selected: false,
	}, nil
}

// cleanExtractedText removes timestamps, inline stock-widget text and the
// trailing copyright/author-bio block that extraction keeps because it is
// part of the article's container.
func cleanExtractedText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")

	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && leadingTimestamp.MatchString(strings.TrimSpace(lines[0])) {
		lines = lines[1:]
	}
	for i, line := range lines {
		lines[i] = inlineTicker.ReplaceAllString(line, "")
	}
	text = strings.Join(lines, "\n")

	// Copyright notices sit at the end of the article, followed by the
	// print blurb and author bios. Only cut in the second half so a short
	// mention of copyright in the body is kept.
	if loc := copyrightLine.FindStringIndex(text); loc != nil && loc[0] > len(text)/2 {
		text = text[:loc[0]]
	}
	return strings.TrimSpace(text)
}

func GetLinuxSystemFonts() []string {
	cmd := exec.Command("fc-list", ":", "family")
	out, err := cmd.Output()
	if err != nil {
		return []string{"Serif", "Sans-Serif", "Monospace"}
	}

	seen := make(map[string]bool)
	var fonts []string
	for _, line := range strings.Split(string(out), "\n") {
		for _, fam := range strings.Split(line, ",") {
			fam = strings.TrimSpace(fam)
			if fam != "" && !seen[fam] && !strings.HasPrefix(fam, ".") {
				seen[fam] = true
				fonts = append(fonts, fam)
			}
		}
	}
	sort.Strings(fonts)
	if len(fonts) == 0 {
		return []string{"Serif", "Sans-Serif", "Monospace"}
	}
	return fonts
}

// PickFiles opens a native Linux file picker (zenity or kdialog)
func PickFiles(title string, multiple bool, patterns ...string) ([]string, error) {
	glob := strings.Join(patterns, " ")
	if glob == "" {
		glob = "*"
	}

	// 1. Try zenity (standard on Arch / CachyOS)
	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--title=" + title}
		if multiple {
			args = append(args, "--multiple", "--separator=|")
		}
		args = append(args, fmt.Sprintf("--file-filter=%s", glob))
		cmd := exec.Command("zenity", args...)
		out, err := cmd.Output()
		if err == nil {
			raw := strings.TrimSpace(string(out))
			if raw == "" {
				return nil, nil
			}
			return strings.Split(raw, "|"), nil
		}
	}

	// 2. Try kdialog (KDE)
	if _, err := exec.LookPath("kdialog"); err == nil {
		var args []string
		if multiple {
			args = []string{"--getopenfilename", "--multiple", "--separate-output", ".", glob}
		} else {
			args = []string{"--getopenfilename", ".", glob}
		}
		cmd := exec.Command("kdialog", args...)
		out, err := cmd.Output()
		if err == nil {
			var files []string
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					files = append(files, line)
				}
			}
			return files, nil
		}
	}

	return nil, fmt.Errorf("no native file picker found (please install zenity or kdialog)")
}
