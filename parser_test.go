package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanExtractedText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "timestamp ticker and trailing boilerplate",
			in: "Oct. 24, 2026 10:00 am ET\n\n" +
				"Wall Street rested its optimism on earnings.\n" +
				"Analysts expect Nvidia NVDA 2.39%increase; up pointing triangle and Micron to lead growth.\n\n" +
				"Copyright ©2026 Example. All Rights Reserved.\n" +
				"Jane Doe is a reporter for Example.",
			want: "Wall Street rested its optimism on earnings.\n" +
				"Analysts expect Nvidia and Micron to lead growth.",
		},
		{
			name: "standalone timezone",
			in:   "ET\n\nThe article starts here.",
			want: "The article starts here.",
		},
		{
			name: "updated timezone",
			in:   "Updated ET\n\nThe article starts here.",
			want: "The article starts here.",
		},
		{
			name: "copyright early is kept",
			in:   "Copyright law changed in 2026.\n\n" + strings.Repeat("Article text. ", 40),
			want: "Copyright law changed in 2026.\n\n" + strings.TrimSpace(strings.Repeat("Article text. ", 40)),
		},
		{
			name: "plain text is unchanged",
			in:   "First paragraph.\n\nSecond paragraph.",
			want: "First paragraph.\n\nSecond paragraph.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanExtractedText(tt.in); got != tt.want {
				t.Errorf("cleanExtractedText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractArticleFromHTMLPrunesFurniture(t *testing.T) {
	body := strings.Repeat("This is a substantial article paragraph that gives the extractor enough text to work with. ", 6)
	page := `<html><head><title>Test Article</title></head><body><article>
<p>Oct. 24, 2026 10:00 am ET</p>
<p>` + body + `</p>
<p>Nvidia NVDA 2.39%increase; up pointing triangle and Micron account for much of the growth.</p>
<div class="ai2html_export"><div class="g-GFX"><p>Chart label that should not be article text</p></div></div>
<p>` + body + `</p>
<p>Copyright ©2026 Example. All Rights Reserved.</p>
<p class="PrintheadlineBlurb">Appeared in the print edition as 'Test'.</p>
<p>Jane Doe is a reporter for Example.</p>
</article></body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "article.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}

	art, err := ExtractArticleFromHTML(path)
	if err != nil {
		t.Fatal(err)
	}

	if art.Title != "Test Article" {
		t.Errorf("title = %q, want %q", art.Title, "Test Article")
	}
	for _, unwanted := range []string{
		"Chart label",
		"Copyright",
		"Appeared in the print edition",
		"Jane Doe",
		"pointing triangle",
		"Oct. 24, 2026",
	} {
		if strings.Contains(art.FullText, unwanted) {
			t.Errorf("FullText contains %q:\n%s", unwanted, art.FullText)
		}
	}
	if !strings.Contains(art.FullText, "Nvidia and Micron") {
		t.Errorf("FullText lost article text:\n%s", art.FullText)
	}
}
