package summary

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	openaicompat "meeting-server/internal/model/openai_compatible"
)

type sessionState struct {
	version uint64
}

type Result struct {
	Version      uint64
	UpdatedAt    string
	AbstractText string
	KeyPoints    []string
	Decisions    []string
	Risks        []string
	ActionItems  []string
	IsFinal      bool
}

type Generator interface {
	Name() string
	Generate(ctx context.Context, sessionID string, transcriptText string, actionItems []string, isFinal bool) (Result, error)
}

type Service struct {
	mu        sync.Mutex
	sessions  map[string]*sessionState
	generator Generator
}

type Option func(*Service)

func WithGenerator(generator Generator) Option {
	return func(service *Service) {
		if generator != nil {
			service.generator = generator
		}
	}
}

func NewService(options ...Option) *Service {
	service := &Service{
		sessions:  make(map[string]*sessionState),
		generator: StubGenerator{},
	}

	for _, option := range options {
		option(service)
	}

	return service
}

func (s *Service) ProviderName() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.generator.Name()
}

func (s *Service) SetGenerator(generator Generator) {
	if generator == nil {
		generator = StubGenerator{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.generator = generator
}

func (s *Service) Generate(sessionID, transcriptText string, actionItems []string, isFinal bool) Result {
	s.mu.Lock()
	state := s.ensureState(sessionID)
	state.version++
	version := state.version
	if isFinal {
		delete(s.sessions, sessionID)
	}
	s.mu.Unlock()

	result, err := s.generator.Generate(context.Background(), sessionID, transcriptText, actionItems, isFinal)
	if err != nil {
		result = StubGenerator{}.mustGenerate(sessionID, transcriptText, actionItems, isFinal)
		result.Risks = append(result.Risks, fmt.Sprintf("模型调用失败，已回退到 stub：%v", err))
	}

	result.Version = version
	result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	result.IsFinal = isFinal
	return result
}

func (s *Service) Consume(sessionID, transcriptText string, actionItems []string) Result {
	return s.Generate(sessionID, transcriptText, actionItems, false)
}

func (s *Service) Flush(sessionID string, actionItems []string) (Result, bool) {
	s.mu.Lock()
	_, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return Result{}, false
	}

	return s.Generate(sessionID, "", actionItems, true), true
}

func (s *Service) ensureState(sessionID string) *sessionState {
	state, ok := s.sessions[sessionID]
	if !ok {
		state = &sessionState{}
		s.sessions[sessionID] = state
	}

	return state
}

type StubGenerator struct{}

func (StubGenerator) Name() string {
	return "stub"
}

func (g StubGenerator) Generate(_ context.Context, _ string, transcriptText string, actionItems []string, isFinal bool) (Result, error) {
	return g.mustGenerate("", transcriptText, actionItems, isFinal), nil
}

func (StubGenerator) mustGenerate(_ string, transcriptText string, actionItems []string, isFinal bool) Result {
	snippets := summarizedTranscript(transcriptText)
	abstract := fmt.Sprintf("已接收 %d 段实时转写占位结果，纪要会继续滚动刷新。", len(snippets))
	if isFinal {
		abstract = fmt.Sprintf("会议结束，累计生成 %d 段转写占位结果。", len(snippets))
	}

	return Result{
		AbstractText: abstract,
		KeyPoints:    lastN(snippets, map[bool]int{true: 5, false: 3}[isFinal]),
		Decisions:    []string{},
		Risks:        []string{"当前为 pipeline stub，待接入真实 STT/总结服务。"},
		ActionItems:  cloneStrings(actionItems),
		IsFinal:      isFinal,
	}
}

type OpenAICompatibleGenerator struct {
	providerName string
	client       *openaicompat.ChatClient
}

func NewOpenAICompatibleGenerator(client *openaicompat.ChatClient) *OpenAICompatibleGenerator {
	return &OpenAICompatibleGenerator{
		providerName: "openai_compatible",
		client:       client,
	}
}

func NewOpenAIGenerator(client *openaicompat.ChatClient) *OpenAICompatibleGenerator {
	return &OpenAICompatibleGenerator{
		providerName: "openai",
		client:       client,
	}
}

func NewDeepSeekGenerator(client *openaicompat.ChatClient) *OpenAICompatibleGenerator {
	return &OpenAICompatibleGenerator{
		providerName: "deepseek",
		client:       client,
	}
}

func NewKimiGenerator(client *openaicompat.ChatClient) *OpenAICompatibleGenerator {
	return &OpenAICompatibleGenerator{
		providerName: "kimi",
		client:       client,
	}
}

func (g *OpenAICompatibleGenerator) Name() string {
	if g.providerName == "" {
		return "openai_compatible"
	}

	return g.providerName
}

func (g *OpenAICompatibleGenerator) Generate(ctx context.Context, sessionID string, transcriptText string, actionItems []string, isFinal bool) (Result, error) {
	type response struct {
		AbstractText string   `json:"abstract"`
		KeyPoints    []string `json:"keyPoints"`
		Decisions    []string `json:"decisions"`
		Risks        []string `json:"risks"`
		ActionItems  []string `json:"actionItems"`
	}

	userPrompt := fmt.Sprintf(
		"session_id=%s\nis_final=%t\ntranscript_text=%s\naction_items=%v\n请输出会议纪要 JSON。",
		sessionID,
		isFinal,
		transcriptText,
		actionItems,
	)

	var decoded response
	if err := g.client.CompleteJSON(
		ctx,
		"你是会议纪要模型。请严格返回 JSON，包含 abstract、keyPoints、decisions、risks、actionItems 字段。",
		userPrompt,
		&decoded,
	); err != nil {
		return Result{}, err
	}

	return Result{
		AbstractText: decoded.AbstractText,
		KeyPoints:    cloneStrings(decoded.KeyPoints),
		Decisions:    cloneStrings(decoded.Decisions),
		Risks:        cloneStrings(decoded.Risks),
		ActionItems:  cloneStrings(decoded.ActionItems),
		IsFinal:      isFinal,
	}, nil
}

func cloneStrings(items []string) []string {
	return append([]string(nil), items...)
}

func summarizedTranscript(transcriptText string) []string {
	trimmed := strings.TrimSpace(transcriptText)
	if trimmed == "" {
		return []string{}
	}

	return []string{trimmed}
}

func lastN(items []string, limit int) []string {
	if len(items) <= limit {
		return cloneStrings(items)
	}

	return cloneStrings(items[len(items)-limit:])
}
