package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"meeting-server/internal/config"
)

func TestNewHandlerReturnsGinEngine(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	meetingStore := NewMemoryMeetingStore()
	handler := NewHandler(service, NewUserService(NewMemoryUserStore(), meetingStore), NewMeetingService(meetingStore), nil)
	if _, ok := handler.(*gin.Engine); !ok {
		t.Fatalf("expected *gin.Engine, got %T", handler)
	}
}

func TestHandlerReturnsCurrentSettings(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
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

	meetingStore := NewMemoryMeetingStore()
	NewHandler(service, NewUserService(NewMemoryUserStore(), meetingStore), NewMeetingService(meetingStore), nil).ServeHTTP(recorder, request)

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
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	body, err := json.Marshal(UpdateSettingsRequest{
		AI: config.AIConfig{
			STT: config.STTProviderConfig{
				Provider: "volcengine_streaming",
				BaseURL:  "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
				APIKey:   "stt-access-key",
				Model:    "bigmodel",
				Options: map[string]string{
					"appKey":     "stt-app-key",
					"resourceId": "volc.seedasr.sauc.duration",
					"language":   "zh-CN",
				},
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

	request := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	meetingStore := NewMemoryMeetingStore()
	NewHandler(service, NewUserService(NewMemoryUserStore(), meetingStore), NewMeetingService(meetingStore), nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response Settings
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.AI.STT.Model != "bigmodel" {
		t.Fatalf("unexpected stt model %s", response.AI.STT.Model)
	}
	if response.AI.STT.Options["appKey"] != "stt-app-key" {
		t.Fatalf("unexpected stt app key %s", response.AI.STT.Options["appKey"])
	}
	if response.AI.TTS.Voice != "alloy" {
		t.Fatalf("unexpected tts voice %s", response.AI.TTS.Voice)
	}
}

func TestHandlerTestsSettings(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
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
			STT: config.STTProviderConfig{
				Provider: "volcengine_streaming",
				BaseURL:  "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
				APIKey:   "stt-access-key",
				Model:    "bigmodel",
				Options: map[string]string{
					"appKey":     "stt-app-key",
					"resourceId": "volc.seedasr.sauc.duration",
				},
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

	meetingStore := NewMemoryMeetingStore()
	NewHandler(service, NewUserService(NewMemoryUserStore(), meetingStore), NewMeetingService(meetingStore), nil).ServeHTTP(recorder, request)

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

func TestHandlerUpsertsMeetings(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	meetingStore := NewMemoryMeetingStore()
	userService := NewUserService(NewMemoryUserStore(), meetingStore)
	meetingService := NewMeetingService(meetingStore)

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := userService.Upsert(context.Background(), UserRecord{
		ID:          "user-1",
		Username:    "zhangsan",
		DisplayName: "张三",
		Role:        UserRoleMember,
		Status:      UserStatusActive,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, err := json.Marshal(MeetingRecord{
		ID:         "meeting-1",
		UserID:     "user-1",
		UserName:   "张三",
		ClientID:   "meeting-desktop",
		Title:      "架构评审会",
		Status:     "recording",
		StartedAt:  "1710000000000",
		DurationMS: 12_345,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/meetings/meeting-1", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	NewHandler(service, userService, meetingService, nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/meetings", nil)
	NewHandler(service, userService, meetingService, nil).ServeHTTP(listRecorder, listRequest)

	if listRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected list status %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	var response struct {
		Items []MeetingRecord `json:"items"`
	}
	if err := json.NewDecoder(listRecorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(response.Items) != 1 {
		t.Fatalf("expected 1 meeting, got %d", len(response.Items))
	}
	if response.Items[0].Title != "架构评审会" {
		t.Fatalf("unexpected meeting title %q", response.Items[0].Title)
	}
	if response.Items[0].ClientID != "meeting-desktop" {
		t.Fatalf("unexpected client id %q", response.Items[0].ClientID)
	}
	if response.Items[0].UserID != "user-1" {
		t.Fatalf("unexpected user id %q", response.Items[0].UserID)
	}
	if response.Items[0].UserName != "张三" {
		t.Fatalf("unexpected user name %q", response.Items[0].UserName)
	}
}

func TestHandlerListsUsersWithMeetingHistory(t *testing.T) {
	service := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	meetingStore := NewMemoryMeetingStore()
	userService := NewUserService(NewMemoryUserStore(), meetingStore)
	meetingService := NewMeetingService(meetingStore)

	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := userService.Upsert(context.Background(), UserRecord{
		ID:          "user-1",
		Username:    "wangxiaoming",
		DisplayName: "王小明",
		Role:        UserRoleMember,
		Status:      UserStatusActive,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, err := json.Marshal(MeetingRecord{
		ID:         "meeting-1",
		UserID:     "user-1",
		UserName:   "王小明",
		ClientID:   "meeting-desktop",
		Title:      "销售复盘会",
		Status:     "completed",
		StartedAt:  "1710001000000",
		DurationMS: 88_000,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	upsertRecorder := httptest.NewRecorder()
	upsertRequest := httptest.NewRequest(http.MethodPut, "/api/admin/meetings/meeting-1", bytes.NewReader(body))
	upsertRequest.Header.Set("Content-Type", "application/json")
	NewHandler(service, userService, meetingService, nil).ServeHTTP(upsertRecorder, upsertRequest)

	if upsertRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected upsert status %d body=%s", upsertRecorder.Code, upsertRecorder.Body.String())
	}

	usersRecorder := httptest.NewRecorder()
	usersRequest := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	NewHandler(service, userService, meetingService, nil).ServeHTTP(usersRecorder, usersRequest)

	if usersRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected users status %d body=%s", usersRecorder.Code, usersRecorder.Body.String())
	}

	var usersResponse struct {
		Items []UserRecord `json:"items"`
	}
	if err := json.NewDecoder(usersRecorder.Body).Decode(&usersResponse); err != nil {
		t.Fatalf("decode users response: %v", err)
	}

	if len(usersResponse.Items) != 1 {
		t.Fatalf("expected 1 user, got %d", len(usersResponse.Items))
	}
	if usersResponse.Items[0].ID != "user-1" {
		t.Fatalf("unexpected user id %q", usersResponse.Items[0].ID)
	}
	if usersResponse.Items[0].DisplayName != "王小明" {
		t.Fatalf("unexpected display name %q", usersResponse.Items[0].DisplayName)
	}
	if usersResponse.Items[0].MeetingCount != 1 {
		t.Fatalf("unexpected meeting count %d", usersResponse.Items[0].MeetingCount)
	}

	meetingsRecorder := httptest.NewRecorder()
	meetingsRequest := httptest.NewRequest(http.MethodGet, "/api/admin/users/user-1/meetings", nil)
	NewHandler(service, userService, meetingService, nil).ServeHTTP(meetingsRecorder, meetingsRequest)

	if meetingsRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected meetings status %d body=%s", meetingsRecorder.Code, meetingsRecorder.Body.String())
	}

	var meetingsResponse struct {
		Items []MeetingRecord `json:"items"`
	}
	if err := json.NewDecoder(meetingsRecorder.Body).Decode(&meetingsResponse); err != nil {
		t.Fatalf("decode meetings response: %v", err)
	}

	if len(meetingsResponse.Items) != 1 {
		t.Fatalf("expected 1 meeting, got %d", len(meetingsResponse.Items))
	}
	if meetingsResponse.Items[0].ID != "meeting-1" {
		t.Fatalf("unexpected meeting id %q", meetingsResponse.Items[0].ID)
	}
	if meetingsResponse.Items[0].UserID != "user-1" {
		t.Fatalf("unexpected meeting user id %q", meetingsResponse.Items[0].UserID)
	}
}
