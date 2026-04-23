package config

import "testing"

func TestDefaultConfigUsesLocalUDPAndDisablesMQTT(t *testing.T) {
	cfg := Default()

	if cfg.UDP.Host != "127.0.0.1" {
		t.Fatalf("unexpected udp host %s", cfg.UDP.Host)
	}
	if cfg.UDP.Port != 6000 {
		t.Fatalf("unexpected udp port %d", cfg.UDP.Port)
	}
	if cfg.MQTT.Enabled {
		t.Fatal("expected mqtt to be disabled by default")
	}
	if cfg.MQTT.Embedded {
		t.Fatal("expected embedded mqtt broker to be disabled by default")
	}
	if cfg.MQTT.ListenHost != "127.0.0.1" {
		t.Fatalf("unexpected mqtt listen host %s", cfg.MQTT.ListenHost)
	}
	if cfg.MQTT.ListenPort != 1883 {
		t.Fatalf("unexpected mqtt listen port %d", cfg.MQTT.ListenPort)
	}
	if cfg.HTTP.Host != "127.0.0.1" {
		t.Fatalf("unexpected http host %s", cfg.HTTP.Host)
	}
	if cfg.HTTP.Port != 8090 {
		t.Fatalf("unexpected http port %d", cfg.HTTP.Port)
	}
	if cfg.Database.URL != "" {
		t.Fatalf("expected database url to be empty by default, got %q", cfg.Database.URL)
	}
}

func TestLoadFromEnvOverridesUDPAndMQTTConfig(t *testing.T) {
	t.Setenv("MEETING_UDP_HOST", "0.0.0.0")
	t.Setenv("MEETING_UDP_PORT", "7001")
	t.Setenv("MEETING_HTTP_HOST", "0.0.0.0")
	t.Setenv("MEETING_HTTP_PORT", "9090")
	t.Setenv("MEETING_DATABASE_URL", "postgres://meeting:secret@127.0.0.1:5432/meeting")
	t.Setenv("MEETING_MQTT_EMBEDDED", "true")
	t.Setenv("MEETING_MQTT_LISTEN_HOST", "0.0.0.0")
	t.Setenv("MEETING_MQTT_LISTEN_PORT", "2883")
	t.Setenv("MEETING_MQTT_BROKER", "tcp://127.0.0.1:1883")
	t.Setenv("MEETING_MQTT_CLIENT_ID", "meeting-server-test")
	t.Setenv("MEETING_MQTT_USERNAME", "user-a")
	t.Setenv("MEETING_MQTT_PASSWORD", "pass-a")
	t.Setenv("MEETING_STT_PROVIDER", "openai_compatible")
	t.Setenv("MEETING_STT_BASE_URL", "https://example.com/v1/audio/transcriptions")
	t.Setenv("MEETING_STT_API_KEY", "stt-key")
	t.Setenv("MEETING_STT_MODEL", "sensevoice-meeting")
	t.Setenv("MEETING_LLM_PROVIDER", "openai_compatible")
	t.Setenv("MEETING_LLM_BASE_URL", "https://example.com/v1")
	t.Setenv("MEETING_LLM_API_KEY", "llm-key")
	t.Setenv("MEETING_LLM_MODEL", "qwen-meeting")
	t.Setenv("MEETING_TTS_PROVIDER", "openai_compatible")
	t.Setenv("MEETING_TTS_BASE_URL", "https://example.com/v1/audio/speech")
	t.Setenv("MEETING_TTS_API_KEY", "tts-key")
	t.Setenv("MEETING_TTS_MODEL", "cosyvoice-meeting")
	t.Setenv("MEETING_TTS_VOICE", "alex")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load from env: %v", err)
	}

	if cfg.UDP.Host != "0.0.0.0" {
		t.Fatalf("unexpected udp host %s", cfg.UDP.Host)
	}
	if cfg.UDP.Port != 7001 {
		t.Fatalf("unexpected udp port %d", cfg.UDP.Port)
	}
	if cfg.HTTP.Host != "0.0.0.0" {
		t.Fatalf("unexpected http host %s", cfg.HTTP.Host)
	}
	if cfg.HTTP.Port != 9090 {
		t.Fatalf("unexpected http port %d", cfg.HTTP.Port)
	}
	if cfg.Database.URL != "postgres://meeting:secret@127.0.0.1:5432/meeting" {
		t.Fatalf("unexpected database url %s", cfg.Database.URL)
	}
	if !cfg.MQTT.Enabled {
		t.Fatal("expected mqtt to be enabled when broker env is present")
	}
	if !cfg.MQTT.Embedded {
		t.Fatal("expected embedded mqtt broker to be enabled from env")
	}
	if cfg.MQTT.BrokerURL != "tcp://127.0.0.1:1883" {
		t.Fatalf("unexpected broker url %s", cfg.MQTT.BrokerURL)
	}
	if cfg.MQTT.ListenHost != "0.0.0.0" {
		t.Fatalf("unexpected mqtt listen host %s", cfg.MQTT.ListenHost)
	}
	if cfg.MQTT.ListenPort != 2883 {
		t.Fatalf("unexpected mqtt listen port %d", cfg.MQTT.ListenPort)
	}
	if cfg.MQTT.ClientID != "meeting-server-test" {
		t.Fatalf("unexpected client id %s", cfg.MQTT.ClientID)
	}
	if cfg.AI.STT.Provider != "openai_compatible" {
		t.Fatalf("unexpected stt provider %s", cfg.AI.STT.Provider)
	}
	if cfg.AI.STT.BaseURL != "https://example.com/v1/audio/transcriptions" {
		t.Fatalf("unexpected stt base url %s", cfg.AI.STT.BaseURL)
	}
	if cfg.AI.STT.APIKey != "stt-key" {
		t.Fatalf("unexpected stt api key %s", cfg.AI.STT.APIKey)
	}
	if cfg.AI.STT.Model != "sensevoice-meeting" {
		t.Fatalf("unexpected stt model %s", cfg.AI.STT.Model)
	}
	if cfg.AI.LLM.Provider != "openai_compatible" {
		t.Fatalf("unexpected llm provider %s", cfg.AI.LLM.Provider)
	}
	if cfg.AI.LLM.BaseURL != "https://example.com/v1" {
		t.Fatalf("unexpected llm base url %s", cfg.AI.LLM.BaseURL)
	}
	if cfg.AI.LLM.APIKey != "llm-key" {
		t.Fatalf("unexpected llm api key %s", cfg.AI.LLM.APIKey)
	}
	if cfg.AI.LLM.Model != "qwen-meeting" {
		t.Fatalf("unexpected llm model %s", cfg.AI.LLM.Model)
	}
	if cfg.AI.TTS.Provider != "openai_compatible" {
		t.Fatalf("unexpected tts provider %s", cfg.AI.TTS.Provider)
	}
	if cfg.AI.TTS.BaseURL != "https://example.com/v1/audio/speech" {
		t.Fatalf("unexpected tts base url %s", cfg.AI.TTS.BaseURL)
	}
	if cfg.AI.TTS.APIKey != "tts-key" {
		t.Fatalf("unexpected tts api key %s", cfg.AI.TTS.APIKey)
	}
	if cfg.AI.TTS.Model != "cosyvoice-meeting" {
		t.Fatalf("unexpected tts model %s", cfg.AI.TTS.Model)
	}
	if cfg.AI.TTS.Voice != "alex" {
		t.Fatalf("unexpected tts voice %s", cfg.AI.TTS.Voice)
	}
}

func TestSummaryIncludesTransportState(t *testing.T) {
	cfg := Config{
		UDP: UDPConfig{
			Host: "127.0.0.1",
			Port: 6000,
		},
		MQTT: MQTTConfig{
			Enabled:   true,
			BrokerURL: "tcp://127.0.0.1:1883",
			ClientID:  "meeting-server",
		},
		HTTP: HTTPConfig{
			Host: "127.0.0.1",
			Port: 8090,
		},
		Database: DatabaseConfig{
			URL: "postgres://meeting:secret@127.0.0.1:5432/meeting",
		},
		AI: AIConfig{
			STT: ModelProviderConfig{Provider: "stub"},
			LLM: ModelProviderConfig{Provider: "stub"},
			TTS: SpeechProviderConfig{Provider: "stub"},
		},
	}

	got := cfg.Summary()
	want := "udp=127.0.0.1:6000 http=127.0.0.1:8090 db=configured mqtt=enabled broker=tcp://127.0.0.1:1883 client_id=meeting-server"

	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
