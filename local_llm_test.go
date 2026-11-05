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

	if !strings.Contains(prompt, DefaultSummaryPrompt) {
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
	if strings.Contains(prompt, DefaultSummaryPrompt) {
		t.Fatalf("prompt should not contain the default instruction when a custom one is set")
	}
}

func TestParseLocalSSEStreamsContent(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"## First Article\n"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"A short summary."},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var chunks []string
	result, err := parseLocalSSE(strings.NewReader(stream), func(s string) {
		chunks = append(chunks, s)
	})
	if err != nil {
		t.Fatalf("parseLocalSSE returned an error: %v", err)
	}

	if result.Answer != "## First Article\nA short summary." {
		t.Errorf("unexpected answer: %q", result.Answer)
	}
	if result.FinishReason != "stop" {
		t.Errorf("unexpected finish reason: %q", result.FinishReason)
	}
	if len(chunks) != 2 {
		t.Errorf("unexpected text chunks: %#v", chunks)
	}
}

func TestFormatLocalHTTPError(t *testing.T) {
	err := formatLocalHTTPError(http.StatusBadRequest, []byte(`{"error":{"code":400,"message":"the request exceeds the available context size"}}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"400", "context size"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
}

func TestParseLocalSSEReturnsAPIError(t *testing.T) {
	stream := "data: {\"error\":{\"code\":500,\"message\":\"model failed\"}}\n\n"
	_, err := parseLocalSSE(strings.NewReader(stream), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "model failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLocalSSERejectsEmptyAnswer(t *testing.T) {
	stream := `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"
	_, err := parseLocalSSE(strings.NewReader(stream), nil)
	if err == nil {
		t.Fatal("expected an error for an empty answer")
	}
}

func TestSummaryFromAnswerExtractsBody(t *testing.T) {
	answer := "```markdown\n## First Article\nA short summary.\n```"
	if got := summaryFromAnswer(answer); got != "A short summary." {
		t.Fatalf("summaryFromAnswer() = %q, want the summary body", got)
	}

	if got := summaryFromAnswer("Just the summary."); got != "Just the summary." {
		t.Fatalf("summaryFromAnswer() = %q, want the whole answer as a fallback", got)
	}

	plain := "First line of the summary.\nSecond line."
	if got := summaryFromAnswer(plain); got != plain {
		t.Fatalf("summaryFromAnswer() = %q, want the whole heading-less answer", got)
	}
}

func TestLiveSummaryTextHidesTitleHeading(t *testing.T) {
	cases := map[string]string{
		"## Solar panels":                 "",
		"## Solar panels\n":               "",
		"## Solar panels\nSolar":          "Solar",
		"## Solar panels\n\nPrices fell.": "Prices fell.",
		"Prices fell.":                    "Prices fell.",
		"```markdown\n## Title\nBody":     "Body",
	}
	for in, want := range cases {
		if got := liveSummaryText(in); got != want {
			t.Errorf("liveSummaryText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalQwenProfileUses9b(t *testing.T) {
	if DefaultLocalLLMModel != "Qwen3.5-9B" {
		t.Fatalf("DefaultLocalLLMModel = %q, want the 9B model", DefaultLocalLLMModel)
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
