package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"meeting-server/internal/admin"
	"meeting-server/internal/config"
	"meeting-server/internal/protocol"
	mqtttransport "meeting-server/internal/transport/mqtt"
)

type recordingPublisher struct {
	ch chan []protocol.RoutedMessage
}

func (p *recordingPublisher) Publish(messages []protocol.RoutedMessage) {
	p.ch <- messages
}

func TestRunStartsUDPServerAndPublishesRealtimeMessages(t *testing.T) {
	publisher := &recordingPublisher{
		ch: make(chan []protocol.RoutedMessage, 1),
	}

	application := NewWithOptions(Options{
		UDPHost:   "127.0.0.1",
		UDPPort:   0,
		Publisher: publisher,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- application.Run(ctx)
	}()

	address := application.UDPServer.WaitUntilListening(time.Second)
	if address == "" {
		t.Fatal("udp server did not start listening")
	}

	if _, err := application.MQTTServer.HandleControlMessage(mqtttransport.ControlMessage{
		Type:      protocol.TypeSessionHello,
		ClientID:  "client-a",
		SessionID: "session-1",
		Title:     "App Run Test",
	}); err != nil {
		t.Fatalf("hello failed: %v", err)
	}

	if _, err := application.MQTTServer.HandleControlMessage(mqtttransport.ControlMessage{
		Type:      protocol.TypeRecordingStart,
		ClientID:  "client-a",
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	connection, err := net.Dial("udp", address)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer connection.Close()

	packet, err := protocol.UDPAudioPacket{
		Version:     1,
		SourceType:  protocol.AudioSourceMixed,
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}.Encode()
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	if _, err := connection.Write(packet); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	select {
	case messages := <-publisher.ch:
		assertRealtimeMessages(t, messages)
	case <-time.After(time.Second):
		t.Fatal("expected app runtime to publish realtime messages")
	}

	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("app run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app run did not stop after cancellation")
	}
}

func TestNewFromConfigLeavesMQTTRuntimeDisabledWithoutBroker(t *testing.T) {
	application := NewFromConfig(config.Config{
		UDP: config.UDPConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		MQTT: config.MQTTConfig{},
		Database: config.DatabaseConfig{
			URL: "postgres://meeting:secret@127.0.0.1:5432/meeting",
		},
	})

	if application.MQTTRuntime != nil {
		t.Fatal("expected mqtt runtime to stay disabled when broker config is absent")
	}
}

func TestNewFromConfigBuildsMQTTRuntimeWhenEmbeddedBrokerIsEnabled(t *testing.T) {
	application := NewFromConfig(config.Config{
		UDP: config.UDPConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		MQTT: config.MQTTConfig{
			Enabled:    true,
			Embedded:   true,
			ListenHost: "127.0.0.1",
			ListenPort: 1883,
			ClientID:   "meeting-server",
		},
		Database: config.DatabaseConfig{
			URL: "postgres://meeting:secret@127.0.0.1:5432/meeting",
		},
	})

	if application.MQTTRuntime == nil {
		t.Fatal("expected mqtt runtime to be available when embedded broker is enabled")
	}
	if application.MQTTBroker == nil {
		t.Fatal("expected embedded mqtt broker to be configured")
	}
}

func TestNewFromConfigBuildsModelBackedMeetingPipelinesWhenConfigured(t *testing.T) {
	application := NewFromConfig(config.Config{
		UDP: config.UDPConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		MQTT: config.MQTTConfig{},
		Database: config.DatabaseConfig{
			URL: "postgres://meeting:secret@127.0.0.1:5432/meeting",
		},
		AI: config.AIConfig{
			STT: config.STTProviderConfig{
				Provider: "volcengine_streaming",
				BaseURL:  "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
				APIKey:   "stt-access-key",
				Model:    "bigmodel",
				Options: map[string]string{
					"appKey":     "stt-app-key",
					"resourceId": "volc.seedasr.sauc.duration",
				},
			},
			LLM: config.ModelProviderConfig{
				Provider: "deepseek",
				BaseURL:  "https://api.deepseek.com/v1",
				APIKey:   "deepseek-key",
				Model:    "deepseek-chat",
			},
			TTS: config.SpeechProviderConfig{
				Provider: "openai_compatible",
				BaseURL:  "https://example.com/v1/audio/speech",
				APIKey:   "tts-key",
				Model:    "cosyvoice-meeting",
				Voice:    "alex",
			},
		},
	})

	if got := application.SummaryService.ProviderName(); got != "deepseek" {
		t.Fatalf("unexpected summary provider %s", got)
	}
	if got := application.STTService.ProviderName(); got != "volcengine_streaming" {
		t.Fatalf("unexpected stt provider %s", got)
	}
	if got := application.ActionItems.ProviderName(); got != "deepseek" {
		t.Fatalf("unexpected action-items provider %s", got)
	}
	if got := application.TTSService.ProviderName(); got != "openai_compatible" {
		t.Fatalf("unexpected tts provider %s", got)
	}
}

func TestNewFromConfigPanicsWithoutDatabaseURL(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic when database url is missing")
		}
	}()

	NewFromConfig(config.Config{
		UDP: config.UDPConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
	})
}

func TestApplyAIConfigUpdatesRunningPipelines(t *testing.T) {
	application := NewWithOptions(Options{
		UDPHost: "127.0.0.1",
		UDPPort: 0,
	})

	application.ApplyAIConfig(config.AIConfig{
		STT: config.STTProviderConfig{
			Provider: "volcengine_streaming",
			BaseURL:  "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
			APIKey:   "stt-access-key",
			Model:    "bigmodel",
			Options: map[string]string{
				"appKey":     "stt-app-key",
				"resourceId": "volc.seedasr.sauc.duration",
			},
		},
		LLM: config.ModelProviderConfig{
			Provider: "kimi",
			BaseURL:  "https://api.moonshot.cn/v1",
			APIKey:   "kimi-key",
			Model:    "moonshot-v1-8k",
		},
		TTS: config.SpeechProviderConfig{
			Provider: "openai_compatible",
			BaseURL:  "https://example.com/v1/audio/speech",
			APIKey:   "tts-key",
			Model:    "cosyvoice-v2",
			Voice:    "alloy",
		},
	})

	if got := application.STTService.ProviderName(); got != "volcengine_streaming" {
		t.Fatalf("unexpected stt provider %s", got)
	}
	if got := application.SummaryService.ProviderName(); got != "kimi" {
		t.Fatalf("unexpected summary provider %s", got)
	}
	if got := application.ActionItems.ProviderName(); got != "kimi" {
		t.Fatalf("unexpected action-items provider %s", got)
	}
	if got := application.TTSService.ProviderName(); got != "openai_compatible" {
		t.Fatalf("unexpected tts provider %s", got)
	}
}

func TestRunDoesNotAutoBootstrapAdminFromConfig(t *testing.T) {
	settingsService := admin.NewService(admin.NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	meetingStore := admin.NewMemoryMeetingStore()
	userService := admin.NewUserService(admin.NewMemoryUserStore(), meetingStore)
	authService := admin.NewAuthService(userService, admin.NewMemoryAuthStore())
	meetingService := admin.NewMeetingService(meetingStore)

	application := NewWithOptions(Options{
		UDPHost:        "127.0.0.1",
		UDPPort:        0,
		HTTPHost:       "127.0.0.1",
		HTTPPort:       0,
		AdminService:   settingsService,
		UserService:    userService,
		AuthService:    authService,
		MeetingService: meetingService,
		BootstrapAdmin: admin.BootstrapAdminConfig{
			Username:    "root-admin",
			Password:    "RootAdmin1234",
			DisplayName: "超级管理员",
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- application.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	user, found, err := userService.FindByUsername(context.Background(), "root-admin")
	if err != nil {
		t.Fatalf("find bootstrap admin: %v", err)
	}
	if found {
		t.Fatalf("expected startup to avoid auto-creating bootstrap admin, got %+v", user)
	}

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("app run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app run did not stop after cancellation")
	}
}

func TestRunStartsAdminHTTPServer(t *testing.T) {
	adminService := admin.NewService(admin.NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})

	application := NewWithOptions(Options{
		UDPHost:      "127.0.0.1",
		UDPPort:      0,
		HTTPHost:     "127.0.0.1",
		HTTPPort:     0,
		AdminService: adminService,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- application.Run(ctx)
	}()

	waitForCondition(t, time.Second, func() bool {
		return application.HTTPAddress() != ""
	})

	response, err := http.Get("http://" + application.HTTPAddress() + "/api/admin/health")
	if err != nil {
		t.Fatalf("request admin health: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected status %d body=%s", response.StatusCode, string(body))
	}

	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("app run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app run did not stop after cancellation")
	}
}

func TestRunBridgesMQTTControlAndUDPRuntimeToBroker(t *testing.T) {
	broker := &fakeBrokerClient{}

	application := NewWithOptions(Options{
		UDPHost:    "127.0.0.1",
		UDPPort:    0,
		MQTTClient: broker,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- application.Run(ctx)
	}()

	address := application.UDPServer.WaitUntilListening(time.Second)
	if address == "" {
		t.Fatal("udp server did not start listening")
	}

	waitForCondition(t, time.Second, func() bool {
		return broker.hasHandler()
	})

	if err := broker.Inject(mqtttransport.IncomingMessage{
		Topic: protocol.ControlTopic("client-a", "session-1"),
		Payload: []byte(`{
			"version": "v1",
			"messageId": "msg-1",
			"clientId": "client-a",
			"sessionId": "session-1",
			"type": "session/hello",
			"payload": {
				"title": "Run Bridge Test"
			}
		}`),
	}); err != nil {
		t.Fatalf("inject hello: %v", err)
	}

	if err := broker.Inject(mqtttransport.IncomingMessage{
		Topic: protocol.ControlTopic("client-a", "session-1"),
		Payload: []byte(`{
			"version": "v1",
			"messageId": "msg-2",
			"clientId": "client-a",
			"sessionId": "session-1",
			"type": "recording/start",
			"payload": {}
		}`),
	}); err != nil {
		t.Fatalf("inject start: %v", err)
	}

	connection, err := net.Dial("udp", address)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer connection.Close()

	packet, err := protocol.UDPAudioPacket{
		Version:     1,
		SourceType:  protocol.AudioSourceMixed,
		SessionID:   "session-1",
		Sequence:    1,
		StartedAtMS: 0,
		DurationMS:  200,
		Payload:     []byte{1, 2, 3, 4},
	}.Encode()
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	if _, err := connection.Write(packet); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		return broker.publishCount() >= 5
	})

	if !broker.hasPublishedTopic(protocol.ControlReplyTopic("client-a", "session-1")) {
		t.Fatal("expected control reply to be published through broker runtime")
	}
	if !broker.hasPublishedTopic(protocol.EventsTopic("client-a", "session-1")) {
		t.Fatal("expected lifecycle event to be published through broker runtime")
	}
	if !broker.hasPublishedTopic(protocol.SttTopic("client-a", "session-1")) {
		t.Fatal("expected stt delta to be published through broker runtime")
	}
	if !broker.hasPublishedTopic(protocol.SummaryTopic("client-a", "session-1")) {
		t.Fatal("expected summary delta to be published through broker runtime")
	}
	if !broker.hasPublishedTopic(protocol.ActionItemsTopic("client-a", "session-1")) {
		t.Fatal("expected action item delta to be published through broker runtime")
	}

	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("app run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app run did not stop after cancellation")
	}
}

func assertRealtimeMessages(t *testing.T, messages []protocol.RoutedMessage) {
	t.Helper()

	foundTranscript := false

	for _, message := range messages {
		if message.Type == protocol.TypeSTTDelta {
			foundTranscript = true
		}
	}

	if !foundTranscript {
		t.Fatalf("unexpected realtime message set: %+v", messages)
	}
}

type fakeBrokerClient struct {
	mu        sync.Mutex
	published []publishedMessage
	handler   func(mqtttransport.IncomingMessage)
}

type publishedMessage struct {
	topic string
}

func (c *fakeBrokerClient) Connect(context.Context) error {
	return nil
}

func (c *fakeBrokerClient) Subscribe(_ context.Context, _ string, _ byte, handler func(mqtttransport.IncomingMessage)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
	return nil
}

func (c *fakeBrokerClient) Publish(_ context.Context, topic string, _ byte, _ bool, _ []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, publishedMessage{topic: topic})
	return nil
}

func (c *fakeBrokerClient) Disconnect() {}

func (c *fakeBrokerClient) Inject(message mqtttransport.IncomingMessage) error {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()

	if handler == nil {
		return errors.New("no subscription handler registered")
	}

	handler(message)
	return nil
}

func (c *fakeBrokerClient) hasHandler() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handler != nil
}

func (c *fakeBrokerClient) publishCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.published)
}

func (c *fakeBrokerClient) hasPublishedTopic(topic string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, published := range c.published {
		if published.topic == topic {
			return true
		}
	}
	return false
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not met in time")
}
