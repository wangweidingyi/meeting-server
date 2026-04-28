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
	"meeting-server/internal/model/llmproviders"
	openaicompat "meeting-server/internal/model/openai_compatible"
	"meeting-server/internal/pipeline/action_items"
	"meeting-server/internal/pipeline/stt"
	"meeting-server/internal/pipeline/summary"
	"meeting-server/internal/pipeline/tts"
	"meeting-server/internal/protocol"
	analysisruntime "meeting-server/internal/runtime/analysis"
	transcriptruntime "meeting-server/internal/runtime/transcripts"
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
	TranscriptStore    transcriptruntime.Store
	AdminService       *admin.Service
	UserService        *admin.UserService
	AuthService        *admin.AuthService
	MeetingService     *admin.MeetingService
	BootstrapAdmin     admin.BootstrapAdminConfig
}

type App struct {
	mu              sync.RWMutex
	SessionManager  *session.Manager
	MQTTServer      *mqtttransport.Server
	MQTTRuntime     *mqtttransport.Runtime
	MQTTBroker      *mqtttransport.EmbeddedBroker
	UDPServer       *udptransport.Server
	STTService      *stt.Service
	SummaryService  *summary.Service
	ActionItems     *action_items.Service
	TTSService      *tts.Service
	TranscriptStore transcriptruntime.Store
	AnalysisRuntime *analysisruntime.Runtime
	Publisher       RoutedMessagePublisher
	AdminService    *admin.Service
	UserService     *admin.UserService
	AuthService     *admin.AuthService
	MeetingService  *admin.MeetingService
	BootstrapAdmin  admin.BootstrapAdminConfig
	AdminHandler    http.Handler
	HTTPServer      *http.Server
	httpHost        string
	httpPort        int
	httpAddress     string
	closeAdmin      func()
}

func New() *App {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		panic(err)
	}

	return NewFromConfig(cfg)
}

func NewFromConfig(cfg config.Config) *App {
	if cfg.Database.URL == "" {
		panic("MEETING_DATABASE_URL is required")
	}

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
		TranscriptStore:    transcriptruntime.NewPostgresStore(cfg.Database.URL),
		BootstrapAdmin: admin.BootstrapAdminConfig{
			Username:    cfg.BootstrapAdmin.Username,
			Password:    cfg.BootstrapAdmin.Password,
			DisplayName: cfg.BootstrapAdmin.DisplayName,
		},
	})
	if cfg.MQTT.Embedded {
		app.MQTTBroker = mqtttransport.NewEmbeddedBroker(mqtttransport.EmbeddedBrokerConfig{
			Host: cfg.MQTT.ListenHost,
			Port: cfg.MQTT.ListenPort,
		})
	}

	var closeResources []func()
	closeResources = append(closeResources, app.TranscriptStore.Close)
	postgresStore := admin.NewPostgresStore(cfg.Database.URL)
	store := admin.Store(postgresStore)
	closeResources = append(closeResources, postgresStore.Close)

	postgresMeetingStore := admin.NewPostgresMeetingStore(cfg.Database.URL)
	meetingStore := admin.MeetingStore(postgresMeetingStore)
	closeResources = append(closeResources, postgresMeetingStore.Close)

	postgresUserStore := admin.NewPostgresUserStore(cfg.Database.URL)
	userStore := admin.UserStore(postgresUserStore)
	closeResources = append(closeResources, postgresUserStore.Close)

	postgresAuthStore := admin.NewPostgresAuthStore(cfg.Database.URL)
	authStore := admin.AuthStore(postgresAuthStore)
	closeResources = append(closeResources, postgresAuthStore.Close)

	adminService := admin.NewService(store, cfg.AI, func(next config.AIConfig) {
		app.ApplyAIConfig(next)
	})
	userService := admin.NewUserService(userStore, meetingStore)
	authService := admin.NewAuthService(userService, authStore)
	meetingService := admin.NewMeetingService(meetingStore)

	app.AdminService = adminService
	app.UserService = userService
	app.AuthService = authService
	app.MeetingService = meetingService
	app.BootstrapAdmin = admin.BootstrapAdminConfig{
		Username:    cfg.BootstrapAdmin.Username,
		Password:    cfg.BootstrapAdmin.Password,
		DisplayName: cfg.BootstrapAdmin.DisplayName,
	}
	app.AdminHandler = admin.NewHandler(adminService, userService, meetingService, authService)
	app.closeAdmin = func() {
		for _, closeFn := range closeResources {
			if closeFn != nil {
				closeFn()
			}
		}
	}
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
	transcriptStore := options.TranscriptStore
	if transcriptStore == nil {
		transcriptStore = transcriptruntime.NewMemoryStore()
	}

	basePublishers := []RoutedMessagePublisher{}
	if options.Publisher != nil {
		basePublishers = append(basePublishers, options.Publisher)
	}
	if len(basePublishers) == 0 {
		basePublishers = append(basePublishers, LogPublisher{})
	}
	publisher := &DynamicPublisher{}
	publisher.SetPublishers(basePublishers)

	analysisRuntime := analysisruntime.NewRuntime(analysisruntime.Options{
		TranscriptStore: transcriptStore,
		ActionItems:     actionItemsService,
		Summary:         summaryService,
		Publisher:       publisher,
	})

	sessionManager := session.NewManager(session.Options{
		UDPHost:            options.UDPHost,
		UDPPort:            options.UDPPort,
		STTService:         sttService,
		SummaryService:     summaryService,
		ActionItemsService: actionItemsService,
		TranscriptStore:    transcriptStore,
		AnalysisScheduler:  analysisRuntime,
	})
	mqttServer := mqtttransport.NewServer(sessionManager)
	var mqttRuntime *mqtttransport.Runtime
	if options.MQTTClient != nil {
		mqttRuntime = mqtttransport.NewRuntime(mqttServer, options.MQTTClient)
		publisher.SetPublishers(append([]RoutedMessagePublisher{mqttRuntime}, basePublishers...))
	}

	return &App{
		SessionManager:  sessionManager,
		MQTTServer:      mqttServer,
		MQTTRuntime:     mqttRuntime,
		MQTTBroker:      options.MQTTBroker,
		UDPServer:       udptransport.NewServer(options.UDPHost, options.UDPPort, sessionManager),
		STTService:      sttService,
		SummaryService:  summaryService,
		ActionItems:     actionItemsService,
		TTSService:      ttsService,
		TranscriptStore: transcriptStore,
		AnalysisRuntime: analysisRuntime,
		Publisher:       publisher,
		AdminService:    options.AdminService,
		UserService:     options.UserService,
		AuthService:     options.AuthService,
		MeetingService:  options.MeetingService,
		BootstrapAdmin:  options.BootstrapAdmin,
		AdminHandler:    adminHandler(options.AdminService, options.UserService, options.MeetingService, options.AuthService),
		httpHost:        options.HTTPHost,
		httpPort:        options.HTTPPort,
	}
}

func (a *App) Run(ctx context.Context) error {
	if a.AdminService != nil {
		if err := a.AdminService.Bootstrap(ctx); err != nil {
			return err
		}
		if a.UserService != nil {
			if _, err := a.UserService.List(ctx); err != nil {
				return err
			}
		}
		if a.AuthService != nil {
			if err := a.AuthService.EnsureReady(ctx); err != nil {
				return err
			}
		}
		if a.MeetingService != nil {
			if _, err := a.MeetingService.List(ctx); err != nil {
				return err
			}
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
			if address := a.MQTTBroker.WaitUntilListening(5 * time.Second); address == "" {
				return errors.New("embedded mqtt broker did not start listening")
			}
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
		if a.AnalysisRuntime != nil {
			a.AnalysisRuntime.Close()
		}
		return err
	case <-ctx.Done():
		<-done
		if a.AnalysisRuntime != nil {
			a.AnalysisRuntime.Close()
		}
		if a.closeAdmin != nil {
			a.closeAdmin()
		}
		return nil
	}
}

func (a *App) ApplyAIConfig(ai config.AIConfig) {
	a.STTService.SetConfig(ai.STT)
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

type DynamicPublisher struct {
	mu         sync.RWMutex
	publishers []RoutedMessagePublisher
}

func (p *DynamicPublisher) Publish(messages []protocol.RoutedMessage) {
	p.mu.RLock()
	publishers := append([]RoutedMessagePublisher(nil), p.publishers...)
	p.mu.RUnlock()

	for _, publisher := range publishers {
		publisher.Publish(messages)
	}
}

func (p *DynamicPublisher) SetPublishers(publishers []RoutedMessagePublisher) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.publishers = append([]RoutedMessagePublisher(nil), publishers...)
}

func buildSTTService(cfg config.Config) *stt.Service {
	service := stt.NewService()
	service.SetConfig(cfg.AI.STT)
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
	providerName, client, ok := llmproviders.NewChatClient(cfg)
	if !ok {
		return summary.StubGenerator{}
	}

	switch providerName {
	case "openai":
		return summary.NewOpenAIGenerator(client)
	case "deepseek":
		return summary.NewDeepSeekGenerator(client)
	case "kimi":
		return summary.NewKimiGenerator(client)
	case "openai_compatible":
		return summary.NewOpenAICompatibleGenerator(client)
	default:
		return summary.StubGenerator{}
	}
}

func actionItemsExtractor(cfg config.ModelProviderConfig) action_items.Extractor {
	providerName, client, ok := llmproviders.NewChatClient(cfg)
	if !ok {
		return action_items.StubExtractor{}
	}

	switch providerName {
	case "openai":
		return action_items.NewOpenAIExtractor(client)
	case "deepseek":
		return action_items.NewDeepSeekExtractor(client)
	case "kimi":
		return action_items.NewKimiExtractor(client)
	case "openai_compatible":
		return action_items.NewOpenAICompatibleExtractor(client)
	default:
		return action_items.StubExtractor{}
	}
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

func adminHandler(service *admin.Service, userService *admin.UserService, meetingService *admin.MeetingService, authService *admin.AuthService) http.Handler {
	if service == nil && userService == nil && meetingService == nil && authService == nil {
		return nil
	}

	if service == nil {
		service = admin.NewService(admin.NewMemoryStore(), config.AIConfig{}, func(config.AIConfig) {})
		_ = service.Bootstrap(context.Background())
	}

	return admin.NewHandler(service, userService, meetingService, authService)
}
