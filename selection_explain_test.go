package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBuildSelectionExplanationPrompt(t *testing.T) {
	prompt := BuildSelectionExplanationPrompt("take stock", "He decided to take stock of the situation.")

	for _, want := range []string{
		"in the context of the sentence",
		"same language",
		"SELECTED TEXT:\ntake stock",
		"SENTENCE:\nHe decided to take stock of the situation.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

func TestSentenceAround(t *testing.T) {
	cases := []struct {
		text       string
		start, end int
		want       string
	}{
		{"He took stock. Then he left.", 3, 7, "He took stock."},
		{"This is one sentence without a full stop", 8, 12, "This is one sentence without a full stop"},
		{"这是一个测试。下一句。", 1, 3, "这是一个测试。"},
		{"First line\nSecond line", 12, 18, "Second line"},
	}
	for _, c := range cases {
		if got := sentenceAround(c.text, c.start, c.end); got != c.want {
			t.Errorf("sentenceAround(%q, %d, %d) = %q, want %q", c.text, c.start, c.end, got, c.want)
		}
	}
}

func TestSelectedTextAndSentence(t *testing.T) {
	selected, sentence := selectedTextAndSentence([]ui.SelectedText{{
		Text:  "He decided to take stock of the situation. Then he left.",
		Start: 14,
		End:   24,
	}})

	if selected != "take stock" {
		t.Errorf("selected = %q", selected)
	}
	if sentence != "He decided to take stock of the situation." {
		t.Errorf("sentence = %q", sentence)
	}
}

func TestBriefExplanationKeepsTheAnswerShort(t *testing.T) {
	answer := "Explanation: To review a situation before deciding, e.g. a business. This second sentence should not appear. This third one is cut too."
	got := briefExplanation(answer)
	if strings.HasPrefix(got, "Explanation:") {
		t.Errorf("the label was kept: %q", got)
	}
	if strings.Contains(got, "third") {
		t.Errorf("the third sentence was kept: %q", got)
	}
	if len([]rune(got)) > 220 {
		t.Errorf("the answer is too long: %d runes", len([]rune(got)))
	}
}

func TestSelectionExplanationCard(t *testing.T) {
	a := &App{
		SelectionText:        "take stock",
		SelectionSource:      "summary",
		SelectionExplanation: "to review the situation before deciding what to do next",
		LocalLLMPhase:        "ready",
		Theme:                MacOSLightTheme(),
		SelectedFontSize:     "16",
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Children(func() {
			a.renderSelectionExplanation(c, "summary")
		})
	}, 700, 300)

	for _, want := range []string{"take stock", "to review the situation"} {
		if !tt.HasText(want) {
			t.Errorf("the explanation card is missing %q: %q", want, tt.Texts())
		}
	}

	// The card belongs to the pane where the text was chosen.
	tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Children(func() {
			a.renderSelectionExplanation(c, "fulltext")
		})
	}, 700, 300)
	if tt.HasText("take stock") {
		t.Errorf("the summary card appeared in the full-text pane: %q", tt.Texts())
	}
}

func TestSelectionExplanationCardCanBeClosed(t *testing.T) {
	a := &App{
		SelectionText:        "take stock",
		SelectionSource:      "summary",
		SelectionExplanation: "to review the situation before deciding what to do next",
		LocalLLMPhase:        "ready",
		Theme:                MacOSLightTheme(),
		SelectedFontSize:     "16",
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Children(func() {
			a.renderSelectionExplanation(c, "summary")
		})
	}, 700, 300)

	if err := tt.Click("Close explanation"); err != nil {
		t.Fatal(err)
	}
	if !a.SelectionClosed {
		t.Fatal("the close button did not close the explanation")
	}
	tt.Frame()
	if tt.HasText("take stock") {
		t.Errorf("the card is still shown after closing: %q", tt.Texts())
	}
}

func TestFullTextSelectionIsObserved(t *testing.T) {
	const text = "He decided to take stock of the situation."
	a := &App{
		Articles:      []*Article{{Title: "One", FullText: text}},
		ActiveIdx:     0,
		LocalLLMPhase: "off",
		Theme:         MacOSLightTheme(),
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderFullTextColumn(c) }, 700, 500)

	if err := tt.Click(text); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	for i := 0; i < 5 && a.SelectionText == ""; i++ {
		tt.Frame()
	}

	if a.SelectionText != text {
		t.Fatalf("observed selection = %q, want %q", a.SelectionText, text)
	}
	if a.SelectionSource != "fulltext" {
		t.Errorf("selection source = %q, want fulltext", a.SelectionSource)
	}
}
