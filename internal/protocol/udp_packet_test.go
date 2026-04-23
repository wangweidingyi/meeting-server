package protocol

import "testing"

func TestUDPAudioPacketRoundTripsDesktopWireFormat(t *testing.T) {
	packet := UDPAudioPacket{
		Version:     1,
		SourceType:  AudioSourceMixed,
		SessionID:   "meeting-1",
		Sequence:    42,
		StartedAtMS: 1000,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}

	encoded, err := packet.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeUDPAudioPacket(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Sequence != 42 {
		t.Fatalf("unexpected sequence %d", decoded.Sequence)
	}
	if decoded.SourceType != AudioSourceMixed {
		t.Fatalf("unexpected source type %d", decoded.SourceType)
	}
	if decoded.StartedAtMS != 1000 {
		t.Fatalf("unexpected startedAt %d", decoded.StartedAtMS)
	}
	if decoded.DurationMS != 200 {
		t.Fatalf("unexpected duration %d", decoded.DurationMS)
	}
	if string(decoded.Payload) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("unexpected payload %v", decoded.Payload)
	}
}
