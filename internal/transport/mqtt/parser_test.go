package mqtt

import (
	"strings"
	"testing"

	"meeting-server/internal/session"
)

func TestParseControlEnvelopeExtractsHelloFields(t *testing.T) {
	message, err := ParseControlEnvelope([]byte(`{
		"version": "v1",
		"messageId": "msg-1",
		"clientId": "client-a",
		"sessionId": "session-1",
		"type": "session/hello",
		"payload": {
			"title": "架构评审会"
		}
	}`))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if message.Type != "session/hello" || message.ClientID != "client-a" || message.Title != "架构评审会" {
		t.Fatalf("unexpected parsed message %+v", message)
	}
}

func TestEncodeRoutedReplyWrapsHelloInEnvelope(t *testing.T) {
	server := NewServer(session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	}))

	reply, err := server.HandleControlMessage(ControlMessage{
		Type:      "session/hello",
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "架构评审会",
	})
	if err != nil {
		t.Fatalf("unexpected routing error: %v", err)
	}
	if len(reply) != 1 {
		t.Fatalf("expected one reply, got %d", len(reply))
	}

	encoded, err := EncodeRoutedReply(reply[0])
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}

	body := string(encoded)
	if !strings.Contains(body, "\"type\":\"session/hello\"") {
		t.Fatalf("encoded body missing hello type: %s", body)
	}
	if !strings.Contains(body, "\"clientId\":\"client-a\"") {
		t.Fatalf("encoded body missing client id: %s", body)
	}
}

func TestParseControlEnvelopeExtractsSessionResumeFields(t *testing.T) {
	message, err := ParseControlEnvelope([]byte(`{
		"version": "v1",
		"messageId": "msg-2",
		"clientId": "client-a",
		"sessionId": "session-1",
		"type": "session/resume",
		"payload": {}
	}`))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if message.Type != "session/resume" || message.ClientID != "client-a" || message.SessionID != "session-1" {
		t.Fatalf("unexpected parsed message %+v", message)
	}
}
