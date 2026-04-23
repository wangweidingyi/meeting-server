package udp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"meeting-server/internal/protocol"
	"meeting-server/internal/session"
)

type MessageHandler func([]protocol.RoutedMessage)

type Server struct {
	host           string
	port           int
	sessionManager *session.Manager

	mu             sync.RWMutex
	messageHandler MessageHandler
	listenAddress  string
	listeningCh    chan struct{}
}

func NewServer(host string, port int, sessionManager *session.Manager) *Server {
	return &Server{
		host:           host,
		port:           port,
		sessionManager: sessionManager,
		listeningCh:    make(chan struct{}),
	}
}

func (s *Server) Address() (string, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listenAddress != "" {
		host, port, err := net.SplitHostPort(s.listenAddress)
		if err == nil {
			parsedPort, parseErr := net.LookupPort("udp", port)
			if parseErr == nil {
				return host, parsedPort
			}
		}
	}

	return s.host, s.port
}

func (s *Server) SetMessageHandler(handler MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messageHandler = handler
}

func (s *Server) WaitUntilListening(timeout time.Duration) string {
	select {
	case <-s.listeningCh:
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.listenAddress
	case <-time.After(timeout):
		return ""
	}
}

// HandleMixedAudio is the minimal ingest seam for the first server milestone.
func (s *Server) HandleMixedAudio(packet protocol.MixedAudioPacket) ([]protocol.RoutedMessage, error) {
	return s.sessionManager.HandleMixedAudio(packet)
}

func (s *Server) HandlePacketBytes(raw []byte) ([]protocol.RoutedMessage, error) {
	packet, err := protocol.DecodeUDPAudioPacket(raw)
	if err != nil {
		return nil, err
	}

	if packet.SourceType != protocol.AudioSourceMixed {
		return nil, nil
	}

	return s.HandleMixedAudio(packet.ToMixedAudioPacket())
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", s.host, s.port))
	if err != nil {
		return err
	}
	defer listener.Close()

	s.markListening(listener.LocalAddr().String())

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	buffer := make([]byte, 64*1024)
	for {
		n, _, err := listener.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		messages, err := s.HandlePacketBytes(buffer[:n])
		if err != nil {
			continue
		}

		if len(messages) == 0 {
			continue
		}

		s.emit(messages)
	}
}

func (s *Server) emit(messages []protocol.RoutedMessage) {
	s.mu.RLock()
	handler := s.messageHandler
	s.mu.RUnlock()

	if handler != nil {
		handler(messages)
	}
}

func (s *Server) markListening(address string) {
	s.mu.Lock()
	s.listenAddress = address
	listeningCh := s.listeningCh
	s.mu.Unlock()

	select {
	case <-listeningCh:
	default:
		close(listeningCh)
	}
}
