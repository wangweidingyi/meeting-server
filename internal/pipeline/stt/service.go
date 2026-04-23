package stt

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	openaicompat "meeting-server/internal/model/openai_compatible"
	"meeting-server/internal/protocol"
)

type sessionState struct {
	nextRevision          uint64
	firstStartMS          uint64
	lastEndMS             uint64
	lastDeltaEndMS        uint64
	chunks                []string
	cumulativePCM         []byte
	cumulativeTranscript  string
	packetsSinceRecognize int
}

type Service struct {
	mu           sync.Mutex
	sessions     map[string]*sessionState
	recognizer   Recognizer
	triggerEvery int
}

type Recognizer interface {
	Name() string
	Recognize(ctx context.Context, sessionID string, wave []byte, isFinal bool) (string, error)
}

type Option func(*Service)

func WithRecognizer(recognizer Recognizer) Option {
	return func(service *Service) {
		if recognizer != nil {
			service.recognizer = recognizer
		}
	}
}

func WithRecognitionTriggerPackets(triggerEvery int) Option {
	return func(service *Service) {
		if triggerEvery > 0 {
			service.triggerEvery = triggerEvery
		}
	}
}

func NewService(options ...Option) *Service {
	service := &Service{
		sessions:     make(map[string]*sessionState),
		recognizer:   StubRecognizer{},
		triggerEvery: 5,
	}

	for _, option := range options {
		option(service)
	}

	return service
}

func (s *Service) SetProvider(provider, baseURL, apiKey, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if provider == "openai_compatible" {
		s.recognizer = NewOpenAICompatibleRecognizer(&openaicompat.TranscriptionClient{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   model,
		})
		return
	}

	s.recognizer = StubRecognizer{}
}

func (s *Service) ProviderName() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.recognizer.Name()
}

func (s *Service) Consume(packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool) {
	if s.ProviderName() == "stub" {
		return s.consumeStub(packet)
	}

	return s.consumeWithRecognizer(packet)
}

func (s *Service) Flush(sessionID string) (protocol.TranscriptPayload, bool) {
	if s.ProviderName() == "stub" {
		return s.flushStub(sessionID)
	}

	return s.flushWithRecognizer(sessionID)
}

func (s *Service) consumeStub(packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(packet.Payload) == 0 {
		return protocol.TranscriptPayload{}, false
	}

	state := s.ensureState(packet.SessionID)
	if state.nextRevision == 0 {
		state.firstStartMS = packet.StartedAtMS
	}

	state.nextRevision++
	state.lastEndMS = packet.StartedAtMS + uint64(packet.DurationMS)

	text := fmt.Sprintf("[stub-stt] chunk-%d bytes=%d", packet.Sequence, len(packet.Payload))
	state.chunks = append(state.chunks, text)

	return protocol.TranscriptPayload{
		SegmentID: fmt.Sprintf("%s-%d", packet.SessionID, packet.Sequence),
		StartMS:   packet.StartedAtMS,
		EndMS:     state.lastEndMS,
		Text:      text,
		IsFinal:   false,
		Revision:  state.nextRevision,
	}, true
}

func (s *Service) flushStub(sessionID string) (protocol.TranscriptPayload, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[sessionID]
	if !ok || len(state.chunks) == 0 {
		return protocol.TranscriptPayload{}, false
	}

	state.nextRevision++
	payload := protocol.TranscriptPayload{
		SegmentID: fmt.Sprintf("%s-final", sessionID),
		StartMS:   state.firstStartMS,
		EndMS:     state.lastEndMS,
		Text:      strings.Join(state.chunks, " "),
		IsFinal:   true,
		Revision:  state.nextRevision,
	}

	delete(s.sessions, sessionID)
	return payload, true
}

func (s *Service) consumeWithRecognizer(packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool) {
	if len(packet.Payload) == 0 {
		return protocol.TranscriptPayload{}, false
	}

	s.mu.Lock()
	state := s.ensureState(packet.SessionID)
	if state.nextRevision == 0 {
		state.firstStartMS = packet.StartedAtMS
	}

	state.lastEndMS = packet.StartedAtMS + uint64(packet.DurationMS)
	state.packetsSinceRecognize++
	state.cumulativePCM = append(state.cumulativePCM, packet.Payload...)
	shouldRecognize := state.packetsSinceRecognize >= s.triggerEvery
	audio := append([]byte(nil), state.cumulativePCM...)
	previousTranscript := state.cumulativeTranscript
	firstStartMS := state.firstStartMS
	lastDeltaEndMS := state.lastDeltaEndMS
	lastEndMS := state.lastEndMS
	recognizer := s.recognizer
	s.mu.Unlock()

	if !shouldRecognize {
		return protocol.TranscriptPayload{}, false
	}

	text, err := recognizer.Recognize(
		context.Background(),
		packet.SessionID,
		encodePCM16MonoWave(audio, 16_000, 1),
		false,
	)
	if err != nil {
		return protocol.TranscriptPayload{}, false
	}

	delta := transcriptDelta(previousTranscript, text)
	if delta == "" {
		s.mu.Lock()
		state = s.ensureState(packet.SessionID)
		state.cumulativeTranscript = text
		state.packetsSinceRecognize = 0
		s.mu.Unlock()
		return protocol.TranscriptPayload{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state = s.ensureState(packet.SessionID)
	state.nextRevision++
	state.cumulativeTranscript = text
	state.packetsSinceRecognize = 0
	state.lastDeltaEndMS = lastEndMS

	startMS := firstStartMS
	if lastDeltaEndMS != 0 {
		startMS = lastDeltaEndMS
	}

	return protocol.TranscriptPayload{
		SegmentID: fmt.Sprintf("%s-%d", packet.SessionID, state.nextRevision),
		StartMS:   startMS,
		EndMS:     lastEndMS,
		Text:      delta,
		IsFinal:   false,
		Revision:  state.nextRevision,
	}, true
}

func (s *Service) flushWithRecognizer(sessionID string) (protocol.TranscriptPayload, bool) {
	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok || len(state.cumulativePCM) == 0 {
		s.mu.Unlock()
		return protocol.TranscriptPayload{}, false
	}

	audio := append([]byte(nil), state.cumulativePCM...)
	firstStartMS := state.firstStartMS
	lastEndMS := state.lastEndMS
	previousTranscript := state.cumulativeTranscript
	recognizer := s.recognizer
	s.mu.Unlock()

	text, err := recognizer.Recognize(
		context.Background(),
		sessionID,
		encodePCM16MonoWave(audio, 16_000, 1),
		true,
	)
	if err != nil || strings.TrimSpace(text) == "" {
		text = previousTranscript
	}
	if strings.TrimSpace(text) == "" {
		return protocol.TranscriptPayload{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state = s.ensureState(sessionID)
	state.nextRevision++
	revision := state.nextRevision
	delete(s.sessions, sessionID)

	return protocol.TranscriptPayload{
		SegmentID: fmt.Sprintf("%s-final", sessionID),
		StartMS:   firstStartMS,
		EndMS:     lastEndMS,
		Text:      text,
		IsFinal:   true,
		Revision:  revision,
	}, true
}

func (s *Service) ensureState(sessionID string) *sessionState {
	state, ok := s.sessions[sessionID]
	if !ok {
		state = &sessionState{}
		s.sessions[sessionID] = state
	}

	return state
}

type StubRecognizer struct{}

func (StubRecognizer) Name() string {
	return "stub"
}

func (StubRecognizer) Recognize(_ context.Context, _ string, _ []byte, _ bool) (string, error) {
	return "", nil
}

type OpenAICompatibleRecognizer struct {
	client *openaicompat.TranscriptionClient
}

func NewOpenAICompatibleRecognizer(client *openaicompat.TranscriptionClient) *OpenAICompatibleRecognizer {
	return &OpenAICompatibleRecognizer{client: client}
}

func (r *OpenAICompatibleRecognizer) Name() string {
	return "openai_compatible"
}

func (r *OpenAICompatibleRecognizer) Recognize(ctx context.Context, _ string, wave []byte, _ bool) (string, error) {
	return r.client.Recognize(ctx, wave)
}

func transcriptDelta(previous, current string) string {
	trimmedPrevious := strings.TrimSpace(previous)
	trimmedCurrent := strings.TrimSpace(current)
	if trimmedCurrent == "" {
		return ""
	}
	if trimmedPrevious == "" {
		return trimmedCurrent
	}
	if strings.HasPrefix(trimmedCurrent, trimmedPrevious) {
		return strings.TrimSpace(strings.TrimPrefix(trimmedCurrent, trimmedPrevious))
	}

	return trimmedCurrent
}

func encodePCM16MonoWave(pcm []byte, sampleRateHz uint32, channels uint16) []byte {
	dataLength := uint32(len(pcm))
	blockAlign := channels * 2
	byteRate := sampleRateHz * uint32(blockAlign)
	riffSize := uint32(36) + dataLength

	buffer := bytes.NewBuffer(make([]byte, 0, int(44+dataLength)))
	buffer.WriteString("RIFF")
	_ = binary.Write(buffer, binary.LittleEndian, riffSize)
	buffer.WriteString("WAVE")
	buffer.WriteString("fmt ")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, channels)
	_ = binary.Write(buffer, binary.LittleEndian, sampleRateHz)
	_ = binary.Write(buffer, binary.LittleEndian, byteRate)
	_ = binary.Write(buffer, binary.LittleEndian, blockAlign)
	_ = binary.Write(buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	_ = binary.Write(buffer, binary.LittleEndian, dataLength)
	buffer.Write(pcm)

	return buffer.Bytes()
}
