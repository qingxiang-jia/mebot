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
	
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		// Remove script and style tags to be safe, though specific selectors below help avoid them.
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

		// 3. Paragraphs (p[data-component="paragraph"])
		// If that specific attribute isn't found, we might fall back to 'section p'?
		// But for now, let's stick to the requirements and the observed HTML structure.
		// We'll also look for just 'p' inside 'section' if the data-component is missing,
		// to be more robust across different WSJ/Economist templates if they differ.
		// However, the prompt asked to "preserve paragraph structure", implying the generic
		// text extraction was losing it.
		
		// Let's try the specific selector first.
		paragraphs := s.Find("p[data-component=\"paragraph\"]")
		if paragraphs.Length() == 0 {
			// Fallback: sections often contain the text in WSJ/Economist
			paragraphs = s.Find("section p")
		}

		paragraphs.Each(func(j int, p *goquery.Selection) {
			text := strings.TrimSpace(p.Text())
			if text != "" {
				sb.WriteString(text)
				sb.WriteString("\n\n")
			}
		})
	})

	return strings.TrimSpace(sb.String()), nil
}
