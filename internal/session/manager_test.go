package session

import (
	"testing"

	"meeting-server/internal/protocol"
)

func TestHelloAllocatesSessionAndUdpDetails(t *testing.T) {
	manager := NewManager(Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	})

	reply, err := manager.HandleHello(HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "产品策略会",
	})
	if err != nil {
		t.Fatalf("unexpected hello error: %v", err)
	}

	if reply.Type != "session/hello" {
		t.Fatalf("unexpected reply type %s", reply.Type)
	}

	if reply.UDP.Server != "127.0.0.1" || reply.UDP.Port != 6000 {
		t.Fatalf("unexpected udp details %+v", reply.UDP)
	}
}

func TestLifecycleTransitionsFollowHelloStartHeartbeatStop(t *testing.T) {
	manager := NewManager(Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	})

	if _, err := manager.HandleHello(HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "客户复盘会",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	started, err := manager.StartRecording("session-1")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if started.Type != "recording_started" {
		t.Fatalf("unexpected start event type %s", started.Type)
	}

	paused, err := manager.PauseRecording("session-1")
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if paused.Type != "recording_paused" {
		t.Fatalf("unexpected pause event type %s", paused.Type)
	}

	resumed, err := manager.ResumeRecording("session-1")
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if resumed.Type != "recording_resumed" {
		t.Fatalf("unexpected resume event type %s", resumed.Type)
	}

	heartbeat, err := manager.Heartbeat("session-1")
	if err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	if heartbeat.Type != "heartbeat" {
		t.Fatalf("unexpected heartbeat event type %s", heartbeat.Type)
	}

	if _, err := manager.HandleMixedAudio(protocol.MixedAudioPacket{
		ClientID:    "client-a",
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}); err != nil {
		t.Fatalf("audio ingest failed: %v", err)
	}

	stopped, err := manager.StopRecording("session-1")
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	foundStopped := false
	foundTranscriptFinal := false
	foundSummaryFinal := false
	foundActionItemFinal := false

	for _, event := range stopped {
		switch event.Type {
		case protocol.TypeRecordingStopped:
			foundStopped = true
		case protocol.TypeSTTFinal:
			foundTranscriptFinal = true
		case protocol.TypeSummaryFinal:
			foundSummaryFinal = true
		case protocol.TypeActionItemFinal:
			foundActionItemFinal = true
		}
	}

	if !foundStopped {
		t.Fatal("expected recording_stopped event")
	}
	if !foundTranscriptFinal {
		t.Fatal("expected stt_final event")
	}
	if !foundSummaryFinal {
		t.Fatal("expected summary_final event")
	}
	if !foundActionItemFinal {
		t.Fatal("expected action_item_final event")
	}
}
