package admin

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"meeting-server/internal/config"
	openaicompat "meeting-server/internal/model/openai_compatible"
)

type RuntimeTester struct{}

func NewRuntimeTester() *RuntimeTester {
	return &RuntimeTester{}
}

func (t *RuntimeTester) Test(ctx context.Context, ai config.AIConfig) (TestSettingsResult, error) {
	return TestSettingsResult{
		STT: testSTTProvider(ctx, ai.STT),
		LLM: testLLMProvider(ctx, ai.LLM),
		TTS: testTTSProvider(ctx, ai.TTS),
	}, nil
}

func testSTTProvider(ctx context.Context, provider config.ModelProviderConfig) ProviderTestResult {
	startedAt := time.Now()
	result := ProviderTestResult{Provider: provider.Provider}

	switch provider.Provider {
	case "stub":
		result.OK = true
		result.Message = "stub provider ready"
	case "openai_compatible":
		client := &openaicompat.TranscriptionClient{
			BaseURL: provider.BaseURL,
			APIKey:  provider.APIKey,
			Model:   provider.Model,
		}
		_, err := client.Recognize(ctx, silentWave())
		if err != nil {
			result.Message = err.Error()
			break
		}
		result.OK = true
		result.Message = "transcription ok"
	default:
		result.Message = fmt.Sprintf("unsupported stt provider %q", provider.Provider)
	}

	result.LatencyMS = time.Since(startedAt).Milliseconds()
	return result
}

func testLLMProvider(ctx context.Context, provider config.ModelProviderConfig) ProviderTestResult {
	startedAt := time.Now()
	result := ProviderTestResult{Provider: provider.Provider}

	switch provider.Provider {
	case "stub":
		result.OK = true
		result.Message = "stub provider ready"
	case "openai_compatible":
		client := &openaicompat.ChatClient{
			BaseURL: provider.BaseURL,
			APIKey:  provider.APIKey,
			Model:   provider.Model,
		}
		var response struct {
			OK bool `json:"ok"`
		}
		err := client.CompleteJSON(
			ctx,
			"你是配置测试助手。请严格输出 JSON。",
			"请返回 {\"ok\": true}",
			&response,
		)
		if err != nil {
			result.Message = err.Error()
			break
		}
		if !response.OK {
			result.Message = "llm test response missing ok=true"
			break
		}
		result.OK = true
		result.Message = "chat completion ok"
	default:
		result.Message = fmt.Sprintf("unsupported llm provider %q", provider.Provider)
	}

	result.LatencyMS = time.Since(startedAt).Milliseconds()
	return result
}

func testTTSProvider(ctx context.Context, provider config.SpeechProviderConfig) ProviderTestResult {
	startedAt := time.Now()
	result := ProviderTestResult{Provider: provider.Provider}

	switch provider.Provider {
	case "stub":
		result.OK = true
		result.Message = "stub provider ready"
	case "openai_compatible":
		client := &openaicompat.SpeechClient{
			BaseURL: provider.BaseURL,
			APIKey:  provider.APIKey,
			Model:   provider.Model,
			Voice:   provider.Voice,
		}
		audio, err := client.Synthesize(ctx, "配置测试")
		if err != nil {
			result.Message = err.Error()
			break
		}
		if len(audio) == 0 {
			result.Message = "tts response is empty"
			break
		}
		result.OK = true
		result.Message = "speech synthesis ok"
	default:
		result.Message = fmt.Sprintf("unsupported tts provider %q", provider.Provider)
	}

	result.LatencyMS = time.Since(startedAt).Milliseconds()
	return result
}

func silentWave() []byte {
	// 200ms of PCM16 mono silence at 16kHz.
	pcm := make([]byte, 16_000/5*2)
	return encodeWave(pcm, 16_000, 1)
}

func encodeWave(pcm []byte, sampleRateHz uint32, channels uint16) []byte {
	dataLength := uint32(len(pcm))
	blockAlign := channels * 2
	byteRate := sampleRateHz * uint32(blockAlign)
	riffSize := uint32(36) + dataLength

	out := make([]byte, 44+len(pcm))
	copy(out[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(out[4:8], riffSize)
	copy(out[8:12], []byte("WAVE"))
	copy(out[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], channels)
	binary.LittleEndian.PutUint32(out[24:28], sampleRateHz)
	binary.LittleEndian.PutUint32(out[28:32], byteRate)
	binary.LittleEndian.PutUint16(out[32:34], blockAlign)
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(out[40:44], dataLength)
	copy(out[44:], pcm)
	return out
}

type fakeTester struct {
	result TestSettingsResult
	err    error
}

func (t *fakeTester) Test(_ context.Context, _ config.AIConfig) (TestSettingsResult, error) {
	if t.err != nil {
		return TestSettingsResult{}, t.err
	}

	return t.result, nil
}

func cleanProviderError(message string) string {
	return strings.TrimSpace(message)
}
