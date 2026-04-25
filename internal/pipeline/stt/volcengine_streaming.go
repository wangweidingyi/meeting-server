package stt

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"meeting-server/internal/config"
	"meeting-server/internal/protocol"
)

const (
	volcengineProtocolVersion   = 0x1
	volcengineHeaderSizeUnits   = 0x1
	volcengineMessageTypeFull   = 0x1
	volcengineMessageTypeAudio  = 0x2
	volcengineMessageTypeServer = 0x9
	volcengineMessageTypeError  = 0xF

	volcengineSerializationNone = 0x0
	volcengineSerializationJSON = 0x1

	volcengineCompressionNone = 0x0
	volcengineCompressionGzip = 0x1

	volcengineFlagNoSequence = 0x0
	volcengineFlagLastPacket = 0x2
)

type VolcengineStreamingFactory struct {
	cfg config.STTProviderConfig
}

func NewVolcengineStreamingFactory(cfg config.STTProviderConfig) *VolcengineStreamingFactory {
	return &VolcengineStreamingFactory{cfg: cloneSTTProviderConfig(cfg)}
}

func (f *VolcengineStreamingFactory) Name() string {
	return "volcengine_streaming"
}

func (f *VolcengineStreamingFactory) NewSession(sessionID string) StreamSession {
	return &volcengineStreamingSession{
		cfg:       cloneSTTProviderConfig(f.cfg),
		sessionID: sessionID,
		connectID: randomConnectID(),
	}
}

type VolcengineStreamingClient struct {
	cfg config.STTProviderConfig
}

func NewVolcengineStreamingClient(cfg config.STTProviderConfig) *VolcengineStreamingClient {
	return &VolcengineStreamingClient{cfg: cloneSTTProviderConfig(cfg)}
}

func (c *VolcengineStreamingClient) SmokeTest(ctx context.Context) error {
	session := &volcengineStreamingSession{
		cfg:       cloneSTTProviderConfig(c.cfg),
		sessionID: "config-test",
		connectID: randomConnectID(),
	}
	defer func() {
		_ = session.Close()
	}()

	if err := session.ensureConnected(ctx); err != nil {
		return err
	}

	_, _, err := session.readAvailableResponses(150 * time.Millisecond)
	if err != nil {
		return err
	}

	return nil
}

type volcengineStreamingSession struct {
	cfg                  config.STTProviderConfig
	sessionID            string
	connectID            string
	conn                 *websocket.Conn
	responses            chan volcengineServerResponse
	readErrCh            chan error
	pending              *protocol.MixedAudioPacket
	cumulativeTranscript string
	nextRevision         uint64
	firstStartMS         uint64
	lastEndMS            uint64
	lastDeltaEndMS       uint64
}

func (s *volcengineStreamingSession) Consume(ctx context.Context, packet protocol.MixedAudioPacket) (protocol.TranscriptPayload, bool, error) {
	if len(packet.Payload) == 0 {
		return protocol.TranscriptPayload{}, false, nil
	}
	if s.firstStartMS == 0 {
		s.firstStartMS = packet.StartedAtMS
	}
	if s.pending == nil {
		s.pending = cloneMixedAudioPacket(packet)
		return protocol.TranscriptPayload{}, false, nil
	}

	toSend := cloneMixedAudioPacket(*s.pending)
	s.pending = cloneMixedAudioPacket(packet)

	if err := s.ensureConnected(ctx); err != nil {
		return protocol.TranscriptPayload{}, false, err
	}
	if err := s.sendAudioPacket(*toSend, false); err != nil {
		return protocol.TranscriptPayload{}, false, err
	}

	response, ok, err := s.readAvailableResponses(120 * time.Millisecond)
	if err != nil {
		return protocol.TranscriptPayload{}, false, err
	}
	if !ok {
		return protocol.TranscriptPayload{}, false, nil
	}

	return s.emitDelta(response.Text)
}

func (s *volcengineStreamingSession) Flush(ctx context.Context, sessionID string) (protocol.TranscriptPayload, bool, error) {
	if s.pending == nil && strings.TrimSpace(s.cumulativeTranscript) == "" {
		return protocol.TranscriptPayload{}, false, nil
	}
	if err := s.ensureConnected(ctx); err != nil {
		return protocol.TranscriptPayload{}, false, err
	}

	if s.pending != nil {
		if err := s.sendAudioPacket(*s.pending, true); err != nil {
			return protocol.TranscriptPayload{}, false, err
		}
		s.pending = nil
	}

	response, ok, err := s.readAvailableResponses(2 * time.Second)
	if err != nil {
		return protocol.TranscriptPayload{}, false, err
	}

	text := s.cumulativeTranscript
	if ok && strings.TrimSpace(response.Text) != "" {
		text = strings.TrimSpace(response.Text)
		s.cumulativeTranscript = text
	}
	if strings.TrimSpace(text) == "" {
		return protocol.TranscriptPayload{}, false, nil
	}

	s.nextRevision++
	return protocol.TranscriptPayload{
		SegmentID: fmt.Sprintf("%s-final", sessionID),
		StartMS:   s.firstStartMS,
		EndMS:     s.lastEndMS,
		Text:      text,
		IsFinal:   true,
		Revision:  s.nextRevision,
	}, true, nil
}

func (s *volcengineStreamingSession) Close() error {
	if s.conn == nil {
		return nil
	}

	err := s.conn.Close()
	s.conn = nil
	return err
}

func (s *volcengineStreamingSession) ensureConnected(ctx context.Context) error {
	if s.conn != nil {
		return nil
	}

	if strings.TrimSpace(s.cfg.BaseURL) == "" {
		return errors.New("volcengine streaming base url is required")
	}
	if strings.TrimSpace(s.cfg.APIKey) == "" {
		return errors.New("volcengine streaming access key is required")
	}
	if strings.TrimSpace(s.cfg.Options["appKey"]) == "" {
		return errors.New("volcengine streaming appKey is required")
	}
	if strings.TrimSpace(s.cfg.Options["resourceId"]) == "" {
		return errors.New("volcengine streaming resourceId is required")
	}

	header := http.Header{}
	header.Set("X-Api-App-Key", s.cfg.Options["appKey"])
	header.Set("X-Api-Access-Key", s.cfg.APIKey)
	header.Set("X-Api-Resource-Id", s.cfg.Options["resourceId"])
	header.Set("X-Api-Connect-Id", s.connectID)

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	connection, _, err := dialer.DialContext(ctx, s.cfg.BaseURL, header)
	if err != nil {
		return err
	}

	s.conn = connection
	s.responses = make(chan volcengineServerResponse, 32)
	s.readErrCh = make(chan error, 1)
	go s.pumpResponses()
	return s.sendInitRequest()
}

func (s *volcengineStreamingSession) sendInitRequest() error {
	payload, err := buildVolcengineInitPayload(s.cfg, s.sessionID)
	if err != nil {
		return err
	}

	frame, err := encodeVolcengineClientRequest(volcengineMessageTypeFull, volcengineFlagNoSequence, volcengineSerializationJSON, volcengineCompressionGzip, payload)
	if err != nil {
		return err
	}

	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcengineStreamingSession) sendAudioPacket(packet protocol.MixedAudioPacket, isFinal bool) error {
	flags := byte(volcengineFlagNoSequence)
	if isFinal {
		flags = byte(volcengineFlagLastPacket)
	}

	frame, err := encodeVolcengineClientRequest(volcengineMessageTypeAudio, flags, volcengineSerializationNone, volcengineCompressionGzip, packet.Payload)
	if err != nil {
		return err
	}
	s.lastEndMS = packet.StartedAtMS + uint64(packet.DurationMS)

	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcengineStreamingSession) readAvailableResponses(wait time.Duration) (volcengineServerResponse, bool, error) {
	var (
		latest volcengineServerResponse
		seen   bool
	)

	timer := time.NewTimer(wait)
	defer timer.Stop()

	for {
		select {
		case response := <-s.responses:
			if strings.TrimSpace(response.Text) != "" {
				latest = response
				seen = true
			}
			if response.IsFinal {
				return latest, seen, nil
			}
		case err := <-s.readErrCh:
			if err != nil && !isNormalWebsocketClose(err) {
				return volcengineServerResponse{}, false, err
			}
			return latest, seen, nil
		case <-timer.C:
			return latest, seen, nil
		}
	}
}

func (s *volcengineStreamingSession) emitDelta(text string) (protocol.TranscriptPayload, bool, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return protocol.TranscriptPayload{}, false, nil
	}

	delta := transcriptDelta(s.cumulativeTranscript, text)
	s.cumulativeTranscript = text
	if delta == "" {
		return protocol.TranscriptPayload{}, false, nil
	}

	s.nextRevision++
	startMS := s.firstStartMS
	if s.lastDeltaEndMS != 0 {
		startMS = s.lastDeltaEndMS
	}
	s.lastDeltaEndMS = s.lastEndMS

	return protocol.TranscriptPayload{
		SegmentID: fmt.Sprintf("%s-%d", s.sessionID, s.nextRevision),
		StartMS:   startMS,
		EndMS:     s.lastEndMS,
		Text:      delta,
		IsFinal:   false,
		Revision:  s.nextRevision,
	}, true, nil
}

type volcengineServerResponse struct {
	Text    string
	IsFinal bool
}

type volcengineResponseEnvelope struct {
	Result struct {
		Text string `json:"text"`
	} `json:"result"`
}

func buildVolcengineInitPayload(cfg config.STTProviderConfig, sessionID string) ([]byte, error) {
	audio := map[string]any{
		"format":  stringOption(cfg.Options, "audioFormat", "pcm"),
		"codec":   stringOption(cfg.Options, "audioCodec", "raw"),
		"rate":    intOption(cfg.Options, "sampleRate", 16000),
		"bits":    intOption(cfg.Options, "bits", 16),
		"channel": intOption(cfg.Options, "channels", 1),
	}
	if language := strings.TrimSpace(cfg.Options["language"]); language != "" && shouldIncludeVolcengineLanguage(cfg.BaseURL) {
		audio["language"] = language
	}

	request := map[string]any{
		// The streaming protocol uses resourceId in headers to select the actual service tier.
		// request.model_name remains the fixed protocol value "bigmodel".
		"model_name":      "bigmodel",
		"enable_itn":      boolOption(cfg.Options, "enableItn", true),
		"enable_punc":     boolOption(cfg.Options, "enablePunc", true),
		"show_utterances": boolOption(cfg.Options, "showUtterances", true),
	}
	if _, ok := cfg.Options["enableNonstream"]; ok {
		request["enable_nonstream"] = boolOption(cfg.Options, "enableNonstream", false)
	}
	if resultType := strings.TrimSpace(cfg.Options["resultType"]); resultType != "" {
		request["result_type"] = resultType
	}
	if endWindowSize := intOption(cfg.Options, "endWindowSize", 0); endWindowSize > 0 {
		request["end_window_size"] = endWindowSize
	}

	payload := map[string]any{
		"user": map[string]any{
			"uid": sessionID,
		},
		"audio":   audio,
		"request": request,
	}

	return json.Marshal(payload)
}

func encodeVolcengineClientRequest(messageType byte, flags byte, serialization byte, compression byte, payload []byte) ([]byte, error) {
	encodedPayload := payload
	var err error
	if compression == volcengineCompressionGzip {
		encodedPayload, err = gzipBytes(payload)
		if err != nil {
			return nil, err
		}
	}

	frame := make([]byte, 8+len(encodedPayload))
	frame[0] = byte(volcengineProtocolVersion<<4 | volcengineHeaderSizeUnits)
	frame[1] = byte(messageType<<4 | flags)
	frame[2] = byte(serialization<<4 | compression)
	frame[3] = 0x00
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(encodedPayload)))
	copy(frame[8:], encodedPayload)

	return frame, nil
}

func decodeVolcengineServerMessage(frame []byte) (volcengineServerResponse, error) {
	if len(frame) < 4 {
		return volcengineServerResponse{}, errors.New("volcengine frame is too short")
	}

	headerSize := int(frame[0]&0x0F) * 4
	if len(frame) < headerSize {
		return volcengineServerResponse{}, errors.New("volcengine frame header is truncated")
	}

	messageType := frame[1] >> 4
	flags := frame[1] & 0x0F
	serialization := frame[2] >> 4
	compression := frame[2] & 0x0F

	cursor := headerSize
	switch messageType {
	case volcengineMessageTypeServer:
		if flags == 0x1 || flags == 0x3 {
			if len(frame) < cursor+4 {
				return volcengineServerResponse{}, errors.New("volcengine server frame missing sequence")
			}
			cursor += 4
		}
		if len(frame) < cursor+4 {
			return volcengineServerResponse{}, errors.New("volcengine server frame missing payload size")
		}
		payloadSize := int(binary.BigEndian.Uint32(frame[cursor : cursor+4]))
		cursor += 4
		if len(frame) < cursor+payloadSize {
			return volcengineServerResponse{}, errors.New("volcengine server frame payload is truncated")
		}
		payload := append([]byte(nil), frame[cursor:cursor+payloadSize]...)
		decoded, err := decodeVolcenginePayload(payload, serialization, compression)
		if err != nil {
			return volcengineServerResponse{}, err
		}
		var envelope volcengineResponseEnvelope
		if err := json.Unmarshal(decoded, &envelope); err != nil {
			return volcengineServerResponse{}, err
		}
		return volcengineServerResponse{
			Text:    strings.TrimSpace(envelope.Result.Text),
			IsFinal: flags == 0x3 || flags == volcengineFlagLastPacket,
		}, nil
	case volcengineMessageTypeError:
		if len(frame) < cursor+8 {
			return volcengineServerResponse{}, errors.New("volcengine error frame is truncated")
		}
		code := binary.BigEndian.Uint32(frame[cursor : cursor+4])
		cursor += 4
		payloadSize := int(binary.BigEndian.Uint32(frame[cursor : cursor+4]))
		cursor += 4
		if len(frame) < cursor+payloadSize {
			return volcengineServerResponse{}, errors.New("volcengine error payload is truncated")
		}
		message := strings.TrimSpace(string(frame[cursor : cursor+payloadSize]))
		return volcengineServerResponse{}, fmt.Errorf("volcengine server error %d: %s", code, message)
	default:
		return volcengineServerResponse{}, fmt.Errorf("unsupported volcengine message type %d", messageType)
	}
}

func decodeVolcenginePayload(payload []byte, serialization byte, compression byte) ([]byte, error) {
	decoded := payload
	var err error
	switch compression {
	case volcengineCompressionNone:
	case volcengineCompressionGzip:
		decoded, err = gunzipBytes(payload)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported volcengine compression %d", compression)
	}

	switch serialization {
	case volcengineSerializationNone, volcengineSerializationJSON:
		return decoded, nil
	default:
		return nil, fmt.Errorf("unsupported volcengine serialization %d", serialization)
	}
}

func encodeVolcengineServerResponse(response volcengineServerResponse) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"text": response.Text,
		},
	})
	if err != nil {
		return nil, err
	}

	compressed, err := gzipBytes(payload)
	if err != nil {
		return nil, err
	}

	flags := byte(0x1)
	if response.IsFinal {
		flags = 0x3
	}

	frame := make([]byte, 12+len(compressed))
	frame[0] = byte(volcengineProtocolVersion<<4 | volcengineHeaderSizeUnits)
	frame[1] = byte(volcengineMessageTypeServer<<4 | flags)
	frame[2] = byte(volcengineSerializationJSON<<4 | volcengineCompressionGzip)
	frame[3] = 0x00
	binary.BigEndian.PutUint32(frame[4:8], 1)
	binary.BigEndian.PutUint32(frame[8:12], uint32(len(compressed)))
	copy(frame[12:], compressed)

	return frame, nil
}

func cloneSTTProviderConfig(cfg config.STTProviderConfig) config.STTProviderConfig {
	cloned := cfg
	if len(cfg.Options) == 0 {
		cloned.Options = nil
		return cloned
	}

	cloned.Options = make(map[string]string, len(cfg.Options))
	for key, value := range cfg.Options {
		cloned.Options[key] = value
	}

	return cloned
}

func cloneMixedAudioPacket(packet protocol.MixedAudioPacket) *protocol.MixedAudioPacket {
	cloned := packet
	cloned.Payload = append([]byte(nil), packet.Payload...)
	return &cloned
}

func gzipBytes(payload []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(payload); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func gunzipBytes(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func randomConnectID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("meeting-%d", time.Now().UnixNano())
	}

	return hex.EncodeToString(raw)
}

func stringOption(options map[string]string, key, defaultValue string) string {
	if options == nil {
		return defaultValue
	}
	return stringOptionValue(options[key], defaultValue)
}

func stringOptionValue(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func intOption(options map[string]string, key string, defaultValue int) int {
	if options == nil {
		return defaultValue
	}
	value := strings.TrimSpace(options[key])
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func boolOption(options map[string]string, key string, defaultValue bool) bool {
	if options == nil {
		return defaultValue
	}
	value := strings.TrimSpace(options[key])
	if value == "" {
		return defaultValue
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func shouldIncludeVolcengineLanguage(baseURL string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "bigmodel_nostream")
}

func (s *volcengineStreamingSession) pumpResponses() {
	for {
		_, raw, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case s.readErrCh <- err:
			default:
			}
			return
		}

		response, err := decodeVolcengineServerMessage(raw)
		if err != nil {
			select {
			case s.readErrCh <- err:
			default:
			}
			return
		}

		select {
		case s.responses <- response:
		default:
			select {
			case s.readErrCh <- errors.New("volcengine response buffer is full"):
			default:
			}
			return
		}
	}
}

func isNormalWebsocketClose(err error) bool {
	var closeErr *websocket.CloseError
	return errors.As(err, &closeErr) || errors.Is(err, net.ErrClosed)
}
