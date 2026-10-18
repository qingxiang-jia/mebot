package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	trafilatura "github.com/markusmobius/go-trafilatura/v2"
)

// ExtractArticleFromHTML reads an HTML file and extracts title & body
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

	// Trafilatura places extracted document metadata inside the .Metadata struct
	title := strings.TrimSpace(result.Metadata.Title)
	if title == "" {
		// Fallback to filename without extension
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
