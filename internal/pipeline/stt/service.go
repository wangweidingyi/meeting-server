package stt

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	"meeting-server/internal/config"
	openaicompat "meeting-server/internal/model/openai_compatible"
	"meeting-server/internal/protocol"
)

type sessionState struct {
	nextRevision          uint64
	firstStartMS          uint64
	lastEndMS             uint64
	chunks                []string
	cumulativePCM         []byte
	cumulativeTranscript  string
	packetsSinceRecognize int
}

type StreamSession interface {
	Consume(ctx context.Context, packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool, error)
	Flush(ctx context.Context, sessionID string) (protocol.TranscriptPayload, bool, error)
	Close() error
}

type StreamFactory interface {
	Name() string
	NewSession(sessionID string) StreamSession
}

type Service struct {
	mu             sync.Mutex
	sessions       map[string]*sessionState
	streamSessions map[string]StreamSession
	recognizer     Recognizer
	streamFactory  StreamFactory
	providerName   string
	triggerEvery   int
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
			service.streamFactory = nil
			service.providerName = recognizer.Name()
		}
	}
}

func WithStreamFactory(factory StreamFactory) Option {
	return func(service *Service) {
		if factory != nil {
			service.streamFactory = factory
			service.providerName = factory.Name()
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
		sessions:       make(map[string]*sessionState),
		streamSessions: make(map[string]StreamSession),
		recognizer:     StubRecognizer{},
		providerName:   "stub",
		triggerEvery:   5,
	}

	for _, option := range options {
		option(service)
	}

	if service.providerName == "" {
		service.providerName = service.recognizer.Name()
	}

	return service
}

func (s *Service) SetProvider(provider, baseURL, apiKey, model string) {
	s.SetConfig(config.STTProviderConfig{
		Provider: provider,
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    model,
	})
}

func (s *Service) SetConfig(cfg config.STTProviderConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sessionID, streamSession := range s.streamSessions {
		_ = streamSession.Close()
		delete(s.streamSessions, sessionID)
	}
	s.sessions = make(map[string]*sessionState)

	switch cfg.Provider {
	case "openai_compatible":
		s.recognizer = NewOpenAICompatibleRecognizer(&openaicompat.TranscriptionClient{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		})
		s.streamFactory = nil
		s.providerName = "openai_compatible"
	case "volcengine_streaming":
		s.recognizer = StubRecognizer{}
		s.streamFactory = NewVolcengineStreamingFactory(cfg)
		s.providerName = "volcengine_streaming"
	default:
		s.recognizer = StubRecognizer{}
		s.streamFactory = nil
		s.providerName = "stub"
	}
}

func (s *Service) ProviderName() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.providerName
}

func (s *Service) Consume(packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool) {
	if s.ProviderName() == "stub" {
		return s.consumeStub(packet)
	}
	if s.hasStreamFactory() {
		return s.consumeWithStreamSession(packet)
	}

	return s.consumeWithRecognizer(packet)
}

func (s *Service) Flush(sessionID string) (protocol.TranscriptPayload, bool) {
	if s.ProviderName() == "stub" {
		return s.flushStub(sessionID)
	}
	if s.hasStreamFactory() {
		return s.flushWithStreamSession(sessionID)
	}

	return s.flushWithRecognizer(sessionID)
}

func (s *Service) hasStreamFactory() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.streamFactory != nil
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
	fullTranscript := strings.Join(state.chunks, " ")

	return protocol.TranscriptPayload{
		SegmentID: liveTranscriptSegmentID(packet.SessionID),
		StartMS:   state.firstStartMS,
		EndMS:     state.lastEndMS,
		Text:      fullTranscript,
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
		SegmentID: liveTranscriptSegmentID(sessionID),
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

	text = strings.TrimSpace(text)
	if text == "" || text == strings.TrimSpace(previousTranscript) {
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

	return protocol.TranscriptPayload{
		SegmentID: liveTranscriptSegmentID(packet.SessionID),
		StartMS:   firstStartMS,
		EndMS:     lastEndMS,
		Text:      text,
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
		SegmentID: liveTranscriptSegmentID(sessionID),
		StartMS:   firstStartMS,
		EndMS:     lastEndMS,
		Text:      text,
		IsFinal:   true,
		Revision:  revision,
	}, true
}

func (s *Service) consumeWithStreamSession(packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool) {
	s.mu.Lock()
	session, ok := s.streamSessions[packet.SessionID]
	if !ok {
		session = s.streamFactory.NewSession(packet.SessionID)
		s.streamSessions[packet.SessionID] = session
	}
	s.mu.Unlock()

	payload, ok, err := session.Consume(context.Background(), packet)
	if err != nil {
		return protocol.TranscriptPayload{}, false
	}

	return payload, ok
}

func (s *Service) flushWithStreamSession(sessionID string) (protocol.TranscriptPayload, bool) {
	s.mu.Lock()
	session, ok := s.streamSessions[sessionID]
	if ok {
		delete(s.streamSessions, sessionID)
	}
	s.mu.Unlock()

	if !ok {
		return protocol.TranscriptPayload{}, false
	}
	defer func() {
		_ = session.Close()
	}()

	payload, ok, err := session.Flush(context.Background(), sessionID)
	if err != nil {
		return protocol.TranscriptPayload{}, false
	}

	return payload, ok
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

func liveTranscriptSegmentID(sessionID string) string {
	return fmt.Sprintf("%s-transcript", sessionID)
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
