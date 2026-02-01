package extract

import (
	"fmt"
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// FromHTML extracts text from specific elements inside <article>.
// It formats <h1> as ## Title, the first <h2> as a paragraph,
// and preserves <p data-component="paragraph"> structure.
func FromHTML(r io.Reader) (string, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	// Helper to extract from a selection
	extractFrom := func(s *goquery.Selection) {
		// Remove script and style tags to be safe
		s.Find("script, style").Remove()

		// 1. Article Title (h1) -> ## Title
		title := strings.TrimSpace(s.Find("h1").First().Text())
		if title != "" {
			sb.WriteString(fmt.Sprintf("## %s\n\n", title))
		}

		// 2. Subtitle (first h2) -> Paragraph
		subtitle := strings.TrimSpace(s.Find("h2").First().Text())
		if subtitle != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", subtitle))
		}

		// 3. Paragraphs
		// Try specific selectors first, then fallback
		selectors := []string{
			"p[data-component=\"paragraph\"]",
			"section p",
			"article p",
			"div[class*=\"article\"] p",
			"p",
		}

		var paragraphs *goquery.Selection
		for _, selector := range selectors {
			paragraphs = s.Find(selector)
			if paragraphs.Length() > 0 {
				break
			}
		}

		if paragraphs != nil {
			paragraphs.Each(func(j int, p *goquery.Selection) {
				text := strings.TrimSpace(p.Text())
				if text != "" {
					// Avoid duplicates if selectors overlap
					sb.WriteString(text)
					sb.WriteString("\n\n")
				}
			})
		}
	}

	articles := doc.Find("article")
	if articles.Length() > 0 {
		articles.Each(func(i int, s *goquery.Selection) {
			extractFrom(s)
		})
	} else {
		// Fallback to the whole body if no article tag is found
		extractFrom(doc.Find("body"))
	}

	return strings.TrimSpace(sb.String()), nil
}
