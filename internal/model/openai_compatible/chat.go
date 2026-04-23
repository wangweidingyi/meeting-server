package openai_compatible

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ChatClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
	Temperature    float64        `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *ChatClient) CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, out any) error {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.Model) == "" {
		return errors.New("openai-compatible chat client is not fully configured")
	}

	requestBody := chatCompletionRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: responseFormat{Type: "json_object"},
		Temperature:    0.2,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(c.BaseURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("openai-compatible chat request failed: %s", response.Status)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return err
	}
	if len(decoded.Choices) == 0 {
		return errors.New("openai-compatible chat response has no choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return errors.New("openai-compatible chat response content is empty")
	}

	return json.Unmarshal([]byte(content), out)
}

func (c *ChatClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return &http.Client{Timeout: 30 * time.Second}
}

func chatCompletionsURL(baseURL string) string {
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}

	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}
