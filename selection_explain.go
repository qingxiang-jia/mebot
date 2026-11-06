package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
)

const selectionExplanationDelay = 350 * time.Millisecond

// BuildSelectionExplanationPrompt asks for a brief explanation of the
// selected text as it is used in its sentence. The answer stays in the
// language of the text, so a bilingual reader is not sent back to English.
func BuildSelectionExplanationPrompt(selected, sentence string) string {
	var sb strings.Builder
	sb.WriteString("Explain the selected text in the context of the sentence where it appears.\n")
	sb.WriteString("Be very brief: reply with one short sentence of at most 20 words.\n")
	sb.WriteString("Answer in the same language as the sentence.\n")
	sb.WriteString("No preamble, no quotes, no formatting.\n\n")
	sb.WriteString("SELECTED TEXT:\n")
	sb.WriteString(strings.TrimSpace(selected))
	sb.WriteString("\n\nSENTENCE:\n")
	sb.WriteString(strings.TrimSpace(sentence))
	sb.WriteString("\n")
	return sb.String()
}

// selectedTextAndSentence returns the selected text and the sentence around
// its start. A selection across paragraphs keeps its first sentence as the
// context.
func selectedTextAndSentence(parts []ui.SelectedText) (selected, sentence string) {
	if len(parts) == 0 {
		return "", ""
	}

	var pieces []string
	for i, part := range parts {
		runes := []rune(part.Text)
		start, end := part.Start, part.End
		if start < 0 {
			start = 0
		}
		if end > len(runes) {
			end = len(runes)
		}
		if start >= end {
			continue
		}
		pieces = append(pieces, string(runes[start:end]))
		if i == 0 {
			sentence = sentenceAround(part.Text, start, end)
		}
	}

	selected = strings.TrimSpace(strings.Join(pieces, "\n"))
	if sentence == "" && len(parts) > 0 {
		sentence = sentenceAround(parts[0].Text, parts[0].Start, parts[0].End)
	}
	return selected, sentence
}

// sentenceAround returns the sentence holding the rune range [start, end).
// It ends at the usual sentence punctuation, including the full-width marks
// of CJK text, or at a line break.
func sentenceAround(text string, start, end int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	if end < start {
		end = start
	}
	if end > len(runes) {
		end = len(runes)
	}

	from := start
	for from > 0 && !sentenceBreak(runes[from-1]) {
		from--
	}
	to := end
	for to < len(runes) && !sentenceBreak(runes[to]) {
		to++
	}
	if to < len(runes) && sentenceBreak(runes[to]) {
		to++
	}
	return strings.TrimSpace(string(runes[from:to]))
}

func sentenceBreak(r rune) bool {
	switch r {
	case '.', '!', '?', '…', '。', '！', '？', '\n', '\r':
		return true
	}
	return false
}

func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// briefExplanation keeps a model answer to the short sentence the card asks
// for, even when the model adds a label or another sentence.
func briefExplanation(answer string) string {
	answer = strings.TrimSpace(stripCodeFences(answer))
	answer = strings.Trim(answer, "\"'“”")
	lower := strings.ToLower(answer)
	for _, prefix := range []string{"explanation:", "meaning:", "definition:"} {
		if strings.HasPrefix(lower, prefix) {
			answer = strings.TrimSpace(answer[len(prefix):])
			break
		}
	}

	runes := []rune(answer)
	sentences := 0
	for i, r := range runes {
		if !sentenceEnd(runes, i, r) {
			continue
		}
		sentences++
		if sentences == 2 {
			answer = string(runes[:i+1])
			break
		}
	}
	return strings.TrimSpace(clipRunes(answer, 220))
}

// sentenceEnd reports whether runes[i] ends a sentence. A period inside an
// abbreviation, as in "e.g.", does not.
func sentenceEnd(runes []rune, i int, r rune) bool {
	switch r {
	case '!', '?', '…', '。', '！', '？', '\n', '\r':
		return true
	case '.':
		return i+1 == len(runes) || unicode.IsSpace(runes[i+1])
	}
	return false
}

// observeSelection notices the selection in a pane and starts an explanation
// once the selection settles. It is called while the pane builds, so the
// card below it sees the new state in the same frame.
func (a *App) observeSelection(box ui.Element, source string) {
	selected, sentence := selectedTextAndSentence(box.SelectedTexts())
	if selected == "" {
		if a.SelectionSource == source {
			a.clearSelectionExplanation()
		}
		return
	}

	key := source + "\x00" + sentence + "\x00" + selected
	if key == a.selectionKey {
		a.maybeStartSelectionExplanation()
		return
	}

	a.clearSelectionExplanation()
	a.SelectionText = selected
	a.SelectionSentence = sentence
	a.SelectionSource = source
	a.selectionKey = key
	a.maybeStartSelectionExplanation()
}

func (a *App) maybeStartSelectionExplanation() {
	if a.selectionKey == "" || a.SelectionClosed || a.LocalLLMPhase != "ready" || a.SelectionRunning ||
		a.selectionExplainedKey == a.selectionKey || a.selectionExplainTimer != nil {
		return
	}

	key := a.selectionKey
	a.SelectionError = ""
	a.SelectionExplanation = ""
	a.selectionExplainTimer = time.AfterFunc(selectionExplanationDelay, func() {
		a.updateUI(func() {
			if a.selectionKey != key {
				return
			}
			a.selectionExplainTimer = nil
			if a.LocalLLMPhase != "ready" {
				return
			}
			a.startSelectionExplanation()
		})
	})
}

func (a *App) startSelectionExplanation() {
	if a.SelectionRunning || a.SelectionText == "" || a.LocalLLMPhase != "ready" {
		return
	}

	key := a.selectionKey
	selected := a.SelectionText
	sentence := a.SelectionSentence
	ctx, cancel := context.WithCancel(context.Background())
	a.selectionExplainCancel = cancel
	a.selectionExplainRunID++
	runID := a.selectionExplainRunID
	a.SelectionRunning = true
	a.SelectionError = ""
	a.SelectionExplanation = ""

	prompt := BuildSelectionExplanationPrompt(selected, sentence)
	go func() {
		result, err := StreamLocalSummary(ctx, prompt, nil)

		a.updateUI(func() {
			if a.selectionExplainRunID != runID {
				return
			}
			a.selectionExplainCancel = nil
			a.SelectionRunning = false
			a.selectionExplainedKey = key

			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				a.SelectionError = fmt.Sprintf("Could not explain the selection: %s", err)
				return
			}

			answer := briefExplanation(result.Answer)
			if answer == "" {
				a.SelectionError = "The local LLM returned no explanation."
				return
			}
			a.SelectionExplanation = answer
		})
	}()
}

// stopSelectionExplanation stops any pending or running lookup without
// forgetting the selected text, so a stale answer cannot land.
func (a *App) stopSelectionExplanation() {
	if a.selectionExplainTimer != nil {
		a.selectionExplainTimer.Stop()
		a.selectionExplainTimer = nil
	}
	if a.selectionExplainCancel != nil {
		a.selectionExplainCancel()
		a.selectionExplainCancel = nil
	}
	a.selectionExplainRunID++
	a.SelectionRunning = false
	a.SelectionExplanation = ""
	a.SelectionError = ""
}

func (a *App) clearSelectionExplanation() {
	a.stopSelectionExplanation()
	a.SelectionText = ""
	a.SelectionSentence = ""
	a.SelectionSource = ""
	a.SelectionClosed = false
	a.selectionKey = ""
	a.selectionExplainedKey = ""
}

// closeSelectionExplanation hides the card for the current selection. The
// selection stays as it is, and choosing another segment starts again.
func (a *App) closeSelectionExplanation() {
	a.stopSelectionExplanation()
	a.SelectionClosed = true
}

// renderSelectionExplanation draws the latest answer under the pane where
// the selection was made. Without a selection, it draws nothing.
func (a *App) renderSelectionExplanation(c *ui.Context, source string) {
	if a.SelectionSource != source || a.SelectionText == "" || a.SelectionClosed || a.LocalLLMPhase != "ready" {
		return
	}

	ui.Column(c).AlignItems(ui.Stretch).FillWidth().Gap(4).Padding(10).Radius(6).
		Background(a.Theme.SidebarBg).Border(1, a.Theme.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "“"+clipRunes(strings.TrimSpace(a.SelectionText), 120)+"”").
				Font(a.CurrentFontFamily()).FontSize(11).TextColor(a.Theme.TextMuted).Grow(1)

			// As the toast docs close a toast: a button without a look, an
			// icon, and a hover tint of its own.
			close := ui.ButtonBase(c).Label("Close explanation").Tooltip("Close explanation").
				Padding(4).Radius(6)
			if close.Hovered() {
				close.Background(a.Theme.HoverBg)
			}
			close.Children(func() {
				ui.Icon(c, IconClose).Size(12, 12).TextColor(a.Theme.TextMuted)
			})
			if close.Clicked() {
				a.closeSelectionExplanation()
			}
		})
		if a.SelectionClosed {
			return
		}

		switch {
		case a.SelectionError != "":
			ui.Text(c, a.SelectionError).Font(a.CurrentFontFamily()).FontSize(a.CurrentFontSize()).
				TextColor(ui.Hex("#ff3b30"))
		case a.SelectionExplanation != "":
			ui.Text(c, a.SelectionExplanation).Font(a.CurrentFontFamily()).FontSize(a.CurrentFontSize()).
				TextColor(a.Theme.Text)
		default:
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "Explaining...").
					FontSize(a.CurrentFontSize()).
					TextColor(a.Theme.TextMuted)
			})
		}
	})
}
