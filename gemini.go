package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const geminiStreamEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?alt=sse"

var geminiHTTPClient = &http.Client{Timeout: 10 * time.Minute}

type GeminiStreamResult struct {
	Thoughts     string
	Answer       string
	FinishReason string
}

type geminiGenerateRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature     float64               `json:"temperature,omitempty"`
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	IncludeThoughts bool   `json:"includeThoughts,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
}

type geminiStreamChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *geminiAPIError `json:"error"`
}

type geminiAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// BuildSummaryPrompt combines the user's instruction with a strict output
// contract. The contract keeps Gemini's response parseable by the existing
// pasted-summary matcher.
func BuildSummaryPrompt(instruction string, articles []Article) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		instruction = DefaultGeminiPrompt
	}

	var sb strings.Builder
	sb.WriteString(instruction)
	sb.WriteString("\n\n")
	sb.WriteString("Output format (follow exactly):\n")
	sb.WriteString("- Return exactly one section for each article, in the order they appear below.\n")
	sb.WriteString("- Start each section with a Markdown heading made of two hash marks and the article's exact title.\n")
	sb.WriteString("- Put the summary text on the lines after that heading.\n")
	sb.WriteString("- Do not change, translate, or rephrase the title.\n")
	sb.WriteString("- Do not add a preamble, conclusion, commentary, or any extra headings.\n")
	sb.WriteString("- Do not wrap the response in a code block.\n")
	sb.WriteString("- Do not skip any article.\n\n")

	for i, art := range articles {
		title := strings.TrimSpace(art.Title)
		if title == "" {
			title = fmt.Sprintf("Untitled article %d", i+1)
		}
		sb.WriteString(fmt.Sprintf("--- ARTICLE %d ---\n", i+1))
		sb.WriteString("TITLE: ")
		sb.WriteString(title)
		sb.WriteString("\nCONTENT:\n")
		sb.WriteString(strings.TrimSpace(art.FullText))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func geminiThinkingConfigForModel(model string) *geminiThinkingConfig {
	if strings.HasPrefix(model, "gemini-3") || strings.HasPrefix(model, "gemini-4") {
		return &geminiThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   "low",
		}
	}
	return &geminiThinkingConfig{IncludeThoughts: true}
}

// StreamGeminiSummaries makes exactly one streaming request for all supplied
// articles. onThought and onText receive incremental chunks as they arrive.
func StreamGeminiSummaries(
	ctx context.Context,
	apiKey, model, prompt string,
	onThought, onText func(string),
) (GeminiStreamResult, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return GeminiStreamResult{}, fmt.Errorf("missing Gemini API key")
	}
	model = strings.TrimSpace(model)
	model = strings.TrimPrefix(model, "models/")
	if model == "" {
		model = DefaultGeminiModel
	}

	body := geminiGenerateRequest{
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: prompt}},
		}},
		GenerationConfig: geminiGenerationConfig{
			MaxOutputTokens: 65536,
			ThinkingConfig:  geminiThinkingConfigForModel(model),
		},
	}
	// Gemini 4 and Gemini 3.6+ ignore temperature; sending it will be an
	// error in a future release, so leave it out for the 3.x/4.x models.
	if !strings.HasPrefix(model, "gemini-3") && !strings.HasPrefix(model, "gemini-4") {
		body.GenerationConfig.Temperature = 0.2
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return GeminiStreamResult{}, fmt.Errorf("preparing Gemini request: %w", err)
	}

	endpoint := fmt.Sprintf(geminiStreamEndpoint, url.PathEscape(model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return GeminiStreamResult{}, fmt.Errorf("preparing Gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := geminiHTTPClient.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return GeminiStreamResult{}, ctx.Err()
		}
		return GeminiStreamResult{}, fmt.Errorf("could not reach Gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return GeminiStreamResult{}, formatGeminiHTTPError(resp.StatusCode, data)
	}

	return parseGeminiSSE(resp.Body, onThought, onText)
}

func formatGeminiHTTPError(status int, body []byte) error {
	var payload struct {
		Error geminiAPIError `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error.Message) != "" {
		if status == http.StatusServiceUnavailable {
			return geminiUnavailableError(payload.Error.Message)
		}
		if status == http.StatusTooManyRequests {
			return fmt.Errorf("Gemini rate limit reached. Wait a bit before trying again. (%s)", payload.Error.Message)
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return fmt.Errorf("Gemini rejected the API key or model access: %s", payload.Error.Message)
		}
		return fmt.Errorf("Gemini API error (%d): %s", status, payload.Error.Message)
	}

	msg := strings.TrimSpace(string(body))
	if runes := []rune(msg); len(runes) > 300 {
		msg = string(runes[:300]) + "..."
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	if status == http.StatusServiceUnavailable {
		return geminiUnavailableError(msg)
	}
	if status == http.StatusTooManyRequests {
		return fmt.Errorf("Gemini rate limit reached. Wait a bit before trying again. (%s)", msg)
	}
	return fmt.Errorf("Gemini API error (%d): %s", status, msg)
}

// geminiUnavailableError explains a 503, which Gemini returns when the model
// is at capacity. It is a Google-side condition, not a malformed request.
func geminiUnavailableError(detail string) error {
	return fmt.Errorf("Gemini is temporarily overloaded (503). This is usually a Google-side capacity issue; retry later, or try a different Gemini model in Settings. (%s)", detail)
}

func parseGeminiSSE(r io.Reader, onThought, onText func(string)) (GeminiStreamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var thoughts, answer strings.Builder
	var finishReason, blockReason string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Ignore non-JSON keepalive data rather than failing the whole stream.
			continue
		}
		if chunk.Error != nil {
			return GeminiStreamResult{
				Thoughts: thoughts.String(),
				Answer:   answer.String(),
			}, fmt.Errorf("Gemini API error (%d): %s", chunk.Error.Code, chunk.Error.Message)
		}
		if chunk.PromptFeedback.BlockReason != "" {
			blockReason = chunk.PromptFeedback.BlockReason
		}

		for _, candidate := range chunk.Candidates {
			if candidate.FinishReason != "" {
				finishReason = candidate.FinishReason
			}
			for _, part := range candidate.Content.Parts {
				if part.Text == "" {
					continue
				}
				if part.Thought {
					thoughts.WriteString(part.Text)
					if onThought != nil {
						onThought(part.Text)
					}
				} else {
					answer.WriteString(part.Text)
					if onText != nil {
						onText(part.Text)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return GeminiStreamResult{
			Thoughts: thoughts.String(),
			Answer:   answer.String(),
		}, fmt.Errorf("reading Gemini response: %w", err)
	}

	result := GeminiStreamResult{
		Thoughts:     thoughts.String(),
		Answer:       answer.String(),
		FinishReason: finishReason,
	}
	if blockReason != "" {
		return result, fmt.Errorf("Gemini blocked the request: %s", blockReason)
	}
	if strings.TrimSpace(result.Answer) == "" {
		if finishReason != "" {
			return result, fmt.Errorf("Gemini returned no summary (finish reason: %s)", finishReason)
		}
		return result, fmt.Errorf("Gemini returned an empty response")
	}
	return result, nil
}

// stripCodeFences removes a wrapping Markdown code fence, which models
// sometimes add even when asked not to.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func tailRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return "..." + string(runes[len(runes)-n:])
}
