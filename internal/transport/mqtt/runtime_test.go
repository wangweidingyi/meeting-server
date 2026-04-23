package mqtt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"meeting-server/internal/protocol"
	"meeting-server/internal/session"
)

type publishedMessage struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

type fakeBrokerClient struct {
	mu           sync.Mutex
	subscribedTo []string
	published    []publishedMessage
	handler      func(IncomingMessage)
	connected    bool
}

func (c *fakeBrokerClient) Connect(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = true
	return nil
}

func (c *fakeBrokerClient) Subscribe(_ context.Context, topic string, _ byte, handler func(IncomingMessage)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscribedTo = append(c.subscribedTo, topic)
	c.handler = handler
	return nil
}

func (c *fakeBrokerClient) Publish(_ context.Context, topic string, qos byte, retained bool, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, publishedMessage{
		topic:    topic,
		qos:      qos,
		retained: retained,
		payload:  append([]byte(nil), payload...),
	})
	return nil
}

func (c *fakeBrokerClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
}

func (c *fakeBrokerClient) Inject(message IncomingMessage) error {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()

	if handler == nil {
		return errors.New("no subscription handler registered")
	}

	handler(message)
	return nil
}

func TestRuntimeSubscribesAndPublishesHelloReply(t *testing.T) {
	client := &fakeBrokerClient{}
	runtime := NewRuntime(
		NewServer(session.NewManager(session.Options{
			UDPHost: "127.0.0.1",
			UDPPort: 6000,
		})),
		client,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.Run(ctx)
	}()

	waitForCondition(t, time.Second, func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return len(client.subscribedTo) == 1
	})

	if err := client.Inject(IncomingMessage{
		Topic: protocol.ControlTopic("client-a", "session-1"),
		Payload: []byte(`{
			"version": "v1",
			"messageId": "msg-1",
			"clientId": "client-a",
			"sessionId": "session-1",
			"type": "session/hello",
			"payload": {
				"title": "运行时测试"
			}
		}`),
	}); err != nil {
		t.Fatalf("inject message: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return len(client.published) == 1
	})

	client.mu.Lock()
	if client.subscribedTo[0] != protocol.ControlSubscriptionTopic() {
		client.mu.Unlock()
		t.Fatalf("unexpected subscription topic %s", client.subscribedTo[0])
	}
	if client.published[0].topic != protocol.ControlReplyTopic("client-a", "session-1") {
		client.mu.Unlock()
		t.Fatalf("unexpected published topic %s", client.published[0].topic)
	}
	if client.published[0].qos != 1 {
		client.mu.Unlock()
		t.Fatalf("expected qos 1 for control reply, got %d", client.published[0].qos)
	}
	client.mu.Unlock()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runtime returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after cancellation")
	}
}

func TestRuntimePublisherUsesLowQosForRealtimeDelta(t *testing.T) {
	client := &fakeBrokerClient{}
	runtime := NewRuntime(
		NewServer(session.NewManager(session.Options{
			UDPHost: "127.0.0.1",
			UDPPort: 6000,
		})),
		client,
	)

	if err := runtime.PublishWithContext(context.Background(), []protocol.RoutedMessage{{
		Topic: protocol.SttTopic("client-a", "session-1"),
		Type:  protocol.TypeSTTDelta,
		Payload: protocol.TranscriptPayload{
			SegmentID: "segment-1",
			StartMS:   0,
			EndMS:     1200,
			Text:      "delta",
			IsFinal:   false,
			Revision:  1,
		},
	}}); err != nil {
		t.Fatalf("publish delta: %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if len(client.published) != 1 {
		t.Fatalf("expected one publish, got %d", len(client.published))
	}
	if client.published[0].qos != 0 {
		t.Fatalf("expected qos 0 for realtime delta, got %d", client.published[0].qos)
	}
	if client.published[0].retained {
		t.Fatal("expected realtime delta not to be retained")
	}
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
