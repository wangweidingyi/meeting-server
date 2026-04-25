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

func TestLoadFromEnvRequiresDatabaseURL(t *testing.T) {
	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected load from env to fail when MEETING_DATABASE_URL is missing")
	}
	if err.Error() != "MEETING_DATABASE_URL is required" {
		t.Fatalf("unexpected error %q", err)
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
	t.Setenv("MEETING_STT_APP_KEY", "stt-app-key")
	t.Setenv("MEETING_STT_RESOURCE_ID", "volc.seedasr.sauc.duration")
	t.Setenv("MEETING_STT_LANGUAGE", "zh-CN")
	t.Setenv("MEETING_STT_AUDIO_FORMAT", "pcm")
	t.Setenv("MEETING_STT_AUDIO_CODEC", "raw")
	t.Setenv("MEETING_STT_SAMPLE_RATE", "16000")
	t.Setenv("MEETING_STT_BITS", "16")
	t.Setenv("MEETING_STT_CHANNELS", "1")
	t.Setenv("MEETING_STT_ENABLE_ITN", "true")
	t.Setenv("MEETING_STT_ENABLE_PUNC", "true")
	t.Setenv("MEETING_STT_ENABLE_NONSTREAM", "false")
	t.Setenv("MEETING_STT_SHOW_UTTERANCES", "true")
	t.Setenv("MEETING_STT_RESULT_TYPE", "full")
	t.Setenv("MEETING_STT_END_WINDOW_SIZE", "800")
	t.Setenv("MEETING_LLM_PROVIDER", "openai_compatible")
	t.Setenv("MEETING_LLM_BASE_URL", "https://example.com/v1")
	t.Setenv("MEETING_LLM_API_KEY", "llm-key")
	t.Setenv("MEETING_LLM_MODEL", "qwen-meeting")
	t.Setenv("MEETING_TTS_PROVIDER", "openai_compatible")
	t.Setenv("MEETING_TTS_BASE_URL", "https://example.com/v1/audio/speech")
	t.Setenv("MEETING_TTS_API_KEY", "tts-key")
	t.Setenv("MEETING_TTS_MODEL", "cosyvoice-meeting")
	t.Setenv("MEETING_TTS_VOICE", "alex")
	t.Setenv("MEETING_BOOTSTRAP_ADMIN_USERNAME", "root-admin")
	t.Setenv("MEETING_BOOTSTRAP_ADMIN_PASSWORD", "RootAdmin1234")
	t.Setenv("MEETING_BOOTSTRAP_ADMIN_DISPLAY_NAME", "超级管理员")

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
	if cfg.AI.STT.Options["appKey"] != "stt-app-key" {
		t.Fatalf("unexpected stt app key %s", cfg.AI.STT.Options["appKey"])
	}
	if cfg.AI.STT.Options["resourceId"] != "volc.seedasr.sauc.duration" {
		t.Fatalf("unexpected stt resource id %s", cfg.AI.STT.Options["resourceId"])
	}
	if cfg.AI.STT.Options["language"] != "zh-CN" {
		t.Fatalf("unexpected stt language %s", cfg.AI.STT.Options["language"])
	}
	if cfg.AI.STT.Options["audioFormat"] != "pcm" {
		t.Fatalf("unexpected stt audio format %s", cfg.AI.STT.Options["audioFormat"])
	}
	if cfg.AI.STT.Options["audioCodec"] != "raw" {
		t.Fatalf("unexpected stt audio codec %s", cfg.AI.STT.Options["audioCodec"])
	}
	if cfg.AI.STT.Options["sampleRate"] != "16000" {
		t.Fatalf("unexpected stt sample rate %s", cfg.AI.STT.Options["sampleRate"])
	}
	if cfg.AI.STT.Options["bits"] != "16" {
		t.Fatalf("unexpected stt bits %s", cfg.AI.STT.Options["bits"])
	}
	if cfg.AI.STT.Options["channels"] != "1" {
		t.Fatalf("unexpected stt channels %s", cfg.AI.STT.Options["channels"])
	}
	if cfg.AI.STT.Options["enableItn"] != "true" {
		t.Fatalf("unexpected stt enableItn %s", cfg.AI.STT.Options["enableItn"])
	}
	if cfg.AI.STT.Options["enablePunc"] != "true" {
		t.Fatalf("unexpected stt enablePunc %s", cfg.AI.STT.Options["enablePunc"])
	}
	if cfg.AI.STT.Options["enableNonstream"] != "false" {
		t.Fatalf("unexpected stt enableNonstream %s", cfg.AI.STT.Options["enableNonstream"])
	}
	if cfg.AI.STT.Options["showUtterances"] != "true" {
		t.Fatalf("unexpected stt showUtterances %s", cfg.AI.STT.Options["showUtterances"])
	}
	if cfg.AI.STT.Options["resultType"] != "full" {
		t.Fatalf("unexpected stt result type %s", cfg.AI.STT.Options["resultType"])
	}
	if cfg.AI.STT.Options["endWindowSize"] != "800" {
		t.Fatalf("unexpected stt endWindowSize %s", cfg.AI.STT.Options["endWindowSize"])
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
	if cfg.BootstrapAdmin.Username != "root-admin" {
		t.Fatalf("unexpected bootstrap admin username %s", cfg.BootstrapAdmin.Username)
	}
	if cfg.BootstrapAdmin.Password != "RootAdmin1234" {
		t.Fatalf("unexpected bootstrap admin password %s", cfg.BootstrapAdmin.Password)
	}
	if cfg.BootstrapAdmin.DisplayName != "超级管理员" {
		t.Fatalf("unexpected bootstrap admin display name %s", cfg.BootstrapAdmin.DisplayName)
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
			STT: STTProviderConfig{Provider: "stub"},
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
