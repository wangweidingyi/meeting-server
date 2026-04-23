package admin

import (
	"context"
	"errors"
	"sync"
	"time"

	"meeting-server/internal/config"
)

type Settings struct {
	Version   int64           `json:"version"`
	UpdatedAt string          `json:"updatedAt"`
	AI        config.AIConfig `json:"ai"`
}

type UpdateSettingsRequest struct {
	AI config.AIConfig `json:"ai"`
}

type ProviderTestResult struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latencyMs"`
}

type TestSettingsResult struct {
	STT ProviderTestResult `json:"stt"`
	LLM ProviderTestResult `json:"llm"`
	TTS ProviderTestResult `json:"tts"`
}

type Store interface {
	Load(ctx context.Context) (Settings, bool, error)
	Save(ctx context.Context, settings Settings) (Settings, error)
}

type ApplyFunc func(config.AIConfig)

type Tester interface {
	Test(ctx context.Context, ai config.AIConfig) (TestSettingsResult, error)
}

type Service struct {
	mu        sync.RWMutex
	store     Store
	apply     ApplyFunc
	tester    Tester
	current   Settings
	booted    bool
	initialAI config.AIConfig
}

func NewService(store Store, initialAI config.AIConfig, apply ApplyFunc) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	if apply == nil {
		apply = func(config.AIConfig) {}
	}

	return &Service{
		store:     store,
		apply:     apply,
		tester:    NewRuntimeTester(),
		initialAI: initialAI,
		current: Settings{
			AI: initialAI,
		},
	}
}

func (s *Service) Bootstrap(ctx context.Context) error {
	stored, ok, err := s.store.Load(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if ok {
		s.current = stored
	} else {
		s.current = Settings{
			AI: s.initialAI,
		}
	}

	s.booted = true
	s.apply(s.current.AI)
	return nil
}

func (s *Service) Current() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneSettings(s.current)
}

func (s *Service) Update(ctx context.Context, request UpdateSettingsRequest) (Settings, error) {
	if err := validateAIConfig(request.AI); err != nil {
		return Settings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.booted {
		return Settings{}, errors.New("admin settings service is not bootstrapped")
	}

	next := Settings{
		Version:   s.current.Version,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		AI:        request.AI,
	}

	saved, err := s.store.Save(ctx, next)
	if err != nil {
		return Settings{}, err
	}

	s.current = saved
	s.apply(saved.AI)
	return cloneSettings(saved), nil
}

func (s *Service) SetTester(tester Tester) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tester == nil {
		s.tester = NewRuntimeTester()
		return
	}

	s.tester = tester
}

func (s *Service) Test(ctx context.Context, request UpdateSettingsRequest) (TestSettingsResult, error) {
	s.mu.RLock()
	tester := s.tester
	current := cloneSettings(s.current)
	s.mu.RUnlock()

	ai := request.AI
	if ai == (config.AIConfig{}) {
		ai = current.AI
	}

	if err := validateAIConfig(ai); err != nil {
		return TestSettingsResult{}, err
	}

	return tester.Test(ctx, ai)
}

func validateAIConfig(ai config.AIConfig) error {
	if ai.STT.Provider == "" {
		return errors.New("ai.stt.provider is required")
	}
	if ai.LLM.Provider == "" {
		return errors.New("ai.llm.provider is required")
	}
	if ai.TTS.Provider == "" {
		return errors.New("ai.tts.provider is required")
	}

	return nil
}

func cloneSettings(settings Settings) Settings {
	return Settings{
		Version:   settings.Version,
		UpdatedAt: settings.UpdatedAt,
		AI: config.AIConfig{
			STT: settings.AI.STT,
			LLM: settings.AI.LLM,
			TTS: settings.AI.TTS,
		},
	}
}
