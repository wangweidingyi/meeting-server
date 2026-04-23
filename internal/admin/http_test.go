package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"meeting-server/internal/config"
)

func TestHandlerReturnsCurrentSettings(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.ModelProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{
			Provider: "openai_compatible",
			Model:    "qwen-meeting",
		},
		TTS: config.SpeechProviderConfig{
			Provider: "stub",
		},
	}, func(config.AIConfig) {})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)
	recorder := httptest.NewRecorder()

	NewHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}

	var response Settings
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.AI.LLM.Model != "qwen-meeting" {
		t.Fatalf("unexpected llm model %s", response.AI.LLM.Model)
	}
}

func TestHandlerUpdatesSettings(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.ModelProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	body, err := json.Marshal(UpdateSettingsRequest{
		AI: config.AIConfig{
			STT: config.ModelProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1/audio/transcriptions",
				Model:    "sensevoice-large",
			},
			LLM: config.ModelProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1",
				Model:    "qwen-max",
			},
			TTS: config.SpeechProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1/audio/speech",
				Model:    "cosyvoice-v2",
				Voice:    "alloy",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	request := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	NewHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response Settings
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.AI.STT.Model != "sensevoice-large" {
		t.Fatalf("unexpected stt model %s", response.AI.STT.Model)
	}
	if response.AI.TTS.Voice != "alloy" {
		t.Fatalf("unexpected tts voice %s", response.AI.TTS.Voice)
	}
}

func TestHandlerTestsSettings(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.ModelProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	service.SetTester(&fakeTester{
		result: TestSettingsResult{
			STT: ProviderTestResult{
				OK:        true,
				Provider:  "openai_compatible",
				Message:   "stt ok",
				LatencyMS: 123,
			},
			LLM: ProviderTestResult{
				OK:        true,
				Provider:  "openai_compatible",
				Message:   "llm ok",
				LatencyMS: 87,
			},
			TTS: ProviderTestResult{
				OK:        false,
				Provider:  "openai_compatible",
				Message:   "tts failed",
				LatencyMS: 54,
			},
		},
	})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	body, err := json.Marshal(UpdateSettingsRequest{
		AI: config.AIConfig{
			STT: config.ModelProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1/audio/transcriptions",
				APIKey:   "stt-key",
				Model:    "sensevoice-large",
			},
			LLM: config.ModelProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1",
				APIKey:   "llm-key",
				Model:    "qwen-max",
			},
			TTS: config.SpeechProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1/audio/speech",
				APIKey:   "tts-key",
				Model:    "cosyvoice-v2",
				Voice:    "alloy",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/settings/test", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	NewHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response TestSettingsResult
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !response.STT.OK {
		t.Fatal("expected stt test to pass")
	}
	if response.TTS.OK {
		t.Fatal("expected tts test to fail")
	}
	if response.TTS.Message != "tts failed" {
		t.Fatalf("unexpected tts message %q", response.TTS.Message)
	}
}
