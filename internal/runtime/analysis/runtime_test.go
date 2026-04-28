package analysis

import (
	"testing"
	"time"

	"meeting-server/internal/pipeline/action_items"
	"meeting-server/internal/pipeline/summary"
	"meeting-server/internal/protocol"
	"meeting-server/internal/runtime/transcripts"
)

func TestRuntimePublishesSummaryAndActionItemsFromLatestTranscriptSnapshot(t *testing.T) {
	transcriptStore := transcripts.NewMemoryStore()
	if err := transcriptStore.UpsertSnapshot(transcripts.Snapshot{
		MeetingID: "meeting-1",
		SegmentID: "meeting-1-transcript",
		StartMS:   0,
		EndMS:     1_200,
		Text:      "确认预算和发布时间",
		IsFinal:   false,
		Revision:  2,
	}); err != nil {
		t.Fatalf("seed transcript snapshot: %v", err)
	}

	publisher := &recordingPublisher{}
	runtime := NewRuntime(Options{
		TranscriptStore: transcriptStore,
		ActionItems:     action_items.NewService(),
		Summary:         summary.NewService(),
		Publisher:       publisher,
	})

	runtime.Schedule("client-a", "meeting-1", false)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if publisher.hasType(protocol.TypeSummaryDelta) && publisher.hasType(protocol.TypeActionItemDelta) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected async runtime to publish summary and action-item deltas, got %+v", publisher.messages)
}

type recordingPublisher struct {
	messages []protocol.RoutedMessage
}

func (p *recordingPublisher) Publish(messages []protocol.RoutedMessage) {
	p.messages = append(p.messages, messages...)
}

func (p *recordingPublisher) hasType(messageType string) bool {
	for _, message := range p.messages {
		if message.Type == messageType {
			return true
		}
	}

	return false
}
