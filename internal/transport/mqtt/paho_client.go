package mqtt

import (
	"context"
	"os"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type PahoClientOptions struct {
	BrokerURL string
	ClientID  string
	Username  string
	Password  string
	KeepAlive time.Duration
}

type PahoBrokerClient struct {
	client paho.Client
}

func NewPahoBrokerClient(options PahoClientOptions) *PahoBrokerClient {
	clientOptions := paho.NewClientOptions()
	clientOptions.AddBroker(options.BrokerURL)
	clientOptions.SetClientID(options.ClientID)
	clientOptions.SetUsername(options.Username)
	clientOptions.SetPassword(options.Password)
	clientOptions.SetAutoReconnect(true)
	clientOptions.SetConnectRetry(true)
	clientOptions.SetOrderMatters(false)

	if options.KeepAlive > 0 {
		clientOptions.SetKeepAlive(options.KeepAlive)
	}

	return &PahoBrokerClient{
		client: paho.NewClient(clientOptions),
	}
}

func NewPahoBrokerClientFromEnv() (*PahoBrokerClient, bool) {
	brokerURL := os.Getenv("MEETING_MQTT_BROKER")
	if brokerURL == "" {
		return nil, false
	}

	clientID := os.Getenv("MEETING_MQTT_CLIENT_ID")
	if clientID == "" {
		clientID = "meeting-server"
	}

	return NewPahoBrokerClient(PahoClientOptions{
		BrokerURL: brokerURL,
		ClientID:  clientID,
		Username:  os.Getenv("MEETING_MQTT_USERNAME"),
		Password:  os.Getenv("MEETING_MQTT_PASSWORD"),
		KeepAlive: 30 * time.Second,
	}), true
}

func (c *PahoBrokerClient) Connect(ctx context.Context) error {
	token := c.client.Connect()
	return waitForToken(ctx, token)
}

func (c *PahoBrokerClient) Subscribe(ctx context.Context, topic string, qos byte, handler func(IncomingMessage)) error {
	token := c.client.Subscribe(topic, qos, func(_ paho.Client, message paho.Message) {
		handler(IncomingMessage{
			Topic:   message.Topic(),
			Payload: append([]byte(nil), message.Payload()...),
		})
	})
	return waitForToken(ctx, token)
}

func (c *PahoBrokerClient) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	token := c.client.Publish(topic, qos, retained, payload)
	return waitForToken(ctx, token)
}

func (c *PahoBrokerClient) Disconnect() {
	c.client.Disconnect(250)
}

func waitForToken(ctx context.Context, token paho.Token) error {
	done := make(chan struct{})
	go func() {
		token.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return token.Error()
	}
}
