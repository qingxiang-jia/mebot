package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
)

func TestBuildDictionaryPromptIncludesSummaryAndContract(t *testing.T) {
	prompt := BuildDictionaryPrompt("He decided to take stock of the situation.")

	for _, want := range []string{
		"SAME LANGUAGE as the summary",
		"Return only a JSON array",
		"He decided to take stock of the situation.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

func TestParseDictionaryEntriesToleratesFencesAndProse(t *testing.T) {
	answer := "Here you go:\n```json\n[{\"term\":\" take stock \",\"explanation\":\" review the situation \"},{\"term\":\"take stock\",\"explanation\":\"duplicate\"},{\"term\":\"\",\"explanation\":\"ignored\"}]\n```"

	entries := ParseDictionaryEntries(answer)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %#v", entries)
	}
	if entries[0].Term != "take stock" {
		t.Errorf("term = %q", entries[0].Term)
	}
	if entries[0].Explanation != "review the situation" {
		t.Errorf("explanation = %q", entries[0].Explanation)
	}
}

func TestFilterDictionaryEntriesKeepsOnlyTermsInSummary(t *testing.T) {
	entries := []DictionaryEntry{
		{Term: "take stock", Explanation: "review the situation"},
		{Term: "parliament", Explanation: "not in the summary"},
	}
	kept := FilterDictionaryEntries(entries, "He decided to take stock of the situation.")
	if len(kept) != 1 || kept[0].Term != "take stock" {
		t.Fatalf("kept = %#v", kept)
	}
}

func TestFindDictionaryRangesPrefersLongerTermsAndWholeWords(t *testing.T) {
	entries := []DictionaryEntry{
		{Term: "stock", Explanation: "shares"},
		{Term: "take stock", Explanation: "review"},
		{Term: "art", Explanation: "not in article"},
	}
	ranges := findDictionaryRanges("Take stock before reading the article.", entries)

	if len(ranges) != 1 {
		t.Fatalf("ranges = %#v, want only the phrase", ranges)
	}
	if got := ranges[0]; got.start != 0 || got.end != 10 {
		t.Fatalf("range = %#v, want [0,10)", got)
	}
}

func TestSplitDictionaryPiecesCutsRunsAtTermEdges(t *testing.T) {
	runs := []richtext.Run{
		{Text: "He decided to "},
		{Text: "take stock", Attributes: richtext.Attributes{Weight: 700}},
		{Text: " of it."},
	}
	entries := []DictionaryEntry{{Term: "take stock", Explanation: "review"}}
	ranges := findDictionaryRanges(paragraphText(richtext.Paragraph{Runs: runs}), entries)
	pieces := splitDictionaryPieces(runs, ranges, entries)

	var terms []string
	var plain strings.Builder
	for _, piece := range pieces {
		if piece.term != nil {
			terms = append(terms, piece.text)
		} else {
			plain.WriteString(piece.text)
		}
	}
	if strings.Join(terms, "|") != "take stock" {
		t.Errorf("term pieces = %#v", terms)
	}
	if plain.String() != "He decided to  of it." {
		t.Errorf("plain text = %q", plain.String())
	}
}

func TestDictionaryTermShowsExplanationOnHover(t *testing.T) {
	entries := []DictionaryEntry{{Term: "take stock", Explanation: "review the situation"}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Children(func() {
			renderDictionarySummary(c, "take stock", entries, MacOSLightTheme(), "Sans", 16)
		})
	}, 500, 200)

	r, ok := tt.Find("take stock")
	if !ok {
		t.Fatalf("the term is not drawn: %q", tt.Texts())
	}
	tt.Move(r.X+10, r.Y+r.H/2)
	tt.Advance(time.Second)
	tt.Frame()

	if !tt.HasText("review the situation") {
		t.Fatalf("hovering the term showed no explanation: %q", tt.Texts())
	}
}

func TestSummaryColumnHasAIDictionaryButtonAndAnnotatedPreview(t *testing.T) {
	a := &App{
		Articles: []*Article{{
			Title:   "One",
			Summary: "He decided to take stock of the situation.",
		}},
		ActiveIdx:           0,
		AISummaryArticle:    -1,
		AIDictionaryArticle: 0,
		AIDictionarySource:  "He decided to take stock of the situation.",
		AIDictionaryEntries: []DictionaryEntry{{
			Term:        "take stock",
			Explanation: "review the situation",
		}},
		ShowMarkdownRender: true,
		LocalLLMPhase:      "ready",
		Theme:              MacOSLightTheme(),
		mdOpts:             markdown.Options{ShareSelection: true},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderSummaryColumn(c) }, 800, 500)

	for _, want := range []string{"AI Summary", "AI Dictionary"} {
		if !tt.HasText(want) {
			t.Errorf("the summary column is missing %q: %q", want, tt.Texts())
		}
	}
	if !tt.HasText("take stock") {
		t.Errorf("the annotated preview is missing the term: %q", tt.Texts())
	}
}
