package protocol

import "encoding/json"

const (
	TypeSessionHello     = "session/hello"
	TypeSessionResume    = "session/resume"
	TypeRecordingStart   = "recording/start"
	TypeRecordingPause   = "recording/pause"
	TypeRecordingResume  = "recording/resume"
	TypeRecordingStop    = "recording/stop"
	TypeRecordingStarted = "recording_started"
	TypeRecordingPaused  = "recording_paused"
	TypeRecordingResumed = "recording_resumed"
	TypeRecordingStopped = "recording_stopped"
	TypeHeartbeat        = "heartbeat"
	TypeSTTDelta         = "stt_delta"
	TypeSTTFinal         = "stt_final"
	TypeSummaryDelta     = "summary_delta"
	TypeSummaryFinal     = "summary_final"
	TypeActionItemDelta  = "action_item_delta"
	TypeActionItemFinal  = "action_item_final"
	TypeAck              = "ack"
	TypeError            = "error"
)

type Envelope[T any] struct {
	Version       string `json:"version"`
	MessageID     string `json:"messageId"`
	CorrelationID string `json:"correlationId,omitempty"`
	ClientID      string `json:"clientId"`
	SessionID     string `json:"sessionId"`
	Seq           uint64 `json:"seq"`
	SentAt        string `json:"sentAt"`
	Type          string `json:"type"`
	Payload       T      `json:"payload"`
}

type AudioFormat struct {
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

type TransportSelection struct {
	Control string `json:"control"`
	Audio   string `json:"audio"`
}

type FeatureFlags struct {
	RealtimeTranscript bool `json:"realtimeTranscript"`
	RealtimeSummary    bool `json:"realtimeSummary"`
	ActionItems        bool `json:"actionItems"`
}

type SessionHelloPayload struct {
	Audio     AudioFormat        `json:"audio"`
	Transport TransportSelection `json:"transport"`
	Features  FeatureFlags       `json:"features"`
	Title     string             `json:"title"`
}

type EmptyPayload struct{}

type RecordingStatePayload struct {
	State string `json:"state"`
}

type AckPayload struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

type MixedAudioPacket struct {
	ClientID    string `json:"clientId,omitempty"`
	SessionID   string `json:"sessionId"`
	Sequence    uint64 `json:"sequence"`
	StartedAtMS uint64 `json:"startedAtMs"`
	DurationMS  uint32 `json:"durationMs"`
	Payload     []byte `json:"payload"`
}

type TranscriptPayload struct {
	SegmentID string  `json:"segmentId"`
	StartMS   uint64  `json:"startMs"`
	EndMS     uint64  `json:"endMs"`
	Text      string  `json:"text"`
	IsFinal   bool    `json:"isFinal"`
	SpeakerID *string `json:"speakerId,omitempty"`
	Revision  uint64  `json:"revision"`
}

type SummaryPayload struct {
	Version      uint64   `json:"version"`
	UpdatedAt    string   `json:"updatedAt"`
	AbstractText string   `json:"abstract"`
	KeyPoints    []string `json:"keyPoints"`
	Decisions    []string `json:"decisions"`
	Risks        []string `json:"risks"`
	ActionItems  []string `json:"actionItems"`
	IsFinal      bool     `json:"isFinal"`
}

type ActionItemsPayload struct {
	Version   uint64   `json:"version"`
	UpdatedAt string   `json:"updatedAt"`
	Items     []string `json:"items"`
	IsFinal   bool     `json:"isFinal"`
}

type RoutedMessage struct {
	Topic   string
	Type    string
	Payload any
}

func EncodeEnvelope[T any](envelope Envelope[T]) ([]byte, error) {
	return json.Marshal(envelope)
}
