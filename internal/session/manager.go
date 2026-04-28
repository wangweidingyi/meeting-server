package session

import (
	"errors"
	"sync"
	"time"

	"meeting-server/internal/pipeline/action_items"
	"meeting-server/internal/pipeline/stt"
	"meeting-server/internal/pipeline/summary"
	"meeting-server/internal/protocol"
	"meeting-server/internal/runtime/transcripts"
)

type Options struct {
	UDPHost            string
	UDPPort            int
	STTService         *stt.Service
	SummaryService     *summary.Service
	ActionItemsService *action_items.Service
	TranscriptStore    TranscriptStore
	AnalysisScheduler  AnalysisScheduler
}

type TranscriptStore interface {
	UpsertSnapshot(snapshot transcripts.Snapshot) error
}

type AnalysisScheduler interface {
	Schedule(clientID, sessionID string, isFinal bool)
}

type HelloRequest struct {
	ClientID  string
	SessionID string
	Title     string
}

type UDPDetails struct {
	Server string `json:"server"`
	Port   int    `json:"port"`
}

type HelloReply struct {
	Type      string     `json:"type"`
	ClientID  string     `json:"clientId"`
	SessionID string     `json:"sessionId"`
	UDP       UDPDetails `json:"udp"`
}

type SessionEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	SentAt    string `json:"sentAt"`
}

type Manager struct {
	mu                 sync.RWMutex
	options            Options
	sessions           map[string]*SessionState
	sttService         *stt.Service
	summaryService     *summary.Service
	actionItemsService *action_items.Service
	transcriptStore    TranscriptStore
	analysisScheduler  AnalysisScheduler
}

type SessionState struct {
	ClientID      string
	SessionID     string
	Title         string
	Status        string
	LastHeartbeat time.Time
}

func NewManager(options Options) *Manager {
	sttService := options.STTService
	if sttService == nil {
		sttService = stt.NewService()
	}

	summaryService := options.SummaryService
	if summaryService == nil {
		summaryService = summary.NewService()
	}

	actionItemsService := options.ActionItemsService
	if actionItemsService == nil {
		actionItemsService = action_items.NewService()
	}

	return &Manager{
		options:            options,
		sessions:           make(map[string]*SessionState),
		sttService:         sttService,
		summaryService:     summaryService,
		actionItemsService: actionItemsService,
		transcriptStore:    options.TranscriptStore,
		analysisScheduler:  options.AnalysisScheduler,
	}
}

func (m *Manager) HandleHello(request HelloRequest) (HelloReply, error) {
	if request.ClientID == "" || request.SessionID == "" {
		return HelloReply{}, errors.New("client_id and session_id are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[request.SessionID] = &SessionState{
		ClientID:      request.ClientID,
		SessionID:     request.SessionID,
		Title:         request.Title,
		Status:        "ready",
		LastHeartbeat: time.Now(),
	}

	return HelloReply{
		Type:      protocol.TypeSessionHello,
		ClientID:  request.ClientID,
		SessionID: request.SessionID,
		UDP: UDPDetails{
			Server: m.options.UDPHost,
			Port:   m.options.UDPPort,
		},
	}, nil
}

func (m *Manager) ResumeSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return errors.New("session not found")
	}

	session.LastHeartbeat = time.Now()
	return nil
}

func (m *Manager) StartRecording(sessionID string) (SessionEvent, error) {
	return m.transition(sessionID, "recording", protocol.TypeRecordingStarted)
}

func (m *Manager) PauseRecording(sessionID string) (SessionEvent, error) {
	return m.transition(sessionID, "paused", protocol.TypeRecordingPaused)
}

func (m *Manager) ResumeRecording(sessionID string) (SessionEvent, error) {
	return m.transition(sessionID, "recording", protocol.TypeRecordingResumed)
}

func (m *Manager) StopRecording(sessionID string) ([]protocol.RoutedMessage, error) {
	session, err := m.setStatus(sessionID, "completed")
	if err != nil {
		return nil, err
	}

	messages := make([]protocol.RoutedMessage, 0, 2)

	if transcriptResult, ok := m.sttService.Flush(sessionID); ok {
		if err := m.persistTranscriptSnapshot(sessionID, transcriptResult); err != nil {
			return nil, err
		}
		messages = append(messages, protocol.RoutedMessage{
			Topic:   protocol.SttTopic(session.ClientID, session.SessionID),
			Type:    protocol.TypeSTTFinal,
			Payload: transcriptResult,
		})
		m.scheduleAnalysis(session.ClientID, session.SessionID, true)
	}

	messages = append(messages, protocol.RoutedMessage{
		Topic: protocol.EventsTopic(session.ClientID, session.SessionID),
		Type:  protocol.TypeRecordingStopped,
		Payload: SessionEvent{
			Type:      protocol.TypeRecordingStopped,
			SessionID: sessionID,
			SentAt:    time.Now().UTC().Format(time.RFC3339),
		},
	})

	return messages, nil
}

func (m *Manager) Heartbeat(sessionID string) (SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return SessionEvent{}, errors.New("session not found")
	}

	session.LastHeartbeat = time.Now()

	return SessionEvent{
		Type:      protocol.TypeHeartbeat,
		SessionID: sessionID,
		SentAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (m *Manager) HandleMixedAudio(packet protocol.MixedAudioPacket) ([]protocol.RoutedMessage, error) {
	session, err := m.sessionForIngest(packet.SessionID)
	if err != nil {
		return nil, err
	}

	if len(packet.Payload) == 0 {
		return nil, errors.New("audio payload is empty")
	}

	if packet.ClientID == "" {
		packet.ClientID = session.ClientID
	}

	transcriptResult, ok := m.sttService.Consume(packet)
	if !ok {
		return nil, nil
	}
	if err := m.persistTranscriptSnapshot(packet.SessionID, transcriptResult); err != nil {
		return nil, err
	}
	m.scheduleAnalysis(packet.ClientID, packet.SessionID, false)

	return []protocol.RoutedMessage{
		{
			Topic:   protocol.SttTopic(packet.ClientID, packet.SessionID),
			Type:    protocol.TypeSTTDelta,
			Payload: transcriptResult,
		},
	}, nil
}

func (m *Manager) transition(sessionID, status, eventType string) (SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return SessionEvent{}, errors.New("session not found")
	}

	session.Status = status

	return SessionEvent{
		Type:      eventType,
		SessionID: sessionID,
		SentAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (m *Manager) setStatus(sessionID, status string) (*SessionState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}

	session.Status = status
	return session, nil
}

func (m *Manager) sessionForIngest(sessionID string) (*SessionState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}

	if session.Status != "recording" {
		return nil, errors.New("session is not recording")
	}

	return session, nil
}

func (m *Manager) persistTranscriptSnapshot(sessionID string, transcript protocol.TranscriptPayload) error {
	if m.transcriptStore == nil {
		return nil
	}

	return m.transcriptStore.UpsertSnapshot(transcripts.Snapshot{
		MeetingID: sessionID,
		SegmentID: transcript.SegmentID,
		StartMS:   transcript.StartMS,
		EndMS:     transcript.EndMS,
		Text:      transcript.Text,
		IsFinal:   transcript.IsFinal,
		Revision:  transcript.Revision,
	})
}

func (m *Manager) scheduleAnalysis(clientID, sessionID string, isFinal bool) {
	if m.analysisScheduler == nil {
		return
	}

	m.analysisScheduler.Schedule(clientID, sessionID, isFinal)
}

func toSummaryPayload(result summary.Result) protocol.SummaryPayload {
	return protocol.SummaryPayload{
		Version:      result.Version,
		UpdatedAt:    result.UpdatedAt,
		AbstractText: result.AbstractText,
		KeyPoints:    result.KeyPoints,
		Decisions:    result.Decisions,
		Risks:        result.Risks,
		ActionItems:  result.ActionItems,
		IsFinal:      result.IsFinal,
	}
}

func toActionItemsPayload(result action_items.Result) protocol.ActionItemsPayload {
	return protocol.ActionItemsPayload{
		Version:   result.Version,
		UpdatedAt: result.UpdatedAt,
		Items:     result.Items,
		IsFinal:   result.IsFinal,
	}
}
