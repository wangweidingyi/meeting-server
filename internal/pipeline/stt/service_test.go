package stt

import (
	"context"
	"fmt"
	"testing"

	"meeting-server/internal/protocol"
)

type fakeRecognizer struct {
	name   string
	texts  []string
	calls  int
	audio  [][]byte
	finals []bool
}

func (r *fakeRecognizer) Name() string {
	return r.name
}

func (r *fakeRecognizer) Recognize(_ context.Context, _ string, audio []byte, isFinal bool) (string, error) {
	r.calls++
	r.audio = append(r.audio, append([]byte(nil), audio...))
	r.finals = append(r.finals, isFinal)
	if r.calls > len(r.texts) {
		return "", fmt.Errorf("unexpected recognize call %d", r.calls)
	}

	return r.texts[r.calls-1], nil
}

func TestOpenAICompatibleRecognizerProducesRealtimeTranscriptSnapshots(t *testing.T) {
	recognizer := &fakeRecognizer{
		name:  "openai_compatible",
		texts: []string{"大家好", "大家好 今天讨论预算", "大家好 今天讨论预算"},
	}

	service := NewService(
		WithRecognizer(recognizer),
		WithRecognitionTriggerPackets(2),
	)

	if _, ok := service.Consume(testPacket(1, 0)); ok {
		t.Fatal("expected first packet to stay buffered")
	}

	firstDelta, ok := service.Consume(testPacket(2, 200))
	if !ok {
		t.Fatal("expected second packet to emit first delta")
	}
	if firstDelta.Text != "大家好" {
		t.Fatalf("unexpected first delta %q", firstDelta.Text)
	}
	if firstDelta.SegmentID != "session-1-transcript" {
		t.Fatalf("unexpected first segment id %q", firstDelta.SegmentID)
	}

	if _, ok := service.Consume(testPacket(3, 400)); ok {
		t.Fatal("expected third packet to stay buffered")
	}

	secondDelta, ok := service.Consume(testPacket(4, 600))
	if !ok {
		t.Fatal("expected fourth packet to emit second delta")
	}
	if secondDelta.Text != "大家好 今天讨论预算" {
		t.Fatalf("unexpected second delta %q", secondDelta.Text)
	}
	if secondDelta.SegmentID != firstDelta.SegmentID {
		t.Fatalf("expected second revision to reuse segment id, got %q vs %q", secondDelta.SegmentID, firstDelta.SegmentID)
	}
	if secondDelta.Revision <= firstDelta.Revision {
		t.Fatalf("expected second revision to increase, got %d after %d", secondDelta.Revision, firstDelta.Revision)
	}

	finalPayload, ok := service.Flush("session-1")
	if !ok {
		t.Fatal("expected final transcript on flush")
	}
	if !finalPayload.IsFinal {
		t.Fatal("expected final payload to be marked final")
	}
	if finalPayload.Text != "大家好 今天讨论预算" {
		t.Fatalf("unexpected final transcript %q", finalPayload.Text)
	}
	if finalPayload.SegmentID != firstDelta.SegmentID {
		t.Fatalf("expected final transcript to reuse segment id, got %q vs %q", finalPayload.SegmentID, firstDelta.SegmentID)
	}
	if finalPayload.Revision <= secondDelta.Revision {
		t.Fatalf("expected final revision to increase, got %d after %d", finalPayload.Revision, secondDelta.Revision)
	}
	if recognizer.calls != 3 {
		t.Fatalf("expected 3 recognizer calls, got %d", recognizer.calls)
	}
	if !recognizer.finals[2] {
		t.Fatal("expected flush call to mark final recognition")
	}
	if len(recognizer.audio[1]) <= len(recognizer.audio[0]) {
		t.Fatal("expected cumulative audio to grow between recognizer calls")
	}
}

func testPacket(sequence uint64, startedAtMS uint64) protocol.MixedAudioPacket {
	return protocol.MixedAudioPacket{
		SessionID:   "session-1",
		Sequence:    sequence,
		StartedAtMS: startedAtMS,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}
}
