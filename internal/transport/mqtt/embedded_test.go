package mqtt

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"meeting-server/internal/protocol"
)

func TestEmbeddedBrokerAcceptsLivePahoConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	broker := NewEmbeddedBroker(EmbeddedBrokerConfig{
		Host: "127.0.0.1",
		Port: port,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- broker.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	client := NewPahoBrokerClient(PahoClientOptions{
		BrokerURL: "tcp://127.0.0.1:" + strconv.Itoa(port),
		ClientID:  "embedded-broker-test",
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect paho client: %v", err)
	}
	defer client.Disconnect()

	received := make(chan IncomingMessage, 1)
	if err := client.Subscribe(context.Background(), protocol.SttTopic("client-a", "session-1"), 0, func(message IncomingMessage) {
		received <- message
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := client.Publish(
		context.Background(),
		protocol.SttTopic("client-a", "session-1"),
		0,
		false,
		[]byte("hello mqtt"),
	); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case message := <-received:
		if string(message.Payload) != "hello mqtt" {
			t.Fatalf("unexpected payload %q", string(message.Payload))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected embedded broker message delivery")
	}

	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("embedded broker exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("embedded broker did not stop after cancellation")
	}
}
