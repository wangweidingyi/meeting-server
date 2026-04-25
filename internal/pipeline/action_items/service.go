package action_items

import (
	"context"
	"fmt"
	"sync"
	"time"

	openaicompat "meeting-server/internal/model/openai_compatible"
)

type sessionState struct {
	version uint64
	items   []string
}

type Result struct {
	Version   uint64
	UpdatedAt string
	Items     []string
	IsFinal   bool
}

type Extractor interface {
	Name() string
	Extract(ctx context.Context, sessionID string, transcriptText string, existingItems []string, isFinal bool) ([]string, error)
}

type Service struct {
	mu        sync.Mutex
	sessions  map[string]*sessionState
	extractor Extractor
}

type Option func(*Service)

func WithExtractor(extractor Extractor) Option {
	return func(service *Service) {
		if extractor != nil {
			service.extractor = extractor
		}
	}
}

func NewService(options ...Option) *Service {
	service := &Service{
		sessions:  make(map[string]*sessionState),
		extractor: StubExtractor{},
	}

	for _, option := range options {
		option(service)
	}

	return service
}

func (s *Service) ProviderName() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.extractor.Name()
}

func (s *Service) SetExtractor(extractor Extractor) {
	if extractor == nil {
		extractor = StubExtractor{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.extractor = extractor
}

func (s *Service) Consume(sessionID, transcriptText string) Result {
	s.mu.Lock()
	state := s.ensureState(sessionID)
	state.version++
	version := state.version
	existingItems := cloneStrings(state.items)
	s.mu.Unlock()

	items, err := s.extractor.Extract(context.Background(), sessionID, transcriptText, existingItems, false)
	if err != nil {
		items = StubExtractor{}.mustExtract(transcriptText, existingItems)
	}

	s.mu.Lock()
	state = s.ensureState(sessionID)
	state.items = cloneStrings(items)
	s.mu.Unlock()

	return Result{
		Version:   version,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Items:     cloneStrings(items),
		IsFinal:   false,
	}
}

func (s *Service) Flush(sessionID string) (Result, bool) {
	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok || len(state.items) == 0 {
		s.mu.Unlock()
		return Result{}, false
	}

	state.version++
	version := state.version
	items := cloneStrings(state.items)
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	return Result{
		Version:   version,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Items:     items,
		IsFinal:   true,
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

type StubExtractor struct{}

func (StubExtractor) Name() string {
	return "stub"
}

func (s StubExtractor) Extract(_ context.Context, _ string, transcriptText string, existingItems []string, _ bool) ([]string, error) {
	return s.mustExtract(transcriptText, existingItems), nil
}

func (StubExtractor) mustExtract(transcriptText string, existingItems []string) []string {
	items := cloneStrings(existingItems)
	items = append(items, fmt.Sprintf("跟进占位片段 %d：%s", len(items)+1, truncate(transcriptText, 48)))
	return items
}

type OpenAICompatibleExtractor struct {
	providerName string
	client *openaicompat.ChatClient
}

func NewOpenAICompatibleExtractor(client *openaicompat.ChatClient) *OpenAICompatibleExtractor {
	return &OpenAICompatibleExtractor{
		providerName: "openai_compatible",
		client:       client,
	}
}

func NewOpenAIExtractor(client *openaicompat.ChatClient) *OpenAICompatibleExtractor {
	return &OpenAICompatibleExtractor{
		providerName: "openai",
		client:       client,
	}
}

func NewDeepSeekExtractor(client *openaicompat.ChatClient) *OpenAICompatibleExtractor {
	return &OpenAICompatibleExtractor{
		providerName: "deepseek",
		client:       client,
	}
}

func NewKimiExtractor(client *openaicompat.ChatClient) *OpenAICompatibleExtractor {
	return &OpenAICompatibleExtractor{
		providerName: "kimi",
		client:       client,
	}
}

func (e *OpenAICompatibleExtractor) Name() string {
	if e.providerName == "" {
		return "openai_compatible"
	}

	return e.providerName
}

func (e *OpenAICompatibleExtractor) Extract(ctx context.Context, sessionID string, transcriptText string, existingItems []string, isFinal bool) ([]string, error) {
	type response struct {
		Items []string `json:"items"`
	}

	userPrompt := fmt.Sprintf(
		"session_id=%s\nis_final=%t\nexisting_items=%v\ntranscript_text=%s\n请抽取行动项并输出 JSON。",
		sessionID,
		isFinal,
		existingItems,
		transcriptText,
	)

	var decoded response
	if err := e.client.CompleteJSON(
		ctx,
		"你是会议行动项提取模型。请严格返回 JSON，包含 items 字段，内容是行动项字符串数组。",
		userPrompt,
		&decoded,
	); err != nil {
		return nil, err
	}

	return cloneStrings(decoded.Items), nil
}

func cloneStrings(items []string) []string {
	return append([]string(nil), items...)
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}

	return text[:limit]
}
