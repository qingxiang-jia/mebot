package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestBuildSummaryPromptUsesDefaultInstruction(t *testing.T) {
	articles := []Article{
		{Title: "First Article", FullText: "First body."},
		{Title: "Second Article", FullText: "Second body."},
	}

	prompt := BuildSummaryPrompt("", articles)

	if !strings.Contains(prompt, DefaultGeminiPrompt) {
		t.Fatalf("prompt does not contain the default instruction")
	}
	for _, want := range []string{
		"TITLE: First Article",
		"First body.",
		"TITLE: Second Article",
		"Second body.",
		"two hash marks and the article's exact title",
		"Do not change, translate, or rephrase the title.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

func TestBuildSummaryPromptUsesCustomInstruction(t *testing.T) {
	prompt := BuildSummaryPrompt("Summarize briefly.", []Article{{Title: "A", FullText: "B"}})
	if !strings.Contains(prompt, "Summarize briefly.") {
		t.Fatalf("prompt does not contain the custom instruction")
	}
	if strings.Contains(prompt, DefaultGeminiPrompt) {
		t.Fatalf("prompt should not contain the default instruction when a custom one is set")
	}
}

func TestParseGeminiSSESeparatesThoughtsAndAnswer(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"Considering the key points.","thought":true}]}}]}`,
		``,
		`data: {"candidates":[{"content":{"parts":[{"text":"## First Article\n"}]},"finishReason":""}]}`,
		``,
		`data: {"candidates":[{"content":{"parts":[{"text":"A short summary."}]},"finishReason":"STOP"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var thoughtChunks, textChunks []string
	result, err := parseGeminiSSE(strings.NewReader(stream), func(s string) {
		thoughtChunks = append(thoughtChunks, s)
	}, func(s string) {
		textChunks = append(textChunks, s)
	})
	if err != nil {
		t.Fatalf("parseGeminiSSE returned an error: %v", err)
	}

	if result.Thoughts != "Considering the key points." {
		t.Errorf("unexpected thoughts: %q", result.Thoughts)
	}
	if result.Answer != "## First Article\nA short summary." {
		t.Errorf("unexpected answer: %q", result.Answer)
	}
	if result.FinishReason != "STOP" {
		t.Errorf("unexpected finish reason: %q", result.FinishReason)
	}
	if len(thoughtChunks) != 1 || thoughtChunks[0] != "Considering the key points." {
		t.Errorf("unexpected thought chunks: %#v", thoughtChunks)
	}
	if len(textChunks) != 2 {
		t.Errorf("unexpected text chunks: %#v", textChunks)
	}
}

func TestFormatGeminiHTTPError503(t *testing.T) {
	err := formatGeminiHTTPError(http.StatusServiceUnavailable, []byte(`{"error":{"code":503,"message":"This model is currently experiencing high demand."}}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"503", "overloaded", "different Gemini model", "high demand"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
}

func TestParseGeminiSSEReturnsAPIError(t *testing.T) {
	stream := "data: {\"error\":{\"code\":429,\"message\":\"quota exceeded\"}}\n\n"
	_, err := parseGeminiSSE(strings.NewReader(stream), nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGeminiSSERejectsEmptyAnswer(t *testing.T) {
	stream := `data: {"candidates":[{"content":{"parts":[{"text":"thinking","thought":true}]},"finishReason":"STOP"}]}` + "\n\n"
	_, err := parseGeminiSSE(strings.NewReader(stream), nil, nil)
	if err == nil {
		t.Fatal("expected an error for an empty answer")
	}
}

func TestStripCodeFences(t *testing.T) {
	wrapped := "```markdown\n## Title\nSummary\n```"
	got := stripCodeFences(wrapped)
	want := "## Title\nSummary"
	if got != want {
		t.Fatalf("stripCodeFences() = %q, want %q", got, want)
	}

	plain := "## Title\nSummary"
	if got := stripCodeFences(plain); got != plain {
		t.Fatalf("stripCodeFences() changed plain text: %q", got)
	}
}

func TestApplySummaryMatches(t *testing.T) {
	articles := []*Article{
		{Title: "First", Summary: ""},
		{Title: "Second", Summary: ""},
		{Title: "Third", Summary: "keep me"},
	}
	matches := []ParsedSummaryBlock{
		{RawTitle: "First", Body: "summary one", SelectedOption: "First"},
		{RawTitle: "Second", Body: "summary two", SelectedOption: OptionDoNotMatch},
		{RawTitle: "Third", Body: "summary three", SelectedOption: "Third"},
	}

	applied := ApplySummaryMatches(matches, articles)

	if len(applied) != 2 {
		t.Fatalf("applied = %#v, want two titles", applied)
	}
	if articles[0].Summary != "summary one" {
		t.Errorf("first summary = %q", articles[0].Summary)
	}
	if articles[1].Summary != "" {
		t.Errorf("second summary should remain empty, got %q", articles[1].Summary)
	}
	if articles[2].Summary != "summary three" {
		t.Errorf("third summary = %q", articles[2].Summary)
	}
}

func TestTailRunes(t *testing.T) {
	if got := tailRunes("hello", 10); got != "hello" {
		t.Fatalf("tailRunes short = %q", got)
	}
	if got := tailRunes("hello", 3); got != "...llo" {
		t.Fatalf("tailRunes long = %q", got)
	}
	if got := tailRunes("hello", 0); got != "" {
		t.Fatalf("tailRunes zero = %q", got)
	}
}
