package openai_compatible

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SpeechClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	Voice      string
	HTTPClient *http.Client
}

func (c *SpeechClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.Model) == "" || strings.TrimSpace(c.Voice) == "" {
		return nil, errors.New("openai-compatible speech client is not fully configured")
	}

	requestBody := fmt.Sprintf(`{"model":%q,"input":%q,"voice":%q,"response_format":"mp3"}`, c.Model, text, c.Voice)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, speechURL(c.BaseURL), bytes.NewBufferString(requestBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("openai-compatible speech request failed: %s", response.Status)
	}

	return io.ReadAll(response.Body)
}

func (c *SpeechClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return &http.Client{Timeout: 30 * time.Second}
}

func speechURL(baseURL string) string {
	if strings.HasSuffix(baseURL, "/audio/speech") {
		return baseURL
	}

	return strings.TrimRight(baseURL, "/") + "/audio/speech"
}
