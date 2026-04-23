package mqtt

import (
	"testing"

	"meeting-server/internal/session"
)

func TestHandleControlMessageRoutesHello(t *testing.T) {
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
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reply) != 1 {
		t.Fatalf("expected one reply, got %d", len(reply))
	}

	if reply[0].Topic != "meetings/client-a/session/session-1/control/reply" {
		t.Fatalf("unexpected reply topic %s", reply[0].Topic)
	}

	if reply[0].Type != "session/hello" {
		t.Fatalf("unexpected reply type %s", reply[0].Type)
	}
}

func TestHandleControlMessageRoutesLifecycleEvents(t *testing.T) {
	server := NewServer(session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	}))

	_, err := server.HandleControlMessage(ControlMessage{
		Type:      "session/hello",
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "架构评审会",
	})
	if err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	reply, err := server.HandleControlMessage(ControlMessage{
		Type:      "recording/start",
		ClientID:  "client-a",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if len(reply) != 1 {
		t.Fatalf("expected one reply, got %d", len(reply))
	}

	if reply[0].Topic != "meetings/client-a/session/session-1/events" {
		t.Fatalf("unexpected event topic %s", reply[0].Topic)
	}

	if reply[0].Type != "recording_started" {
		t.Fatalf("unexpected lifecycle type %s", reply[0].Type)
	}

	pausedReply, err := server.HandleControlMessage(ControlMessage{
		Type:      "recording/pause",
		ClientID:  "client-a",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	if len(pausedReply) != 1 || pausedReply[0].Type != "recording_paused" {
		t.Fatalf("unexpected pause reply %+v", pausedReply)
	}

	resumedReply, err := server.HandleControlMessage(ControlMessage{
		Type:      "recording/resume",
		ClientID:  "client-a",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	if len(resumedReply) != 1 || resumedReply[0].Type != "recording_resumed" {
		t.Fatalf("unexpected resume reply %+v", resumedReply)
	}
}

func TestHandleControlEnvelopeRoutesParsedPayload(t *testing.T) {
	server := NewServer(session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	}))

	replies, err := server.HandleControlEnvelope([]byte(`{
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
		t.Fatalf("unexpected error: %v", err)
	}

	if len(replies) != 1 {
		t.Fatalf("expected one routed reply, got %d", len(replies))
	}

	if replies[0].Topic != "meetings/client-a/session/session-1/control/reply" {
		t.Fatalf("unexpected reply topic %s", replies[0].Topic)
	}
}

func TestHandleControlMessageRoutesSessionResumeAck(t *testing.T) {
	server := NewServer(session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	}))

	_, err := server.HandleControlMessage(ControlMessage{
		Type:      "session/hello",
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "恢复测试会",
	})
	if err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	reply, err := server.HandleControlMessage(ControlMessage{
		Type:      "session/resume",
		ClientID:  "client-a",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	if len(reply) != 1 {
		t.Fatalf("expected one reply, got %d", len(reply))
	}
	if reply[0].Topic != "meetings/client-a/session/session-1/control/reply" {
		t.Fatalf("unexpected reply topic %s", reply[0].Topic)
	}
	if reply[0].Type != "ack" {
		t.Fatalf("unexpected reply type %s", reply[0].Type)
	}
}

func TestHandleControlMessageReturnsStructuredErrorForInvalidLifecycle(t *testing.T) {
	server := NewServer(session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	}))

	reply, err := server.HandleControlMessage(ControlMessage{
		Type:      "recording/start",
		ClientID:  "client-a",
		SessionID: "missing-session",
	})
	if err != nil {
		t.Fatalf("expected structured error reply, got error: %v", err)
	}

	if len(reply) != 1 {
		t.Fatalf("expected one error reply, got %d", len(reply))
	}
	if reply[0].Topic != "meetings/client-a/session/missing-session/control/reply" {
		t.Fatalf("unexpected reply topic %s", reply[0].Topic)
	}
	if reply[0].Type != "error" {
		t.Fatalf("unexpected reply type %s", reply[0].Type)
	}
}
