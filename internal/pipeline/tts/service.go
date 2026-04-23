package tts

import (
	"context"
	"sync"

	openaicompat "meeting-server/internal/model/openai_compatible"
)

type Synthesizer interface {
	Name() string
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

type Service struct {
	mu          sync.RWMutex
	synthesizer Synthesizer
}

type Option func(*Service)

func WithSynthesizer(synthesizer Synthesizer) Option {
	return func(service *Service) {
		if synthesizer != nil {
			service.synthesizer = synthesizer
		}
	}
}

func NewService(options ...Option) *Service {
	service := &Service{
		synthesizer: StubSynthesizer{},
	}

	for _, option := range options {
		option(service)
	}

	return service
}

func (s *Service) ProviderName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.synthesizer.Name()
}

func (s *Service) Synthesize(ctx context.Context, text string) ([]byte, error) {
	s.mu.RLock()
	synthesizer := s.synthesizer
	s.mu.RUnlock()

	return synthesizer.Synthesize(ctx, text)
}

func (s *Service) SetSynthesizer(synthesizer Synthesizer) {
	if synthesizer == nil {
		synthesizer = StubSynthesizer{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.synthesizer = synthesizer
}

type StubSynthesizer struct{}

func (StubSynthesizer) Name() string {
	return "stub"
}

func (StubSynthesizer) Synthesize(_ context.Context, _ string) ([]byte, error) {
	return []byte("stub-tts-audio"), nil
}

type OpenAICompatibleSynthesizer struct {
	client *openaicompat.SpeechClient
}

func NewOpenAICompatibleSynthesizer(client *openaicompat.SpeechClient) *OpenAICompatibleSynthesizer {
	return &OpenAICompatibleSynthesizer{client: client}
}

func (s *OpenAICompatibleSynthesizer) Name() string {
	return "openai_compatible"
}

func (s *OpenAICompatibleSynthesizer) Synthesize(ctx context.Context, text string) ([]byte, error) {
	return s.client.Synthesize(ctx, text)
}
