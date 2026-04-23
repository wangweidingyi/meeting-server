package mqtt

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"meeting-server/internal/protocol"
	"meeting-server/internal/session"
)

type Server struct {
	sessionManager *session.Manager
}

type ControlMessage struct {
	Type      string
	ClientID  string
	SessionID string
	Title     string
}

type RoutedReply = protocol.RoutedMessage

type controlEnvelope struct {
	ClientID  string          `json:"clientId"`
	SessionID string          `json:"sessionId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type helloPayload struct {
	Title string `json:"title"`
}

func NewServer(sessionManager *session.Manager) *Server {
	return &Server{
		sessionManager: sessionManager,
	}
}

func (s *Server) SessionManager() *session.Manager {
	return s.sessionManager
}

func (s *Server) HandleControlEnvelope(raw []byte) ([]RoutedReply, error) {
	message, err := ParseControlEnvelope(raw)
	if err != nil {
		return nil, err
	}

	return s.HandleControlMessage(message)
}

func (s *Server) HandleControlMessage(message ControlMessage) ([]RoutedReply, error) {
	switch message.Type {
	case protocol.TypeSessionHello:
		reply, err := s.sessionManager.HandleHello(session.HelloRequest{
			ClientID:  message.ClientID,
			SessionID: message.SessionID,
			Title:     message.Title,
		})
		if err != nil {
			return s.errorReply(message, err), nil
		}

		return []RoutedReply{{
			Topic:   protocol.ControlReplyTopic(message.ClientID, message.SessionID),
			Type:    reply.Type,
			Payload: reply,
		}}, nil
	case protocol.TypeSessionResume:
		if err := s.sessionManager.ResumeSession(message.SessionID); err != nil {
			return s.errorReply(message, err), nil
		}

		return []RoutedReply{{
			Topic: protocol.ControlReplyTopic(message.ClientID, message.SessionID),
			Type:  protocol.TypeAck,
			Payload: protocol.AckPayload{
				Accepted: true,
			},
		}}, nil
	case protocol.TypeRecordingStart:
		event, err := s.sessionManager.StartRecording(message.SessionID)
		if err != nil {
			return s.errorReply(message, err), nil
		}

		return []RoutedReply{{
			Topic:   protocol.EventsTopic(message.ClientID, message.SessionID),
			Type:    event.Type,
			Payload: event,
		}}, nil
	case protocol.TypeRecordingPause:
		event, err := s.sessionManager.PauseRecording(message.SessionID)
		if err != nil {
			return s.errorReply(message, err), nil
		}

		return []RoutedReply{{
			Topic:   protocol.EventsTopic(message.ClientID, message.SessionID),
			Type:    event.Type,
			Payload: event,
		}}, nil
	case protocol.TypeRecordingResume:
		event, err := s.sessionManager.ResumeRecording(message.SessionID)
		if err != nil {
			return s.errorReply(message, err), nil
		}

		return []RoutedReply{{
			Topic:   protocol.EventsTopic(message.ClientID, message.SessionID),
			Type:    event.Type,
			Payload: event,
		}}, nil
	case protocol.TypeHeartbeat:
		event, err := s.sessionManager.Heartbeat(message.SessionID)
		if err != nil {
			return s.errorReply(message, err), nil
		}

		return []RoutedReply{{
			Topic:   protocol.EventsTopic(message.ClientID, message.SessionID),
			Type:    event.Type,
			Payload: event,
		}}, nil
	case protocol.TypeRecordingStop:
		replies, err := s.sessionManager.StopRecording(message.SessionID)
		if err != nil {
			return s.errorReply(message, err), nil
		}

		return replies, nil
	default:
		return s.errorReply(message, errors.New("unsupported control message type")), nil
	}
}

func (s *Server) errorReply(message ControlMessage, err error) []RoutedReply {
	return []RoutedReply{{
		Topic: protocol.ControlReplyTopic(message.ClientID, message.SessionID),
		Type:  protocol.TypeError,
		Payload: protocol.ErrorPayload{
			Message: err.Error(),
		},
	}}
}

func ParseControlEnvelope(raw []byte) (ControlMessage, error) {
	var envelope controlEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ControlMessage{}, err
	}

	message := ControlMessage{
		Type:      envelope.Type,
		ClientID:  envelope.ClientID,
		SessionID: envelope.SessionID,
	}

	if envelope.Type == protocol.TypeSessionHello {
		var payload helloPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return ControlMessage{}, err
		}
		message.Title = payload.Title
	}

	return message, nil
}

func EncodeRoutedReply(reply RoutedReply) ([]byte, error) {
	envelope := protocol.Envelope[any]{
		Version:   "v1",
		MessageID: "server-reply",
		ClientID:  extractClientID(reply.Topic),
		SessionID: extractSessionID(reply.Topic),
		Seq:       1,
		SentAt:    time.Unix(0, 0).UTC().Format(time.RFC3339),
		Type:      reply.Type,
		Payload:   reply.Payload,
	}

	return protocol.EncodeEnvelope(envelope)
}

func extractClientID(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func extractSessionID(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}
