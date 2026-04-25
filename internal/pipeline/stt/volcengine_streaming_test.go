package stt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"

	"meeting-server/internal/config"
	"meeting-server/internal/protocol"
)

func TestVolcengineStreamingProviderEmitsRealtimeDeltaAndFinalTranscript(t *testing.T) {
	var (
		seenAppKey     string
		seenAccessKey  string
		seenResourceID string
	)

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenAppKey = request.Header.Get("X-Api-App-Key")
		seenAccessKey = request.Header.Get("X-Api-Access-Key")
		seenResourceID = request.Header.Get("X-Api-Resource-Id")

		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer connection.Close()

		if _, _, err := connection.ReadMessage(); err != nil {
			t.Errorf("read full client request: %v", err)
			return
		}
		if _, _, err := connection.ReadMessage(); err != nil {
			t.Errorf("read first audio frame: %v", err)
			return
		}
		if err := connection.WriteMessage(websocket.BinaryMessage, encodeVolcengineServerResponseForTest("大家好", false)); err != nil {
			t.Errorf("write first response: %v", err)
			return
		}
		if _, _, err := connection.ReadMessage(); err != nil {
			t.Errorf("read final audio frame: %v", err)
			return
		}
		if err := connection.WriteMessage(websocket.BinaryMessage, encodeVolcengineServerResponseForTest("大家好 今天开始开会", true)); err != nil {
			t.Errorf("write final response: %v", err)
			return
		}
	}))
	defer server.Close()

	service := NewService()
	service.SetConfig(config.STTProviderConfig{
		Provider: "volcengine_streaming",
		BaseURL:  "ws" + server.URL[len("http"):],
		APIKey:   "access-key",
		Model:    "bigmodel",
		Options: map[string]string{
			"appKey":      "app-key",
			"resourceId":  "volc.seedasr.sauc.duration",
			"language":    "zh-CN",
			"audioFormat": "pcm",
			"audioCodec":  "raw",
		},
	})

	if _, ok := service.Consume(streamingTestPacket(1, 0)); ok {
		t.Fatal("expected first packet to remain buffered for final framing")
	}

	firstDelta, ok := service.Consume(streamingTestPacket(2, 200))
	if !ok {
		t.Fatal("expected second packet to emit transcript delta")
	}
	if firstDelta.Text != "大家好" {
		t.Fatalf("unexpected first delta %q", firstDelta.Text)
	}

	finalPayload, ok := service.Flush("session-1")
	if !ok {
		t.Fatal("expected final transcript from volcengine streaming session")
	}
	if !finalPayload.IsFinal {
		t.Fatal("expected final payload to be marked final")
	}
	if finalPayload.Text != "大家好 今天开始开会" {
		t.Fatalf("unexpected final transcript %q", finalPayload.Text)
	}
	if seenAppKey != "app-key" {
		t.Fatalf("unexpected app key header %q", seenAppKey)
	}
	if seenAccessKey != "access-key" {
		t.Fatalf("unexpected access key header %q", seenAccessKey)
	}
	if seenResourceID != "volc.seedasr.sauc.duration" {
		t.Fatalf("unexpected resource id header %q", seenResourceID)
	}
}

func TestVolcengineStreamingSmokeTestValidatesHandshake(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer connection.Close()

		if _, _, err := connection.ReadMessage(); err != nil {
			t.Errorf("read full client request: %v", err)
			return
		}
		if err := connection.WriteMessage(websocket.BinaryMessage, encodeVolcengineServerResponseForTest("测试通过", true)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewVolcengineStreamingClient(config.STTProviderConfig{
		Provider: "volcengine_streaming",
		BaseURL:  "ws" + server.URL[len("http"):],
		APIKey:   "access-key",
		Model:    "bigmodel",
		Options: map[string]string{
			"appKey":     "app-key",
			"resourceId": "volc.seedasr.sauc.duration",
		},
	})

	if err := client.SmokeTest(context.Background()); err != nil {
		t.Fatalf("smoke test failed: %v", err)
	}
}

func TestBuildVolcengineInitPayloadUsesFixedBigmodelAndOmitsLanguageForAsync(t *testing.T) {
	payload, err := buildVolcengineInitPayload(config.STTProviderConfig{
		Provider: "volcengine_streaming",
		BaseURL:  "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
		APIKey:   "access-key",
		Model:    "seedasr-2.0",
		Options: map[string]string{
			"language":    "zh-CN",
			"audioFormat": "pcm",
			"audioCodec":  "raw",
		},
	}, "session-1")
	if err != nil {
		t.Fatalf("build init payload: %v", err)
	}

	var decoded struct {
		Audio map[string]any `json:"audio"`
		Request struct {
			ModelName string `json:"model_name"`
		} `json:"request"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if decoded.Request.ModelName != "bigmodel" {
		t.Fatalf("expected model_name bigmodel, got %q", decoded.Request.ModelName)
	}
	if _, ok := decoded.Audio["language"]; ok {
		t.Fatal("expected async payload to omit audio.language")
	}
}

func encodeVolcengineServerResponseForTest(text string, isFinal bool) []byte {
	frame, err := encodeVolcengineServerResponse(volcengineServerResponse{
		Text:    text,
		IsFinal: isFinal,
	})
	if err != nil {
		panic(err)
	}

	return frame
}

func streamingTestPacket(sequence uint64, startedAtMS uint64) protocol.MixedAudioPacket {
	return protocol.MixedAudioPacket{
		SessionID:   "session-1",
		Sequence:    sequence,
		StartedAtMS: startedAtMS,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}
}
