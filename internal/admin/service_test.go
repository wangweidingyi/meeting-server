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

func TestMeetingDetailServiceStoresTranscriptSummaryCheckpointAndAssets(t *testing.T) {
	service := NewMeetingDetailService(NewMemoryMeetingDetailStore())

	firstSegment, err := service.UpsertTranscriptSegment(context.Background(), TranscriptSegmentRecord{
		MeetingID: "meeting-1",
		SegmentID: "segment-2",
		StartMS:   1000,
		EndMS:     2000,
		Text:      "第二段",
		Revision:  2,
	})
	if err != nil {
		t.Fatalf("upsert first transcript segment: %v", err)
	}
	if firstSegment.SegmentID != "segment-2" {
		t.Fatalf("unexpected first segment id %q", firstSegment.SegmentID)
	}

	if _, err := service.UpsertTranscriptSegment(context.Background(), TranscriptSegmentRecord{
		MeetingID: "meeting-1",
		SegmentID: "segment-1",
		StartMS:   0,
		EndMS:     900,
		Text:      "第一段",
		Revision:  1,
	}); err != nil {
		t.Fatalf("upsert second transcript segment: %v", err)
	}

	segments, err := service.ListTranscriptSegmentsByMeeting(context.Background(), "meeting-1")
	if err != nil {
		t.Fatalf("list transcript segments: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 transcript segments, got %d", len(segments))
	}
	if segments[0].SegmentID != "segment-1" {
		t.Fatalf("expected transcript segments to sort by start time, got first id %q", segments[0].SegmentID)
	}

	summary, err := service.UpsertSummarySnapshot(context.Background(), SummarySnapshotRecord{
		MeetingID:    "meeting-1",
		Version:      3,
		UpdatedAt:    "2026-05-07T10:00:00Z",
		AbstractText: "会议摘要",
		KeyPoints:    []string{"关键点"},
		Decisions:    []string{"决策"},
		Risks:        []string{"风险"},
		ActionItems:  []string{"行动项-旧"},
		IsFinal:      false,
	})
	if err != nil {
		t.Fatalf("upsert summary: %v", err)
	}
	if summary.Version != 3 {
		t.Fatalf("unexpected summary version %d", summary.Version)
	}

	mergedSummary, err := service.ApplyActionItems(context.Background(), ActionItemsRecord{
		MeetingID: "meeting-1",
		Version:   4,
		UpdatedAt: "2026-05-07T10:01:00Z",
		Items:     []string{"行动项-新"},
		IsFinal:   true,
	})
	if err != nil {
		t.Fatalf("apply action items: %v", err)
	}
	if mergedSummary.AbstractText != "会议摘要" {
		t.Fatalf("expected action item merge to preserve summary abstract, got %q", mergedSummary.AbstractText)
	}
	if len(mergedSummary.ActionItems) != 1 || mergedSummary.ActionItems[0] != "行动项-新" {
		t.Fatalf("unexpected merged action items %+v", mergedSummary.ActionItems)
	}
	if !mergedSummary.IsFinal {
		t.Fatal("expected merged summary to become final")
	}

	recoveryToken := "recover-1"
	checkpoint, err := service.UpsertCheckpoint(context.Background(), SessionCheckpointRecord{
		MeetingID:                     "meeting-1",
		LastControlSeq:                2,
		LastUDPSeqSent:                7,
		LastUploadedMixedMS:           1200,
		LastTranscriptSegmentRevision: 3,
		LastSummaryVersion:            4,
		LastActionItemVersion:         4,
		LocalRecordingState:           "recording",
		RecoveryToken:                 &recoveryToken,
		UpdatedAt:                     "2026-05-07T10:02:00Z",
	})
	if err != nil {
		t.Fatalf("upsert checkpoint: %v", err)
	}
	if checkpoint.LastUDPSeqSent != 7 {
		t.Fatalf("unexpected checkpoint udp seq %d", checkpoint.LastUDPSeqSent)
	}

	loadedCheckpoint, foundCheckpoint, err := service.FindCheckpoint(context.Background(), "meeting-1")
	if err != nil {
		t.Fatalf("find checkpoint: %v", err)
	}
	if !foundCheckpoint {
		t.Fatal("expected checkpoint to exist")
	}
	if loadedCheckpoint.LastUploadedMixedMS != 1200 {
		t.Fatalf("unexpected checkpoint last uploaded mixed ms %d", loadedCheckpoint.LastUploadedMixedMS)
	}

	micPath := "/tmp/meeting-1/mic.wav"
	systemPath := "/tmp/meeting-1/system.wav"
	mixedPath := "/tmp/meeting-1/mixed.wav"
	assets, err := service.UpsertAudioAssets(context.Background(), AudioAssetRecord{
		MeetingID:          "meeting-1",
		MicOriginalPath:    &micPath,
		SystemOriginalPath: &systemPath,
		MixedUplinkPath:    &mixedPath,
	})
	if err != nil {
		t.Fatalf("upsert audio assets: %v", err)
	}
	if assets.MixedUplinkPath == nil || *assets.MixedUplinkPath != mixedPath {
		t.Fatalf("unexpected mixed uplink path %+v", assets.MixedUplinkPath)
	}

	loadedAssets, foundAssets, err := service.FindAudioAssets(context.Background(), "meeting-1")
	if err != nil {
		t.Fatalf("find audio assets: %v", err)
	}
	if !foundAssets {
		t.Fatal("expected audio assets to exist")
	}
	if loadedAssets.SystemOriginalPath == nil || *loadedAssets.SystemOriginalPath != systemPath {
		t.Fatalf("unexpected loaded system path %+v", loadedAssets.SystemOriginalPath)
	}
}
