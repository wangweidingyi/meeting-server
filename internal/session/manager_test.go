package session

import (
	"testing"

	"meeting-server/internal/protocol"
	"meeting-server/internal/runtime/transcripts"
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

	for _, event := range stopped {
		switch event.Type {
		case protocol.TypeRecordingStopped:
			foundStopped = true
		case protocol.TypeSTTFinal:
			foundTranscriptFinal = true
		}
	}

	if !foundStopped {
		t.Fatal("expected recording_stopped event")
	}
	if !foundTranscriptFinal {
		t.Fatal("expected stt_final event")
	}
}

func TestHandleMixedAudioReturnsTranscriptImmediatelyAndSchedulesAnalysis(t *testing.T) {
	transcriptStore := &recordingTranscriptStore{}
	analysisScheduler := &recordingAnalysisScheduler{}
	manager := NewManager(Options{
		UDPHost:           "127.0.0.1",
		UDPPort:           6000,
		TranscriptStore:   transcriptStore,
		AnalysisScheduler: analysisScheduler,
	})

	if _, err := manager.HandleHello(HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "客户复盘会",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	if _, err := manager.StartRecording("session-1"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	replies, err := manager.HandleMixedAudio(protocol.MixedAudioPacket{
		ClientID:    "client-a",
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	})
	if err != nil {
		t.Fatalf("audio ingest failed: %v", err)
	}

	if len(replies) != 1 || replies[0].Type != protocol.TypeSTTDelta {
		t.Fatalf("expected only immediate stt_delta, got %+v", replies)
	}
	if len(transcriptStore.upserts) != 1 {
		t.Fatalf("expected one transcript upsert, got %d", len(transcriptStore.upserts))
	}
	if len(analysisScheduler.jobs) != 1 {
		t.Fatalf("expected one analysis job, got %d", len(analysisScheduler.jobs))
	}
	if analysisScheduler.jobs[0].IsFinal {
		t.Fatal("expected realtime analysis job to be non-final")
	}
}

func TestStopRecordingSchedulesFinalAnalysisWithoutBlockingOnSummary(t *testing.T) {
	transcriptStore := &recordingTranscriptStore{}
	analysisScheduler := &recordingAnalysisScheduler{}
	manager := NewManager(Options{
		UDPHost:           "127.0.0.1",
		UDPPort:           6000,
		TranscriptStore:   transcriptStore,
		AnalysisScheduler: analysisScheduler,
	})

	if _, err := manager.HandleHello(HelloRequest{
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "客户复盘会",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	if _, err := manager.StartRecording("session-1"); err != nil {
		t.Fatalf("start failed: %v", err)
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

	replies, err := manager.StopRecording("session-1")
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	if len(replies) != 2 {
		t.Fatalf("expected transcript final and recording stopped replies, got %+v", replies)
	}
	if replies[0].Type != protocol.TypeSTTFinal {
		t.Fatalf("expected first reply stt_final, got %s", replies[0].Type)
	}
	if replies[1].Type != protocol.TypeRecordingStopped {
		t.Fatalf("expected second reply recording_stopped, got %s", replies[1].Type)
	}
	if len(analysisScheduler.jobs) != 2 {
		t.Fatalf("expected realtime and final analysis jobs, got %d", len(analysisScheduler.jobs))
	}
	if !analysisScheduler.jobs[1].IsFinal {
		t.Fatal("expected second analysis job to be final")
	}
}

type recordingTranscriptStore struct {
	upserts []transcripts.Snapshot
}

func (s *recordingTranscriptStore) UpsertSnapshot(snapshot transcripts.Snapshot) error {
	s.upserts = append(s.upserts, snapshot)
	return nil
}

func (s *recordingTranscriptStore) LoadSnapshot(_ string) (transcripts.Snapshot, bool, error) {
	if len(s.upserts) == 0 {
		return transcripts.Snapshot{}, false, nil
	}

	return s.upserts[len(s.upserts)-1], true, nil
}

type recordingAnalysisScheduler struct {
	jobs []scheduledAnalysisJob
}

type scheduledAnalysisJob struct {
	ClientID  string
	SessionID string
	IsFinal   bool
}

func (s *recordingAnalysisScheduler) Schedule(clientID, sessionID string, isFinal bool) {
	s.jobs = append(s.jobs, scheduledAnalysisJob{
		ClientID:  clientID,
		SessionID: sessionID,
		IsFinal:   isFinal,
	})
}
