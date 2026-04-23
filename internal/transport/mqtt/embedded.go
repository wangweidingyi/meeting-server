package mqtt

import (
	"context"
	"fmt"
	"net"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

type EmbeddedBrokerConfig struct {
	Host string
	Port int
}

type EmbeddedBroker struct {
	config EmbeddedBrokerConfig
}

func NewEmbeddedBroker(config EmbeddedBrokerConfig) *EmbeddedBroker {
	return &EmbeddedBroker{config: config}
}

func (b *EmbeddedBroker) Run(ctx context.Context) error {
	server := mqtt.New(nil)
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", b.config.Host, b.config.Port))
	if err != nil {
		return err
	}

	if err := server.AddListener(listeners.NewNet("meeting-mqtt", listener)); err != nil {
		_ = listener.Close()
		return err
	}

	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	if err := server.Serve(); err != nil && ctx.Err() == nil {
		return err
	}

	return nil
}
