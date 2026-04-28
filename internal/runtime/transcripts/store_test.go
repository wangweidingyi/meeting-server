package transcripts

import "testing"

func TestMemoryStoreUpsertsLatestTranscriptSnapshotByMeeting(t *testing.T) {
	store := NewMemoryStore()

	if err := store.UpsertSnapshot(Snapshot{
		MeetingID: "meeting-1",
		SegmentID: "meeting-1-transcript",
		StartMS:   0,
		EndMS:     1_200,
		Text:      "先记录第一版转写",
		IsFinal:   false,
		Revision:  1,
	}); err != nil {
		t.Fatalf("upsert first snapshot: %v", err)
	}

	if err := store.UpsertSnapshot(Snapshot{
		MeetingID: "meeting-1",
		SegmentID: "meeting-1-transcript",
		StartMS:   0,
		EndMS:     1_400,
		Text:      "这是修正后的实时转写",
		IsFinal:   true,
		Revision:  2,
	}); err != nil {
		t.Fatalf("upsert second snapshot: %v", err)
	}

	snapshot, ok, err := store.LoadSnapshot("meeting-1")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected transcript snapshot to exist")
	}
	if snapshot.Text != "这是修正后的实时转写" {
		t.Fatalf("unexpected transcript text %q", snapshot.Text)
	}
	if snapshot.Revision != 2 {
		t.Fatalf("unexpected transcript revision %d", snapshot.Revision)
	}
	if !snapshot.IsFinal {
		t.Fatal("expected latest transcript snapshot to be final")
	}
}
