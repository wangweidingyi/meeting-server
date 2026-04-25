package mqtt

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

type EmbeddedBrokerConfig struct {
	Host string
	Port int
}

type EmbeddedBroker struct {
	config      EmbeddedBrokerConfig
	mu          sync.RWMutex
	address     string
	listeningCh chan string
}

func NewEmbeddedBroker(config EmbeddedBrokerConfig) *EmbeddedBroker {
	return &EmbeddedBroker{
		config:      config,
		listeningCh: make(chan string, 1),
	}
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

	b.markListening(listener.Addr().String())

	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	if err := server.Serve(); err != nil && ctx.Err() == nil {
		return err
	}

	return nil
}

func (b *EmbeddedBroker) WaitUntilListening(timeout time.Duration) string {
	b.mu.RLock()
	if b.address != "" {
		address := b.address
		b.mu.RUnlock()
		return address
	}
	b.mu.RUnlock()

	select {
	case address := <-b.listeningCh:
		b.mu.Lock()
		if b.address == "" {
			b.address = address
		}
		b.mu.Unlock()
		return address
	case <-time.After(timeout):
		return ""
	}
}

func (b *EmbeddedBroker) markListening(address string) {
	b.mu.Lock()
	if b.address != "" {
		b.mu.Unlock()
		return
	}
	b.address = address
	b.mu.Unlock()

	select {
	case b.listeningCh <- address:
	default:
	}
}
