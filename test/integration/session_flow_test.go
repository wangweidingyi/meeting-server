package integration_test

import (
	"testing"

	"meeting-server/internal/app"
	"meeting-server/internal/protocol"
)

func TestSessionFlowProducesRealtimeAndFinalEvents(t *testing.T) {
	application := app.New()

	helloReplies, err := application.MQTTServer.HandleControlMessage(protocolAwareControl("session/hello", "client-a", "session-1", "集成测试会"))
	if err != nil {
		t.Fatalf("hello failed: %v", err)
	}
	if len(helloReplies) != 1 || helloReplies[0].Type != protocol.TypeSessionHello {
		t.Fatalf("unexpected hello replies %+v", helloReplies)
	}

	startReplies, err := application.MQTTServer.HandleControlMessage(protocolAwareControl("recording/start", "client-a", "session-1", ""))
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startReplies) != 1 || startReplies[0].Type != protocol.TypeRecordingStarted {
		t.Fatalf("unexpected start replies %+v", startReplies)
	}

	audioPacket, err := protocol.UDPAudioPacket{
		Version:     1,
		SourceType:  protocol.AudioSourceMixed,
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}.Encode()
	if err != nil {
		t.Fatalf("encode udp packet: %v", err)
	}

	realtimeReplies, err := application.UDPServer.HandlePacketBytes(audioPacket)
	if err != nil {
		t.Fatalf("udp pipeline failed: %v", err)
	}
	assertReplyTypes(t, realtimeReplies, protocol.TypeSTTDelta, protocol.TypeSummaryDelta, protocol.TypeActionItemDelta)

	stopReplies, err := application.MQTTServer.HandleControlMessage(protocolAwareControl("recording/stop", "client-a", "session-1", ""))
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	assertReplyTypes(t, stopReplies, protocol.TypeActionItemFinal, protocol.TypeSummaryFinal, protocol.TypeSTTFinal, protocol.TypeRecordingStopped)
}

func protocolAwareControl(messageType, clientID, sessionID, title string) struct {
	Type      string
	ClientID  string
	SessionID string
	Title     string
} {
	return struct {
		Type      string
		ClientID  string
		SessionID string
		Title     string
	}{
		Type:      messageType,
		ClientID:  clientID,
		SessionID: sessionID,
		Title:     title,
	}
}

func assertReplyTypes(t *testing.T, replies []protocol.RoutedMessage, expectedTypes ...string) {
	t.Helper()

	if len(replies) != len(expectedTypes) {
		t.Fatalf("expected %d replies, got %d (%+v)", len(expectedTypes), len(replies), replies)
	}

	for index, expectedType := range expectedTypes {
		if replies[index].Type != expectedType {
			t.Fatalf("reply %d expected %s, got %s", index, expectedType, replies[index].Type)
		}
	}
}
