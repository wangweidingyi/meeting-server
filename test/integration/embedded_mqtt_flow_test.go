package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"meeting-server/internal/app"
	"meeting-server/internal/protocol"
	mqtttransport "meeting-server/internal/transport/mqtt"
)

func TestEmbeddedMQTTFlowBridgesControlAndRealtimeEvents(t *testing.T) {
	mqttPort := reserveTCPPort(t)
	udpPort := reserveUDPPort(t)

	application := app.NewWithOptions(app.Options{
		UDPHost:  "127.0.0.1",
		UDPPort:  udpPort,
		HTTPHost: "127.0.0.1",
		HTTPPort: 0,
		MQTTBroker: mqtttransport.NewEmbeddedBroker(mqtttransport.EmbeddedBrokerConfig{
			Host: "127.0.0.1",
			Port: mqttPort,
		}),
		MQTTClient: mqtttransport.NewPahoBrokerClient(mqtttransport.PahoClientOptions{
			BrokerURL: fmt.Sprintf("tcp://127.0.0.1:%d", mqttPort),
			ClientID:  "meeting-server-integration",
		}),
	})
	application.AdminService = nil
	application.AdminHandler = nil
	application.MeetingService = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- application.Run(ctx)
	}()

	waitForUDPListener(t, application)

	messages := newBrokerMessageCollector()
	client := connectIntegrationMQTTClient(t, mqttPort, messages)
	defer func() {
		client.Disconnect(250)
	}()

	subscribeTopic(t, client, protocol.ControlReplyTopic("client-a", "session-1"), messages.handle)
	subscribeTopic(t, client, protocol.EventsTopic("client-a", "session-1"), messages.handle)
	subscribeTopic(t, client, protocol.SttTopic("client-a", "session-1"), messages.handle)
	subscribeTopic(t, client, protocol.SummaryTopic("client-a", "session-1"), messages.handle)
	subscribeTopic(t, client, protocol.ActionItemsTopic("client-a", "session-1"), messages.handle)

	publishUntilReceived(t, client, messages, protocol.ControlTopic("client-a", "session-1"), map[string]any{
		"version":   "v1",
		"messageId": "hello-1",
		"clientId":  "client-a",
		"sessionId": "session-1",
		"type":      "session/hello",
		"payload": map[string]any{
			"title": "embedded mqtt integration",
		},
	}, protocol.TypeSessionHello)

	publishEnvelope(t, client, protocol.ControlTopic("client-a", "session-1"), map[string]any{
		"version":   "v1",
		"messageId": "start-1",
		"clientId":  "client-a",
		"sessionId": "session-1",
		"type":      "recording/start",
		"payload":   map[string]any{},
	})

	messages.waitForType(t, protocol.TypeRecordingStarted)

	connection, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(udpPort)))
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
		t.Fatalf("encode udp packet: %v", err)
	}

	if _, err := connection.Write(packet); err != nil {
		t.Fatalf("write udp packet: %v", err)
	}

	messages.waitForType(t, protocol.TypeSTTDelta)
	messages.waitForType(t, protocol.TypeSummaryDelta)
	messages.waitForType(t, protocol.TypeActionItemDelta)

	publishEnvelope(t, client, protocol.ControlTopic("client-a", "session-1"), map[string]any{
		"version":   "v1",
		"messageId": "stop-1",
		"clientId":  "client-a",
		"sessionId": "session-1",
		"type":      "recording/stop",
		"payload":   map[string]any{},
	})

	messages.waitForType(t, protocol.TypeSTTFinal)
	messages.waitForType(t, protocol.TypeSummaryFinal)
	messages.waitForType(t, protocol.TypeActionItemFinal)
	messages.waitForType(t, protocol.TypeRecordingStopped)

	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("app run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("application did not stop after cancellation")
	}
}

type brokerMessageCollector struct {
	mu       sync.Mutex
	received map[string][]map[string]any
}

func newBrokerMessageCollector() *brokerMessageCollector {
	return &brokerMessageCollector{
		received: make(map[string][]map[string]any),
	}
}

func (c *brokerMessageCollector) handle(_ paho.Client, message paho.Message) {
	var envelope map[string]any
	if err := json.Unmarshal(message.Payload(), &envelope); err != nil {
		return
	}

	messageType, _ := envelope["type"].(string)
	if messageType == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.received[messageType] = append(c.received[messageType], envelope)
}

func (c *brokerMessageCollector) waitForType(t *testing.T, messageType string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		count := len(c.received[messageType])
		c.mu.Unlock()
		if count > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for mqtt message type %s", messageType)
}

func (c *brokerMessageCollector) hasType(messageType string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.received[messageType]) > 0
}

func connectIntegrationMQTTClient(
	t *testing.T,
	port int,
	collector *brokerMessageCollector,
) paho.Client {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		options := paho.NewClientOptions()
		options.AddBroker("tcp://127.0.0.1:" + strconv.Itoa(port))
		options.SetClientID("desktop-integration-client")
		options.SetAutoReconnect(false)
		options.SetDefaultPublishHandler(collector.handle)

		client := paho.NewClient(options)
		token := client.Connect()
		if token.WaitTimeout(500*time.Millisecond) && token.Error() == nil {
			return client
		}

		client.Disconnect(50)
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("mqtt connect timed out for embedded broker on port %d", port)
	return nil
}

func subscribeTopic(
	t *testing.T,
	client paho.Client,
	topic string,
	handler paho.MessageHandler,
) {
	t.Helper()

	token := client.Subscribe(topic, 1, handler)
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatalf("subscribe timed out for %s", topic)
	}
	if err := token.Error(); err != nil {
		t.Fatalf("subscribe %s: %v", topic, err)
	}
}

func publishEnvelope(t *testing.T, client paho.Client, topic string, payload map[string]any) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	token := client.Publish(topic, 1, false, body)
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatalf("publish timed out for %s", topic)
	}
	if err := token.Error(); err != nil {
		t.Fatalf("publish %s: %v", topic, err)
	}
}

func publishUntilReceived(
	t *testing.T,
	client paho.Client,
	collector *brokerMessageCollector,
	topic string,
	payload map[string]any,
	messageType string,
) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		publishEnvelope(t, client, topic, payload)
		if collector.hasType(messageType) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for mqtt message type %s", messageType)
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func reserveUDPPort(t *testing.T) int {
	t.Helper()

	address, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve udp addr: %v", err)
	}
	connection, err := net.ListenUDP("udp", address)
	if err != nil {
		t.Fatalf("reserve udp port: %v", err)
	}
	defer connection.Close()

	return connection.LocalAddr().(*net.UDPAddr).Port
}

func waitForUDPListener(t *testing.T, application *app.App) {
	t.Helper()

	if address := application.UDPServer.WaitUntilListening(5 * time.Second); address == "" {
		t.Fatal("udp server did not start listening")
	}
}
