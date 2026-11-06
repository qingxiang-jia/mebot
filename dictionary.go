package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

// DictionaryEntry is one rare word, phrase or idiom in a summary, with a
// short explanation in the summary's language. Entries are kept only in
// memory: they are never written to the project file.
type DictionaryEntry struct {
	Term        string `json:"term"`
	Explanation string `json:"explanation"`
}

// BuildDictionaryPrompt asks the local model for a JSON array of the terms
// in the summary that a reader may not know. The model is told to explain
// them in the language of the summary, which is how a bilingual reader gets
// explanations without translating the whole text.
func BuildDictionaryPrompt(summary string) string {
	var sb strings.Builder
	sb.WriteString("You are a dictionary assistant. Read the summary below and find rare words, idioms, and common phrases that a reader might not know.\n")
	sb.WriteString("For each one, write a short explanation in the SAME LANGUAGE as the summary, at most 25 words.\n")
	sb.WriteString("Only include expressions that appear exactly in the summary. Do not explain ordinary words. Do not add commentary.\n")
	sb.WriteString("Return only a JSON array whose objects have \"term\" and \"explanation\" keys. Do not wrap it in a code block.\n")
	sb.WriteString("Example: [{\"term\":\"take stock\",\"explanation\":\"to review a situation before deciding what to do next\"}]\n\n")
	sb.WriteString("SUMMARY:\n")
	sb.WriteString(strings.TrimSpace(summary))
	sb.WriteString("\n")
	return sb.String()
}

// ParseDictionaryEntries reads the model's JSON array. It tolerates a code
// fence or prose around the array because small local models sometimes add
// both despite the prompt.
func ParseDictionaryEntries(answer string) []DictionaryEntry {
	answer = stripCodeFences(answer)
	start := strings.Index(answer, "[")
	if start < 0 {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(answer[start:]))
	var raw []DictionaryEntry
	if err := dec.Decode(&raw); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	entries := make([]DictionaryEntry, 0, len(raw))
	for _, entry := range raw {
		term := normalizeDictionaryTerm(entry.Term)
		explanation := strings.Join(strings.Fields(entry.Explanation), " ")
		if term == "" || explanation == "" {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, DictionaryEntry{Term: term, Explanation: explanation})
	}
	return entries
}

// FilterDictionaryEntries keeps the entries whose term is actually in the
// summary, in the order the model returned them. The check stops the model
// from underlining words it invented or paraphrased.
func FilterDictionaryEntries(entries []DictionaryEntry, summary string) []DictionaryEntry {
	var kept []DictionaryEntry
	for _, entry := range entries {
		if !dictionaryContains(summary, entry.Term) {
			continue
		}
		kept = append(kept, entry)
		if len(kept) == 40 {
			break
		}
	}
	return kept
}

func normalizeDictionaryTerm(term string) string {
	term = strings.TrimSpace(term)
	term = strings.Trim(term, "\"'“”‘’`*_")
	return strings.Join(strings.Fields(term), " ")
}

func dictionaryContains(text, term string) bool {
	return len(findDictionaryRanges(text, []DictionaryEntry{{Term: term}})) > 0
}

// dictionaryRange is one occurrence of a term in text, in rune offsets, and
// the index of its entry.
type dictionaryRange struct {
	start, end int
	entry      int
}

// findDictionaryRanges locates every non-overlapping occurrence of every
// term. Longer terms win, so "take stock" is not also underlined as "stock".
func findDictionaryRanges(text string, entries []DictionaryEntry) []dictionaryRange {
	runes := []rune(text)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}

	type term struct {
		entry int
		runes []rune
		lower []rune
	}
	var terms []term
	for i, entry := range entries {
		termRunes := []rune(entry.Term)
		if len(termRunes) == 0 {
			continue
		}
		termLower := make([]rune, len(termRunes))
		for k, r := range termRunes {
			termLower[k] = unicode.ToLower(r)
		}
		terms = append(terms, term{entry: i, runes: termRunes, lower: termLower})
	}
	sort.SliceStable(terms, func(i, j int) bool {
		return len(terms[i].runes) > len(terms[j].runes)
	})

	claimed := make([]bool, len(runes))
	var ranges []dictionaryRange
	for _, t := range terms {
		for i := 0; i+len(t.runes) <= len(runes); {
			if !runesEqualAt(lower, t.lower, i) {
				i++
				continue
			}
			j := i + len(t.runes)
			if !termBoundary(runes, t.runes, i, j) {
				i++
				continue
			}
			overlap := false
			for k := i; k < j; k++ {
				if claimed[k] {
					overlap = true
					break
				}
			}
			if overlap {
				i++
				continue
			}
			for k := i; k < j; k++ {
				claimed[k] = true
			}
			ranges = append(ranges, dictionaryRange{start: i, end: j, entry: t.entry})
			i = j
		}
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	return ranges
}

func runesEqualAt(text, term []rune, at int) bool {
	for i, r := range term {
		if text[at+i] != r {
			return false
		}
	}
	return true
}

// termBoundary reports whether the match at [start, end) starts and ends on
// word boundaries, so that "art" does not match the "art" in "article".
func termBoundary(text, term []rune, start, end int) bool {
	if isWordRune(term[0]) && start > 0 && isWordRune(text[start-1]) {
		return false
	}
	if isWordRune(term[len(term)-1]) && end < len(text) && isWordRune(text[end]) {
		return false
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// dictionaryTextStyle is the base style of a paragraph before its runs add
// their own bold, italic, code and so on.
type dictionaryTextStyle struct {
	Font   string
	Size   float32
	Weight int
	Color  ui.Color
	Accent ui.Color
}

// renderDictionarySummary draws summary as Markdown with its dictionary
// terms highlighted. Hovering a term shows its explanation. The renderer is
// a small subset of the Markdown view, enough for the headings, lists,
// quotes and emphasis the summaries use.
func renderDictionarySummary(c *ui.Context, summary string, entries []DictionaryEntry, theme MacOSTheme, font string, fontSize float32) {
	doc := richtext.ParseMarkdown(summary)
	numbers := make(map[int]int)

	ui.Column(c).AlignItems(ui.Stretch).FillWidth().Gap(fontSize * 0.55).Children(func() {
		for _, p := range doc.Paragraphs() {
			style := dictionaryTextStyle{Font: font, Size: fontSize, Color: theme.Text, Accent: theme.Accent}
			if p.Style.Quote {
				style.Color = theme.TextMuted
			}
			if p.Style.CodeBlock {
				style.Font = "monospace"
				style.Size = fontSize * 0.9
			}
			if p.Style.Heading > 0 {
				sizes := [6]float32{1.55, 1.35, 1.18, 1.05, 1, 0.95}
				style.Size = fontSize * sizes[p.Style.Heading-1]
				style.Weight = 600
			}

			marker := listMarker(p.Style, numbers)
			renderDictionaryParagraph(c, p, entries, theme, style, marker)
		}
	})
}

func listMarker(style richtext.ParagraphStyle, numbers map[int]int) string {
	switch style.List {
	case richtext.Bullets:
		return "•  "
	case richtext.Numbers:
		numbers[style.Indent]++
		return fmt.Sprintf("%d.  ", numbers[style.Indent])
	case richtext.Tasks:
		return "☐  "
	default:
		for indent := range numbers {
			if indent >= style.Indent {
				delete(numbers, indent)
			}
		}
		return ""
	}
}

func renderDictionaryParagraph(c *ui.Context, p richtext.Paragraph, entries []DictionaryEntry, theme MacOSTheme, style dictionaryTextStyle, marker string) {
	text := paragraphText(p)
	ranges := findDictionaryRanges(text, entries)
	pieces := splitDictionaryPieces(p.Runs, ranges, entries)

	container := ui.Column(c).AlignItems(ui.Stretch).FillWidth()
	if p.Style.Indent > 0 {
		container.PaddingStart(style.Size * 1.6 * float32(p.Style.Indent))
	}
	if p.Style.Quote {
		container.BorderStartWidth(3).BorderColor(theme.Border).PaddingStart(style.Size * 0.85)
	}
	if p.Style.CodeBlock {
		container.Background(theme.Text.Alpha(0.04)).Radius(6).Padding(style.Size * 0.5)
	}

	container.Children(func() {
		line := ui.RichText(c).FillWidth()
		if p.Style.Heading > 0 {
			line.LineHeight(1.25)
		}
		line.Children(func() {
			if marker != "" {
				markerStyle := style
				markerStyle.Color = theme.TextMuted
				markerStyle.Weight = 0
				ui.RichText(c, dictionarySpan(marker, richtext.Attributes{}, markerStyle))
			}
			for _, piece := range pieces {
				if piece.text == "" {
					continue
				}
				if piece.term == nil {
					ui.RichText(c, dictionarySpan(piece.text, piece.attrs, style))
					continue
				}
				renderDictionaryTerm(c, piece.text, piece.attrs, style, piece.term.Explanation, theme)
			}
		})
	})
}

// dictionaryPiece is a slice of a paragraph with the style of the run it
// came from, and the entry it belongs to when it is inside a term.
type dictionaryPiece struct {
	text  string
	attrs richtext.Attributes
	term  *DictionaryEntry
}

// splitDictionaryPieces cuts the runs at the edges of dictionary ranges so
// each piece is either fully a term or fully plain text.
func splitDictionaryPieces(runs []richtext.Run, ranges []dictionaryRange, entries []DictionaryEntry) []dictionaryPiece {
	var pieces []dictionaryPiece
	offset := 0
	rangeIndex := 0

	for _, run := range runs {
		runes := []rune(run.Text)
		i := 0
		for i < len(runes) {
			global := offset + i
			for rangeIndex < len(ranges) && ranges[rangeIndex].end <= global {
				rangeIndex++
			}

			end := offset + len(runes)
			var entry *DictionaryEntry
			if rangeIndex < len(ranges) && ranges[rangeIndex].start <= global {
				r := ranges[rangeIndex]
				entry = &entries[r.entry]
				if r.end < end {
					end = r.end
				}
			} else if rangeIndex < len(ranges) && ranges[rangeIndex].start < end {
				end = ranges[rangeIndex].start
			}

			pieces = append(pieces, dictionaryPiece{
				text:  string(runes[i : end-offset]),
				attrs: run.Attributes,
				term:  entry,
			})
			i = end - offset
		}
		offset += len(runes)
	}
	return pieces
}

func paragraphText(p richtext.Paragraph) string {
	var sb strings.Builder
	for _, run := range p.Runs {
		sb.WriteString(run.Text)
	}
	return sb.String()
}

// dictionarySpan styles a plain slice from its Markdown run attributes.
func dictionarySpan(text string, a richtext.Attributes, style dictionaryTextStyle) ui.Span {
	span := ui.Span{
		Text:          text,
		Font:          style.Font,
		Size:          style.Size,
		Weight:        style.Weight,
		Italic:        a.Italic,
		Underline:     a.Underline,
		Strikethrough: a.Strikethrough,
		Color:         style.Color,
	}
	if a.Font != "" {
		span.Font = a.Font
	}
	if a.Size > 0 {
		span.Size = a.Size
	}
	if a.Weight > 0 {
		span.Weight = a.Weight
	}
	if a.Color.A > 0 {
		span.Color = a.Color
	}
	if a.Background.A > 0 {
		span.Background = a.Background
		span.Radius = 3
	}
	if a.Code {
		span.Font = "monospace"
		span.Size = style.Size * 0.875
		span.Background = style.Color.Alpha(0.07)
		span.Radius = 3
	}
	if a.Link != "" {
		span.Color = style.Accent
		span.Underline = true
	}
	return span
}

// addDictionaryTooltip shows explanation above a term in a larger version of
// the theme's tooltip look.
func addDictionaryTooltip(c *ui.Context, anchor ui.Element, explanation string) {
	t := c.Theme()
	fill, text := t.Inverse, t.InverseText
	if fill == ui.Transparent {
		fill = t.Text
	}
	if text == ui.Transparent {
		text = t.Background
	}

	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		tip.MaxWidth(t.Space(100)).
			Padding(t.Space(1.5), t.Space(2.5)).
			Radius(t.Space(1.5)).
			Background(fill).
			TextColor(text).
			FontSize(t.FontSize+3).
			LineHeight(1.4).
			Shadow(0, 2, 10, 0, ui.RGBA(0, 0, 0, 0.2))
		tip.Children(func() {
			ui.Text(c, explanation)
		})
		tip.AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, t.Space(1.5), 0)
	})
}

// renderDictionaryTerm draws a term with the library's supported text
// highlight: a rounded background tint behind the span, as search results
// and Markdown's <mark> use. The parent text has no glyphs of its own; it
// only holds the tooltip and the highlighted span.
func renderDictionaryTerm(c *ui.Context, text string, a richtext.Attributes, style dictionaryTextStyle, explanation string, theme MacOSTheme) {
	parent := ui.Text(c, "")
	parent.Children(func() {
		span := dictionarySpan(text, a, style)
		span.Background = theme.Accent.Alpha(0.16)
		span.Radius = 3
		ui.RichText(c, span)
	})
	parent.Description(explanation)
	addDictionaryTooltip(c, parent, explanation)
}
