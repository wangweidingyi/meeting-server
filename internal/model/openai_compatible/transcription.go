package openai_compatible

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type TranscriptionClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type transcriptionResponse struct {
	Text string `json:"text"`
}

func (c *TranscriptionClient) Recognize(ctx context.Context, wave []byte) (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.Model) == "" {
		return "", errors.New("openai-compatible transcription client is not fully configured")
	}
	if len(wave) == 0 {
		return "", errors.New("transcription wave payload is empty")
	}

	body, contentType, err := buildTranscriptionRequestBody(c.Model, wave)
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, transcriptionURL(c.BaseURL), body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", contentType)

	response, err := c.httpClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("openai-compatible transcription request failed: %s", response.Status)
	}

	var decoded transcriptionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return "", err
	}

	text := strings.TrimSpace(decoded.Text)
	if text == "" {
		return "", errors.New("openai-compatible transcription response text is empty")
	}

	return text, nil
}

func buildTranscriptionRequestBody(model string, wave []byte) (*bytes.Buffer, string, error) {
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)

	modelField, err := writer.CreateFormField("model")
	if err != nil {
		return nil, "", err
	}
	if _, err := modelField.Write([]byte(model)); err != nil {
		return nil, "", err
	}

	fileField, err := writer.CreateFormFile("file", "meeting.wav")
	if err != nil {
		return nil, "", err
	}
	if _, err := fileField.Write(wave); err != nil {
		return nil, "", err
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return buffer, writer.FormDataContentType(), nil
}

func (c *TranscriptionClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return &http.Client{Timeout: 60 * time.Second}
}

func transcriptionURL(baseURL string) string {
	if strings.HasSuffix(baseURL, "/audio/transcriptions") {
		return baseURL
	}

	return strings.TrimRight(baseURL, "/") + "/audio/transcriptions"
}
