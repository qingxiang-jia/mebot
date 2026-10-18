package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	trafilatura "github.com/markusmobius/go-trafilatura/v2"
)

func ExtractArticleFromHTML(filePath string) (*Article, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	result, err := trafilatura.Extract(bytes.NewReader(data), trafilatura.Options{
		ExcludeComments: true,
		ExcludeTables:   false,
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
		FullText: result.ContentText,
		Summary:  "",
		Selected: false,
	}, nil
}

// GetLinuxSystemFonts queries fontconfig for installed fonts
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
func PickFiles(title string, multiple bool, filter string) ([]string, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--title=" + title}
		if multiple {
			args = append(args, "--multiple", "--separator=|")
		}
		if filter != "" {
			args = append(args, "--file-filter="+filter)
		}
		cmd := exec.Command("zenity", args...)
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		raw := strings.TrimSpace(string(out))
		if raw == "" {
			return nil, nil
		}
		return strings.Split(raw, "|"), nil
	}

	if _, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--getopenfilename", "."}
		if multiple {
			args = []string{"--getopenfilename", "--multiple", "--separate-output", "."}
		}
		cmd := exec.Command("kdialog", args...)
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return lines, nil
	}

	return nil, fmt.Errorf("no native file dialog found; please install zenity or kdialog")
}
