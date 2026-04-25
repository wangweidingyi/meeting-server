package admin

import (
	"context"
	"testing"

	"meeting-server/internal/config"
)

func TestServiceReturnsInitialSettingsWhenStoreIsEmpty(t *testing.T) {
	initialAI := config.AIConfig{
		STT: config.STTProviderConfig{
			Provider: "stub",
			Model:    "stt-stub",
		},
		LLM: config.ModelProviderConfig{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
			APIKey:   "openai-key",
			Model:    "gpt-4.1-mini",
		},
		TTS: config.SpeechProviderConfig{
			Provider: "openai_compatible",
			BaseURL:  "https://example.com/v1/audio/speech",
			Model:    "voice-meeting",
			Voice:    "alloy",
		},
	}

	var applied config.AIConfig
	service := NewService(NewMemoryStore(), initialAI, func(next config.AIConfig) {
		applied = next
	})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	settings := service.Current()

	if settings.AI.LLM.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected llm model %s", settings.AI.LLM.Model)
	}
	if applied.TTS.Voice != "alloy" {
		t.Fatalf("expected bootstrap to apply initial settings, got voice %s", applied.TTS.Voice)
	}
	if settings.Version != 0 {
		t.Fatalf("expected empty store bootstrap version to stay 0, got %d", settings.Version)
	}
}

func TestServiceUpdatePersistsAndAppliesSettings(t *testing.T) {
	initialAI := config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}

	var applied []config.AIConfig
	service := NewService(NewMemoryStore(), initialAI, func(next config.AIConfig) {
		applied = append(applied, next)
	})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	updated, err := service.Update(context.Background(), UpdateSettingsRequest{
		AI: config.AIConfig{
			STT: config.STTProviderConfig{
				Provider: "volcengine_streaming",
				BaseURL:  "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
				APIKey:   "stt-access-key",
				Model:    "bigmodel",
				Options: map[string]string{
					"appKey":         "stt-app-key",
					"resourceId":     "volc.seedasr.sauc.duration",
					"language":       "zh-CN",
					"audioFormat":    "pcm",
					"audioCodec":     "raw",
					"sampleRate":     "16000",
					"bits":           "16",
					"channels":       "1",
					"enableItn":      "true",
					"enablePunc":     "true",
					"showUtterances": "true",
				},
			},
			LLM: config.ModelProviderConfig{
				Provider: "deepseek",
				BaseURL:  "https://api.deepseek.com/v1",
				APIKey:   "deepseek-key",
				Model:    "deepseek-chat",
			},
			TTS: config.SpeechProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1/audio/speech",
				APIKey:   "tts-key",
				Model:    "cosyvoice-v2",
				Voice:    "xiaoyi",
			},
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	current := service.Current()

	if current.AI.STT.Model != "bigmodel" {
		t.Fatalf("unexpected stt model %s", current.AI.STT.Model)
	}
	if current.AI.STT.Provider != "volcengine_streaming" {
		t.Fatalf("unexpected stt provider %s", current.AI.STT.Provider)
	}
	if current.AI.STT.Options["appKey"] != "stt-app-key" {
		t.Fatalf("unexpected stt app key %s", current.AI.STT.Options["appKey"])
	}
	if updated.Version != 1 {
		t.Fatalf("expected version 1 after first save, got %d", updated.Version)
	}
	if len(applied) != 2 {
		t.Fatalf("expected bootstrap and update apply callbacks, got %d", len(applied))
	}
	if applied[1].TTS.Voice != "xiaoyi" {
		t.Fatalf("unexpected applied voice %s", applied[1].TTS.Voice)
	}
}

func TestServiceRejectsIncompleteDeepSeekLLMConfig(t *testing.T) {
	initialAI := config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}

	service := NewService(NewMemoryStore(), initialAI, func(config.AIConfig) {})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	_, err := service.Test(context.Background(), UpdateSettingsRequest{
		AI: config.AIConfig{
			STT: config.STTProviderConfig{Provider: "stub"},
			LLM: config.ModelProviderConfig{
				Provider: "deepseek",
				APIKey:   "deepseek-key",
				Model:    "deepseek-chat",
			},
			TTS: config.SpeechProviderConfig{Provider: "stub"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if err.Error() != "ai.llm.baseUrl is required for deepseek" {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestNewServicePanicsWhenStoreIsNil(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic when store is nil")
		}
	}()

	NewService(nil, config.AIConfig{}, func(config.AIConfig) {})
}
