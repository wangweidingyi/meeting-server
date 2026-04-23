package admin

import (
	"context"
	"testing"

	"meeting-server/internal/config"
)

func TestServiceReturnsInitialSettingsWhenStoreIsEmpty(t *testing.T) {
	initialAI := config.AIConfig{
		STT: config.ModelProviderConfig{
			Provider: "stub",
			Model:    "stt-stub",
		},
		LLM: config.ModelProviderConfig{
			Provider: "openai_compatible",
			BaseURL:  "https://example.com/v1",
			Model:    "gpt-meeting",
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

	if settings.AI.LLM.Model != "gpt-meeting" {
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
		STT: config.ModelProviderConfig{Provider: "stub"},
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
				Voice:    "xiaoyi",
			},
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	current := service.Current()

	if current.AI.STT.Model != "sensevoice-large" {
		t.Fatalf("unexpected stt model %s", current.AI.STT.Model)
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
