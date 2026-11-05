package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// LocalLLMBaseURL is the OpenAI-compatible endpoint exposed by the
	// llama.cpp server from ~/Apps/qwen3.5.
	LocalLLMBaseURL = "http://127.0.0.1:8081/v1"
	// LocalLLMHealthURL is the server's health endpoint.
	LocalLLMHealthURL = "http://127.0.0.1:8081/health"
	// DefaultLocalLLMModel is the model name llama.cpp is serving. The
	// server ignores the name, but OpenAI clients require one.
	DefaultLocalLLMModel = "Qwen3.5-9B"

	localLLMService      = "qwen35.service"
	localLLMStartTimeout = 3 * time.Minute
)

// localLLMHTTPClient talks to the local server directly. It deliberately
// bypasses the desktop proxy: proxying a 127.0.0.1 request only adds latency
// and can fail when the proxy is unavailable.
var localLLMHTTPClient = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{
		Timeout:   10 * time.Minute,
		Transport: transport,
	}
}()

var localLLMHealthClient = &http.Client{Timeout: 3 * time.Second}

type LocalStreamResult struct {
	Answer       string
	FinishReason string
}

type localChatRequest struct {
	Model              string         `json:"model"`
	Messages           []localMessage `json:"messages"`
	Stream             bool           `json:"stream"`
	MaxTokens          int            `json:"max_tokens,omitempty"`
	Temperature        float64        `json:"temperature,omitempty"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

type localMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type localStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *localAPIError `json:"error"`
}

type localAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// BuildSummaryPrompt combines the user's instruction with the article and a
// strict output contract. The contract keeps the response parseable by the
// existing pasted-summary parser. The caller passes one article at a time so
// each request fits comfortably in the local model's context window.
func BuildSummaryPrompt(instruction string, articles []Article) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		instruction = DefaultSummaryPrompt
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

// LocalLLMHealthy reports whether the local llama.cpp server is accepting
// requests.
func LocalLLMHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LocalLLMHealthURL, nil)
	if err != nil {
		return false
	}
	resp, err := localLLMHealthClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4*1024))
	return resp.StatusCode == http.StatusOK
}

// StreamLocalSummary sends one article prompt to the local model and streams
// the summary back. Thinking is disabled per request; the 9B model is fast
// enough for summaries without spending output tokens on a reasoning trace.
func StreamLocalSummary(
	ctx context.Context,
	prompt string,
	onText func(string),
) (LocalStreamResult, error) {
	body := localChatRequest{
		Model:       DefaultLocalLLMModel,
		Messages:    []localMessage{{Role: "user", Content: prompt}},
		Stream:      true,
		MaxTokens:   4096,
		Temperature: 0.2,
		ChatTemplateKwargs: map[string]any{
			"enable_thinking": false,
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return LocalStreamResult{}, fmt.Errorf("preparing local AI request: %w", err)
	}

	endpoint := strings.TrimRight(LocalLLMBaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return LocalStreamResult{}, fmt.Errorf("preparing local AI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := localLLMHTTPClient.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return LocalStreamResult{}, ctx.Err()
		}
		return LocalStreamResult{}, fmt.Errorf("could not reach the local AI service. Turn it on from the AI Summary dialog. (%w)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return LocalStreamResult{}, formatLocalHTTPError(resp.StatusCode, data)
	}

	return parseLocalSSE(resp.Body, onText)
}

func formatLocalHTTPError(status int, body []byte) error {
	var payload struct {
		Error localAPIError `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return fmt.Errorf("local AI error (%d): %s", status, payload.Error.Message)
	}

	msg := strings.TrimSpace(string(body))
	if runes := []rune(msg); len(runes) > 300 {
		msg = string(runes[:300]) + "..."
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return fmt.Errorf("local AI error (%d): %s", status, msg)
}

func parseLocalSSE(r io.Reader, onText func(string)) (LocalStreamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var answer strings.Builder
	var finishReason string

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

		var chunk localStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Ignore non-JSON keepalive data rather than failing the stream.
			continue
		}
		if chunk.Error != nil {
			return LocalStreamResult{
				Answer: answer.String(),
			}, fmt.Errorf("local AI error (%d): %s", chunk.Error.Code, chunk.Error.Message)
		}

		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			piece := choice.Delta.Content
			if piece == "" {
				continue
			}
			answer.WriteString(piece)
			if onText != nil {
				onText(piece)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return LocalStreamResult{
			Answer: answer.String(),
		}, fmt.Errorf("reading local AI response: %w", err)
	}

	result := LocalStreamResult{
		Answer:       answer.String(),
		FinishReason: finishReason,
	}
	if strings.TrimSpace(result.Answer) == "" {
		if finishReason != "" {
			return result, fmt.Errorf("local AI returned no summary (finish reason: %s)", finishReason)
		}
		return result, fmt.Errorf("local AI returned an empty response")
	}
	return result, nil
}

// summaryFromAnswer extracts the summary body from the model's single-section
// Markdown response. If the model ignored the heading contract, the whole
// answer is used rather than losing its first line to the heading parser.
func summaryFromAnswer(answer string) string {
	answer = stripCodeFences(answer)
	hasHeading := false
	for _, line := range strings.Split(answer, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasHeading = true
			break
		}
	}
	if hasHeading {
		blocks := ParsePastedSummaries(answer)
		if len(blocks) > 0 && strings.TrimSpace(blocks[0].Body) != "" {
			return strings.TrimSpace(blocks[0].Body)
		}
	}
	return strings.TrimSpace(answer)
}

// localQwenCLI returns the qwen launcher that starts and stops the server.
// It prefers the copy on PATH and falls back to the standard install folder.
func localQwenCLI() string {
	if path, err := exec.LookPath("qwen"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(home, "Apps", "qwen3.5", "bin", "qwen")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// StartLocalLLMService starts the on-demand Qwen service and waits until its
// health endpoint answers. The server holds several GB of VRAM, which is why
// this is an explicit action rather than starting at login.
func StartLocalLLMService(ctx context.Context) error {
	if LocalLLMHealthy() {
		return nil
	}

	if qwen := localQwenCLI(); qwen != "" {
		out, err := exec.CommandContext(ctx, qwen, "start").CombinedOutput()
		if err == nil && LocalLLMHealthy() {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("starting the local AI service timed out: %w", ctx.Err())
		}
		if err != nil {
			return fmt.Errorf("could not start the local AI service: %v%s", err, commandDetail(out))
		}
		return fmt.Errorf("the local AI service did not become ready%s", commandDetail(out))
	}

	out, err := exec.CommandContext(ctx, "systemctl", "--user", "start", localLLMService).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not start the local AI service: %v%s", err, commandDetail(out))
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if LocalLLMHealthy() {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("starting the local AI service timed out: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// StopLocalLLMService shuts the server down and releases its VRAM.
func StopLocalLLMService(ctx context.Context) error {
	if qwen := localQwenCLI(); qwen != "" {
		out, err := exec.CommandContext(ctx, qwen, "stop").CombinedOutput()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("stopping the local AI service timed out: %w", ctx.Err())
		}
		if strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf("could not stop the local AI service: %v%s", err, commandDetail(out))
		}
	}

	out, err := exec.CommandContext(ctx, "systemctl", "--user", "stop", localLLMService).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not stop the local AI service: %v%s", err, commandDetail(out))
	}
	return nil
}

func commandDetail(out []byte) string {
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return ""
	}
	if runes := []rune(detail); len(runes) > 500 {
		detail = string(runes[:500]) + "..."
	}
	return ": " + detail
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
