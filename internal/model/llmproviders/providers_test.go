package llmproviders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"meeting-server/internal/config"
)

func TestNewChatClientKimiOmitsTemperature(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if _, ok := payload["temperature"]; ok {
			t.Fatalf("expected kimi request to omit temperature, got %v", payload["temperature"])
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer server.Close()

	providerName, client, ok := NewChatClient(config.ModelProviderConfig{
		Provider: "kimi",
		BaseURL:  server.URL + "/v1",
		APIKey:   "kimi-key",
		Model:    "kimi-k2.5",
	})
	if !ok {
		t.Fatal("expected kimi provider to build a chat client")
	}
	if providerName != "kimi" {
		t.Fatalf("unexpected provider name %q", providerName)
	}

	var response struct {
		OK bool `json:"ok"`
	}
	if err := client.CompleteJSON(
		context.Background(),
		"你是测试助手。请返回 JSON。",
		"请返回 {\"ok\": true}",
		&response,
	); err != nil {
		t.Fatalf("complete json: %v", err)
	}
	if !response.OK {
		t.Fatal("expected ok=true in response")
	}
}
