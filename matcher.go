package main

import (
	"strings"
	"unicode"
)

type ParsedSummaryBlock struct {
	RawTitle       string
	Body           string
	SelectedOption string // Title of the assigned article or "[Do Not Match]"
}

const OptionDoNotMatch = "[Do Not Match]"

// ParsePastedSummaries splits markdown text by headings (#, ##, ###) into title + summary blocks
func ParsePastedSummaries(raw string) []ParsedSummaryBlock {
	lines := strings.Split(raw, "\n")
	var blocks []ParsedSummaryBlock
	var currentTitle string
	var currentBody []string

	flush := func() {
		body := strings.TrimSpace(strings.Join(currentBody, "\n"))
		title := strings.TrimSpace(currentTitle)
		// Clean leading markdown formatting like **Title** or *Title*
		title = strings.Trim(title, "*_`# ")
		if title != "" && body != "" {
			// Skip standalone top-level section headers like "# Summary"
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
		if strings.HasPrefix(trimmed, "#") {
			flush()
			currentTitle = strings.TrimLeft(trimmed, "# ")
		} else {
			currentBody = append(currentBody, line)
		}
	}
	flush()

	return blocks
}

// Levenshtein edit distance algorithm
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

// Normalized alphanumeric string for robust comparison
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

// TitleSimilarity returns a score between 0.0 (completely different) and 1.0 (exact match)
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

// MatchSummariesToUnmatched matches parsed summaries to articles that do NOT have a summary yet
func MatchSummariesToUnmatched(summaries []ParsedSummaryBlock, articles []*Article) ([]ParsedSummaryBlock, []string) {
	// 1. Gather candidate articles that have no summary
	var unmatched []*Article
	var optionTitles []string
	optionTitles = append(optionTitles, OptionDoNotMatch)

	for _, art := range articles {
		if strings.TrimSpace(art.Summary) == "" {
			unmatched = append(unmatched, art)
			optionTitles = append(optionTitles, art.Title)
		}
	}

	if len(unmatched) == 0 {
		return summaries, optionTitles
	}

	used := make(map[string]bool)

	// 2. Perform fuzzy matching using edit distance
	for i := range summaries {
		bestScore := 0.0
		bestTitle := OptionDoNotMatch

		for _, art := range unmatched {
			if used[art.Title] {
				continue
			}
			score := TitleSimilarity(summaries[i].RawTitle, art.Title)
			if score > bestScore {
				bestScore = score
				bestTitle = art.Title
			}
		}

		// Minimum similarity threshold (tolerates typos, punctuation, capitalization)
		if bestScore >= 0.45 && bestTitle != OptionDoNotMatch {
			summaries[i].SelectedOption = bestTitle
			used[bestTitle] = true
		} else {
			summaries[i].SelectedOption = OptionDoNotMatch
		}
	}

	return summaries, optionTitles
}
