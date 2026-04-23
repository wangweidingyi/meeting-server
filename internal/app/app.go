package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"meeting-server/internal/admin"
	"meeting-server/internal/config"
	openaicompat "meeting-server/internal/model/openai_compatible"
	"meeting-server/internal/pipeline/action_items"
	"meeting-server/internal/pipeline/stt"
	"meeting-server/internal/pipeline/summary"
	"meeting-server/internal/pipeline/tts"
	"meeting-server/internal/protocol"
	"meeting-server/internal/session"
	mqtttransport "meeting-server/internal/transport/mqtt"
	udptransport "meeting-server/internal/transport/udp"
)

type RoutedMessagePublisher interface {
	Publish(messages []protocol.RoutedMessage)
}

type LogPublisher struct{}

func (LogPublisher) Publish(messages []protocol.RoutedMessage) {
	for _, message := range messages {
		fmt.Printf("published topic=%s type=%s\n", message.Topic, message.Type)
	}
}

type Options struct {
	UDPHost            string
	UDPPort            int
	HTTPHost           string
	HTTPPort           int
	Publisher          RoutedMessagePublisher
	MQTTClient         mqtttransport.BrokerClient
	MQTTBroker         *mqtttransport.EmbeddedBroker
	STTService         *stt.Service
	SummaryService     *summary.Service
	ActionItemsService *action_items.Service
	TTSService         *tts.Service
	AdminService       *admin.Service
}

type App struct {
	mu             sync.RWMutex
	SessionManager *session.Manager
	MQTTServer     *mqtttransport.Server
	MQTTRuntime    *mqtttransport.Runtime
	MQTTBroker     *mqtttransport.EmbeddedBroker
	UDPServer      *udptransport.Server
	STTService     *stt.Service
	SummaryService *summary.Service
	ActionItems    *action_items.Service
	TTSService     *tts.Service
	Publisher      RoutedMessagePublisher
	AdminService   *admin.Service
	AdminHandler   http.Handler
	HTTPServer     *http.Server
	httpHost       string
	httpPort       int
	httpAddress    string
	closeAdmin     func()
}

func New() *App {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		panic(err)
	}

	return NewFromConfig(cfg)
}

func NewFromConfig(cfg config.Config) *App {
	var mqttClient mqtttransport.BrokerClient
	if cfg.MQTT.Enabled {
		brokerURL := mqttBrokerURL(cfg.MQTT)
		if brokerURL != "" {
			mqttClient = mqtttransport.NewPahoBrokerClient(mqtttransport.PahoClientOptions{
				BrokerURL: brokerURL,
				ClientID:  cfg.MQTT.ClientID,
				Username:  cfg.MQTT.Username,
				Password:  cfg.MQTT.Password,
			})
		}
	}

	app := NewWithOptions(Options{
		UDPHost:            cfg.UDP.Host,
		UDPPort:            cfg.UDP.Port,
		HTTPHost:           cfg.HTTP.Host,
		HTTPPort:           cfg.HTTP.Port,
		Publisher:          LogPublisher{},
		MQTTClient:         mqttClient,
		STTService:         buildSTTService(cfg),
		SummaryService:     buildSummaryService(cfg),
		ActionItemsService: buildActionItemsService(cfg),
		TTSService:         buildTTSService(cfg),
	})
	if cfg.MQTT.Embedded {
		app.MQTTBroker = mqtttransport.NewEmbeddedBroker(mqtttransport.EmbeddedBrokerConfig{
			Host: cfg.MQTT.ListenHost,
			Port: cfg.MQTT.ListenPort,
		})
	}

	store := admin.Store(admin.NewMemoryStore())
	var closeAdmin func()
	if cfg.Database.URL != "" {
		postgresStore := admin.NewPostgresStore(cfg.Database.URL)
		store = postgresStore
		closeAdmin = postgresStore.Close
	}

	adminService := admin.NewService(store, cfg.AI, func(next config.AIConfig) {
		app.ApplyAIConfig(next)
	})

	app.AdminService = adminService
	app.AdminHandler = admin.NewHandler(adminService)
	app.closeAdmin = closeAdmin
	app.httpHost = cfg.HTTP.Host
	app.httpPort = cfg.HTTP.Port

	return app
}

func NewWithOptions(options Options) *App {
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
	ttsService := options.TTSService
	if ttsService == nil {
		ttsService = tts.NewService()
	}

	sessionManager := session.NewManager(session.Options{
		UDPHost:            options.UDPHost,
		UDPPort:            options.UDPPort,
		STTService:         sttService,
		SummaryService:     summaryService,
		ActionItemsService: actionItemsService,
	})

	mqttServer := mqtttransport.NewServer(sessionManager)
	var mqttRuntime *mqtttransport.Runtime
	if options.MQTTClient != nil {
		mqttRuntime = mqtttransport.NewRuntime(mqttServer, options.MQTTClient)
	}

	publishers := []RoutedMessagePublisher{}
	if mqttRuntime != nil {
		publishers = append(publishers, mqttRuntime)
	}
	if options.Publisher != nil {
		publishers = append(publishers, options.Publisher)
	}
	if len(publishers) == 0 {
		publishers = append(publishers, LogPublisher{})
	}

	publisher := MultiPublisher{
		publishers: publishers,
	}

	return &App{
		SessionManager: sessionManager,
		MQTTServer:     mqttServer,
		MQTTRuntime:    mqttRuntime,
		MQTTBroker:     options.MQTTBroker,
		UDPServer:      udptransport.NewServer(options.UDPHost, options.UDPPort, sessionManager),
		STTService:     sttService,
		SummaryService: summaryService,
		ActionItems:    actionItemsService,
		TTSService:     ttsService,
		Publisher:      publisher,
		AdminService:   options.AdminService,
		AdminHandler:   adminHandler(options.AdminService),
		httpHost:       options.HTTPHost,
		httpPort:       options.HTTPPort,
	}
}

func (a *App) Run(ctx context.Context) error {
	if a.AdminService != nil {
		if err := a.AdminService.Bootstrap(ctx); err != nil {
			return err
		}
	}

	a.UDPServer.SetMessageHandler(func(messages []protocol.RoutedMessage) {
		a.Publisher.Publish(messages)
	})

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	runComponent := func(run func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}

	if a.MQTTRuntime != nil {
		if a.MQTTBroker != nil {
			runComponent(a.MQTTBroker.Run)
		}
		runComponent(a.MQTTRuntime.Run)
	}

	runComponent(a.UDPServer.ListenAndServe)
	if a.AdminService != nil {
		runComponent(a.runHTTPServer)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		<-done
		if a.closeAdmin != nil {
			a.closeAdmin()
		}
		return nil
	}
}

func (a *App) ApplyAIConfig(ai config.AIConfig) {
	a.STTService.SetProvider(ai.STT.Provider, ai.STT.BaseURL, ai.STT.APIKey, ai.STT.Model)
	a.SummaryService.SetGenerator(summaryGenerator(ai.LLM))
	a.ActionItems.SetExtractor(actionItemsExtractor(ai.LLM))
	a.TTSService.SetSynthesizer(ttsSynthesizer(ai.TTS))
}

func (a *App) HTTPAddress() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.httpAddress
}

func (a *App) runHTTPServer(ctx context.Context) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(a.httpHost, strconv.Itoa(a.httpPort)))
	if err != nil {
		return err
	}

	server := &http.Server{
		Handler: a.AdminHandler,
	}

	a.mu.Lock()
	a.httpAddress = listener.Addr().String()
	a.HTTPServer = server
	a.mu.Unlock()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

type MultiPublisher struct {
	publishers []RoutedMessagePublisher
}

func (p MultiPublisher) Publish(messages []protocol.RoutedMessage) {
	for _, publisher := range p.publishers {
		publisher.Publish(messages)
	}
}

func buildSTTService(cfg config.Config) *stt.Service {
	service := stt.NewService()
	service.SetProvider(cfg.AI.STT.Provider, cfg.AI.STT.BaseURL, cfg.AI.STT.APIKey, cfg.AI.STT.Model)
	return service
}

func buildSummaryService(cfg config.Config) *summary.Service {
	return summary.NewService(summary.WithGenerator(summaryGenerator(cfg.AI.LLM)))
}

func buildActionItemsService(cfg config.Config) *action_items.Service {
	return action_items.NewService(action_items.WithExtractor(actionItemsExtractor(cfg.AI.LLM)))
}

func buildTTSService(cfg config.Config) *tts.Service {
	return tts.NewService(tts.WithSynthesizer(ttsSynthesizer(cfg.AI.TTS)))
}

func mqttBrokerURL(cfg config.MQTTConfig) string {
	if cfg.BrokerURL != "" {
		return cfg.BrokerURL
	}
	if cfg.Embedded && cfg.ListenPort > 0 {
		host := cfg.ListenHost
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		return fmt.Sprintf("tcp://%s:%d", host, cfg.ListenPort)
	}

	return ""
}

func summaryGenerator(cfg config.ModelProviderConfig) summary.Generator {
	if cfg.Provider == "openai_compatible" {
		return summary.NewOpenAICompatibleGenerator(&openaicompat.ChatClient{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		})
	}

	return summary.StubGenerator{}
}

func actionItemsExtractor(cfg config.ModelProviderConfig) action_items.Extractor {
	if cfg.Provider == "openai_compatible" {
		return action_items.NewOpenAICompatibleExtractor(&openaicompat.ChatClient{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		})
	}

	return action_items.StubExtractor{}
}

func ttsSynthesizer(cfg config.SpeechProviderConfig) tts.Synthesizer {
	if cfg.Provider == "openai_compatible" {
		return tts.NewOpenAICompatibleSynthesizer(&openaicompat.SpeechClient{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			Voice:   cfg.Voice,
		})
	}

	return tts.StubSynthesizer{}
}

func adminHandler(service *admin.Service) http.Handler {
	if service == nil {
		return nil
	}

	return admin.NewHandler(service)
}
