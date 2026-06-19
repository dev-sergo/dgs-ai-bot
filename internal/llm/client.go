// Package llm — минимальный клиент OpenAI-совместимого Chat API (llama.cpp).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"dgsbot/internal/config"
)

// Message — реплика чата.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatOptions — параметры генерации.
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	// JSONObject требует от модели валидный JSON (response_format).
	JSONObject bool
}

// Client — обёртка над /v1/chat/completions.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New создаёт клиента по конфигу.
func New(cfg config.LLM) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Chat выполняет запрос и возвращает текст ответа ассистента.
func (c *Client) Chat(ctx context.Context, model string, msgs []Message, opt ChatOptions) (string, error) {
	reqBody := chatRequest{
		Model:       model,
		Messages:    msgs,
		Temperature: opt.Temperature,
		MaxTokens:   opt.MaxTokens,
	}
	if opt.JSONObject {
		reqBody.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm http %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("llm error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return cr.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
