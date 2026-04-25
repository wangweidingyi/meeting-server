package llmproviders

import (
	"fmt"
	"strings"

	"meeting-server/internal/config"
	openaicompat "meeting-server/internal/model/openai_compatible"
)

type Definition struct {
	Name           string
	DefaultBaseURL string
}

var definitions = map[string]Definition{
	"openai": {
		Name:           "openai",
		DefaultBaseURL: "https://api.openai.com/v1",
	},
	"deepseek": {
		Name:           "deepseek",
		DefaultBaseURL: "https://api.deepseek.com/v1",
	},
	"kimi": {
		Name:           "kimi",
		DefaultBaseURL: "https://api.moonshot.cn/v1",
	},
}

func CanonicalProviderName(provider string) string {
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return "openai"
	default:
		return strings.TrimSpace(provider)
	}
}

func RuntimeProviderName(provider string) string {
	trimmed := strings.TrimSpace(provider)
	if trimmed == "openai_compatible" {
		return "openai_compatible"
	}

	return CanonicalProviderName(trimmed)
}

func SupportsProvider(provider string) bool {
	trimmed := strings.TrimSpace(provider)
	if trimmed == "" || trimmed == "stub" {
		return true
	}

	_, ok := definitions[CanonicalProviderName(trimmed)]
	return ok
}

func DefaultBaseURL(provider string) string {
	definition, ok := definitions[CanonicalProviderName(provider)]
	if !ok {
		return ""
	}

	return definition.DefaultBaseURL
}

func Validate(fieldPath string, cfg config.ModelProviderConfig) error {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		return fmt.Errorf("%s.provider is required", fieldPath)
	}
	if provider == "stub" {
		return nil
	}
	if !SupportsProvider(provider) {
		return fmt.Errorf("%s.provider %q is not supported", fieldPath, provider)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("%s.baseUrl is required for %s", fieldPath, provider)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("%s.apiKey is required for %s", fieldPath, provider)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("%s.model is required for %s", fieldPath, provider)
	}

	return nil
}

func NewChatClient(cfg config.ModelProviderConfig) (string, *openaicompat.ChatClient, bool) {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" || provider == "stub" || !SupportsProvider(provider) {
		return "", nil, false
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}

	return RuntimeProviderName(provider), &openaicompat.ChatClient{
		BaseURL: baseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	}, true
}
