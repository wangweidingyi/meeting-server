package udp

import (
	"context"
	"net"
	"testing"
	"time"

	"meeting-server/internal/protocol"
	"meeting-server/internal/session"
)

func TestHandleMixedAudioPublishesRealtimeDeltaEvents(t *testing.T) {
	manager := session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 6000,
	})

	if _, err := manager.HandleHello(session.HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "产品策略会",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	if _, err := manager.StartRecording("session-1"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	server := NewServer("127.0.0.1", 6000, manager)
	packet := protocol.UDPAudioPacket{
		Version:     1,
		SourceType:  protocol.AudioSourceMixed,
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}
	rawPacket, err := packet.Encode()
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	events, err := server.HandlePacketBytes(rawPacket)
	if err != nil {
		t.Fatalf("udp ingest failed: %v", err)
	}

	foundTranscriptDelta := false
	for _, event := range events {
		if event.Type == protocol.TypeSTTDelta {
			foundTranscriptDelta = true
		}
	}

	if !foundTranscriptDelta {
		t.Fatal("expected stt_delta event")
	}
}

func TestListenAndServeReceivesUDPPacketsFromSocket(t *testing.T) {
	manager := session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 0,
	})

	if _, err := manager.HandleHello(session.HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "UDP Socket Test",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	if _, err := manager.StartRecording("session-1"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	server := NewServer("127.0.0.1", 0, manager)
	repliesCh := make(chan []protocol.RoutedMessage, 1)
	server.SetMessageHandler(func(messages []protocol.RoutedMessage) {
		repliesCh <- messages
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe(ctx)
	}()

	address := server.WaitUntilListening(time.Second)
	if address == "" {
		t.Fatal("server did not start listening in time")
	}

	connection, err := net.Dial("udp", address)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer connection.Close()

	packet := protocol.UDPAudioPacket{
		Version:     1,
		SourceType:  protocol.AudioSourceMixed,
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}
	rawPacket, err := packet.Encode()
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	if _, err := connection.Write(rawPacket); err != nil {
		t.Fatalf("write udp packet: %v", err)
	}

	select {
	case replies := <-repliesCh:
		assertRealtimeTypes(t, replies)
	case <-time.After(time.Second):
		t.Fatal("expected routed messages from live udp listener")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("listen and serve returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

func TestListenAndServeIgnoresPacketsUntilSessionStartsRecording(t *testing.T) {
	manager := session.NewManager(session.Options{
		UDPHost: "127.0.0.1",
		UDPPort: 0,
	})

	if _, err := manager.HandleHello(session.HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "UDP Resilience Test",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	server := NewServer("127.0.0.1", 0, manager)
	repliesCh := make(chan []protocol.RoutedMessage, 1)
	server.SetMessageHandler(func(messages []protocol.RoutedMessage) {
		repliesCh <- messages
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe(ctx)
	}()

	address := server.WaitUntilListening(time.Second)
	if address == "" {
		t.Fatal("server did not start listening in time")
	}

	connection, err := net.Dial("udp", address)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer connection.Close()

	packet := protocol.UDPAudioPacket{
		Version:     1,
		SourceType:  protocol.AudioSourceMixed,
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}
	rawPacket, err := packet.Encode()
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	if _, err := connection.Write(rawPacket); err != nil {
		t.Fatalf("write udp packet before start: %v", err)
	}

	select {
	case replies := <-repliesCh:
		t.Fatalf("unexpected realtime replies before recording starts: %+v", replies)
	case err := <-errCh:
		t.Fatalf("server exited after pre-start packet: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := manager.StartRecording("session-1"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := connection.Write(rawPacket); err != nil {
		t.Fatalf("write udp packet after start: %v", err)
	}

	select {
	case replies := <-repliesCh:
		assertRealtimeTypes(t, replies)
	case err := <-errCh:
		t.Fatalf("server exited after recording started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("expected routed messages after session entered recording state")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("listen and serve returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

func assertRealtimeTypes(t *testing.T, events []protocol.RoutedMessage) {
	t.Helper()

	foundTranscriptDelta := false

	for _, event := range events {
		if event.Type == protocol.TypeSTTDelta {
			foundTranscriptDelta = true
		}
	}

	if !foundTranscriptDelta {
		t.Fatal("expected stt_delta event")
	}
}
