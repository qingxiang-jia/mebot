package extract

import (
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// FromHTML extracts text from <section> elements inside <article> elements.
// This logic applies to both WSJ and The Economist as per requirements.
func FromHTML(r io.Reader) (string, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	doc.Find("article section").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
	})

	return strings.TrimSpace(sb.String()), nil
}
