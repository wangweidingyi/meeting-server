package mqtt

import (
	"context"
	"errors"

	"meeting-server/internal/protocol"
)

type IncomingMessage struct {
	Topic   string
	Payload []byte
}

type BrokerClient interface {
	Connect(ctx context.Context) error
	Subscribe(ctx context.Context, topic string, qos byte, handler func(IncomingMessage)) error
	Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error
	Disconnect()
}

type Runtime struct {
	server *Server
	client BrokerClient
}

func NewRuntime(server *Server, client BrokerClient) *Runtime {
	return &Runtime{
		server: server,
		client: client,
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	if r.client == nil {
		return errors.New("mqtt broker client is not configured")
	}

	if err := r.client.Connect(ctx); err != nil {
		return err
	}
	defer r.client.Disconnect()

	errCh := make(chan error, 1)
	if err := r.client.Subscribe(ctx, protocol.ControlSubscriptionTopic(), 1, func(message IncomingMessage) {
		replies, err := r.server.HandleControlEnvelope(message.Payload)
		if err != nil {
			pushRuntimeError(errCh, err)
			return
		}

		if err := r.PublishWithContext(ctx, replies); err != nil {
			pushRuntimeError(errCh, err)
		}
	}); err != nil {
		return err
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return nil
	}
}

func (r *Runtime) Publish(messages []protocol.RoutedMessage) {
	_ = r.publishWithContext(context.Background(), messages)
}

func (r *Runtime) PublishWithContext(ctx context.Context, messages []protocol.RoutedMessage) error {
	return r.publishWithContext(ctx, messages)
}

func (r *Runtime) publishWithContext(ctx context.Context, messages []protocol.RoutedMessage) error {
	if r.client == nil {
		return errors.New("mqtt broker client is not configured")
	}

	for _, message := range messages {
		payload, err := EncodeRoutedReply(message)
		if err != nil {
			return err
		}

		policy := publishPolicyFor(message)
		if err := r.client.Publish(ctx, message.Topic, policy.qos, policy.retained, payload); err != nil {
			return err
		}
	}

	return nil
}

type publishPolicy struct {
	qos      byte
	retained bool
}

func publishPolicyFor(message protocol.RoutedMessage) publishPolicy {
	switch message.Type {
	case protocol.TypeSTTDelta, protocol.TypeSummaryDelta, protocol.TypeActionItemDelta, protocol.TypeHeartbeat:
		return publishPolicy{qos: 0, retained: false}
	default:
		return publishPolicy{qos: 1, retained: false}
	}
}

func pushRuntimeError(errCh chan error, err error) {
	select {
	case errCh <- err:
	default:
	}
}
