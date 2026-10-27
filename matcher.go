package main

import (
	"strings"
	"unicode"
)

type ParsedSummaryBlock struct {
	RawTitle       string
	Body           string
	SelectedOption string // Title of assigned article or "[Do Not Match]"
}

const OptionDoNotMatch = "[Do Not Match]"

// IsSummaryEmpty checks if a summary is empty, whitespace, or invalid header artifacts
func IsSummaryEmpty(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\u00a0\u200b\ufeff\r\n\t ")
	// If it only contains an accidental dangling header like "## Article Title", treat as empty
	if strings.HasPrefix(s, "## ") && !strings.Contains(s, "\n") {
		return true
	}
	return s == ""
}

// ParsePastedSummaries splits markdown text by headings into title + summary blocks
func ParsePastedSummaries(raw string) []ParsedSummaryBlock {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")

	lines := strings.Split(raw, "\n")
	var blocks []ParsedSummaryBlock
	var currentTitle string
	var currentBody []string

	flush := func() {
		body := strings.TrimSpace(strings.Join(currentBody, "\n"))
		title := strings.TrimSpace(currentTitle)
		title = strings.Trim(title, "*_`# ")

		if idx := strings.Index(title, ". "); idx > 0 && idx < 4 {
			title = strings.TrimSpace(title[idx+2:])
		}
		title = strings.Trim(title, "*_`# ")

		if title != "" && body != "" {
			if !strings.EqualFold(title, "summary") && !strings.EqualFold(title, "summaries") {
				blocks = append(blocks, ParsedSummaryBlock{
					RawTitle:       title,
					Body:           body,
					SelectedOption: OptionDoNotMatch,
				})
			}
		}
		currentBody = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isHeading := false
		var headingText string

		if strings.HasPrefix(trimmed, "#") {
			isHeading = true
			headingText = strings.TrimLeft(trimmed, "# ")
		} else if strings.HasPrefix(trimmed, "**") && strings.HasSuffix(trimmed, "**") && len(trimmed) > 4 {
			isHeading = true
			headingText = strings.Trim(trimmed, "* ")
		} else if strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "===") {
			continue
		}

		if isHeading {
			flush()
			currentTitle = headingText
		} else {
			currentBody = append(currentBody, line)
		}
	}
	flush()

	// Fallback for single-summary pastes without headings
	if len(blocks) == 0 && strings.TrimSpace(raw) != "" {
		var firstLine string
		var rest []string
		for _, l := range lines {
			if firstLine == "" && strings.TrimSpace(l) != "" {
				firstLine = strings.Trim(strings.TrimSpace(l), "*_`# ")
			} else if firstLine != "" {
				rest = append(rest, l)
			}
		}
		body := strings.TrimSpace(strings.Join(rest, "\n"))
		if body == "" {
			body = firstLine
		}
		blocks = append(blocks, ParsedSummaryBlock{
			RawTitle:       firstLine,
			Body:           body,
			SelectedOption: OptionDoNotMatch,
		})
	}

	return blocks
}

func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)

	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := d[i-1][j] + 1
			ins := d[i][j-1] + 1
			sub := d[i-1][j-1] + cost

			minVal := del
			if ins < minVal {
				minVal = ins
			}
			if sub < minVal {
				minVal = sub
			}
			d[i][j] = minVal
		}
	}
	return d[la][lb]
}

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TitleSimilarity(s1, s2 string) float64 {
	n1 := normalizeTitle(s1)
	n2 := normalizeTitle(s2)
	if n1 == n2 {
		return 1.0
	}
	l1 := len([]rune(n1))
	l2 := len([]rune(n2))
	maxLen := l1
	if l2 > maxLen {
		maxLen = l2
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshtein(n1, n2)
	return 1.0 - (float64(dist) / float64(maxLen))
}

// ApplySummaryMatches writes the chosen summary bodies onto their articles.
// It returns the titles of the articles that were changed.
func ApplySummaryMatches(matches []ParsedSummaryBlock, articles []*Article) []string {
	articleMap := make(map[string]*Article)
	for _, art := range articles {
		articleMap[art.Title] = art
	}

	var applied []string
	appliedSet := make(map[string]bool)
	for _, block := range matches {
		if block.SelectedOption == OptionDoNotMatch {
			continue
		}
		if art, ok := articleMap[block.SelectedOption]; ok {
			art.Summary = block.Body
			if !appliedSet[art.Title] {
				appliedSet[art.Title] = true
				applied = append(applied, art.Title)
			}
		}
	}
	return applied
}

// MatchSummariesToArticles matches against empty summaries, or all articles if includeExisting is true
func MatchSummariesToArticles(summaries []ParsedSummaryBlock, articles []*Article, includeExisting bool) ([]ParsedSummaryBlock, []string) {
	var candidates []*Article
	var optionTitles []string
	optionTitles = append(optionTitles, OptionDoNotMatch)

	for _, art := range articles {
		if includeExisting || IsSummaryEmpty(art.Summary) {
			candidates = append(candidates, art)
			optionTitles = append(optionTitles, art.Title)
		}
	}

	if len(candidates) == 0 {
		return summaries, optionTitles
	}

	used := make(map[string]bool)

	for i := range summaries {
		bestScore := 0.0
		bestTitle := OptionDoNotMatch

		for _, art := range candidates {
			if used[art.Title] {
				continue
			}
			score := TitleSimilarity(summaries[i].RawTitle, art.Title)
			if score > bestScore {
				bestScore = score
				bestTitle = art.Title
			}
		}

		if bestScore >= 0.35 && bestTitle != OptionDoNotMatch {
			summaries[i].SelectedOption = bestTitle
			used[bestTitle] = true
		} else {
			summaries[i].SelectedOption = OptionDoNotMatch
		}
	}

	return summaries, optionTitles
}
