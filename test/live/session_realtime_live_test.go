//go:build livecheck

package live

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"meeting-server/internal/protocol"
)

func TestLiveSessionPublishesRealtimeTranscriptOverMQTT(t *testing.T) {
	brokerURL := stringOrDefault("MEETING_MQTT_BROKER", "tcp://127.0.0.1:1883")
	udpTarget := stringOrDefault("MEETING_UDP_TARGET", "127.0.0.1:6000")
	wavPath := strings.TrimSpace(os.Getenv("MEETING_LIVE_WAV_PATH"))
	if wavPath == "" {
		t.Skip("MEETING_LIVE_WAV_PATH is required")
	}

	clientID := fmt.Sprintf("livecheck-client-%d", time.Now().UnixNano())
	sessionID := fmt.Sprintf("livecheck-session-%d", time.Now().UnixNano())
	controlClientID := fmt.Sprintf("%s-mqtt", clientID)

	messages := make(chan mqttEnvelope, 128)
	mqttClient := newLiveMQTTClient(t, brokerURL, controlClientID, messages)
	defer disconnectMQTTClient(mqttClient)

	topics := []string{
		protocol.ControlReplyTopic(clientID, sessionID),
		protocol.EventsTopic(clientID, sessionID),
		protocol.SttTopic(clientID, sessionID),
	}
	for _, topic := range topics {
		token := mqttClient.Subscribe(topic, 1, func(_ paho.Client, message paho.Message) {
			var envelope mqttEnvelope
			if err := json.Unmarshal(message.Payload(), &envelope); err == nil {
				messages <- envelope
			}
		})
		waitMQTTToken(t, token, "subscribe "+topic)
	}

	publishControlMessage(t, mqttClient, protocol.ControlTopic(clientID, sessionID), map[string]any{
		"version":   "v1",
		"messageId": sessionID + "-hello",
		"clientId":  clientID,
		"sessionId": sessionID,
		"seq":       1,
		"sentAt":    time.Now().UTC().Format(time.RFC3339),
		"type":      protocol.TypeSessionHello,
		"payload": map[string]any{
			"title": "livecheck",
		},
	})

	waitForEnvelopeType(t, messages, protocol.TypeSessionHello, 5*time.Second)

	publishControlMessage(t, mqttClient, protocol.ControlTopic(clientID, sessionID), map[string]any{
		"version":   "v1",
		"messageId": sessionID + "-start",
		"clientId":  clientID,
		"sessionId": sessionID,
		"seq":       2,
		"sentAt":    time.Now().UTC().Format(time.RFC3339),
		"type":      protocol.TypeRecordingStart,
		"payload":   map[string]any{},
	})

	waitForEnvelopeType(t, messages, protocol.TypeRecordingStarted, 5*time.Second)

	pcm := loadPCMFromWave(t, wavPath)
	conn, err := net.Dial("udp", udpTarget)
	if err != nil {
		t.Fatalf("dial udp target %s: %v", udpTarget, err)
	}
	defer conn.Close()

	const packetSize = 16_000 * 2 / 5 // 200ms PCM16 mono @ 16kHz
	sequence := uint64(1)
	startedAtMS := uint64(0)
	chunkCount := 0
	for offset := 0; offset < len(pcm) && chunkCount < 15; offset += packetSize {
		end := offset + packetSize
		if end > len(pcm) {
			end = len(pcm)
		}

		packet, err := protocol.UDPAudioPacket{
			SourceType:  protocol.AudioSourceMixed,
			SessionID:   sessionID,
			Sequence:    sequence,
			StartedAtMS: startedAtMS,
			DurationMS:  200,
			Payload:     append([]byte(nil), pcm[offset:end]...),
		}.Encode()
		if err != nil {
			t.Fatalf("encode udp audio packet: %v", err)
		}

		if _, err := conn.Write(packet); err != nil {
			t.Fatalf("send udp audio packet: %v", err)
		}

		sequence++
		startedAtMS += 200
		chunkCount++
		time.Sleep(220 * time.Millisecond)
	}

	delta := waitForEnvelopeType(t, messages, protocol.TypeSTTDelta, 10*time.Second)
	text := extractTranscriptText(t, delta.Payload)
	if strings.TrimSpace(text) == "" {
		t.Fatal("received stt_delta with empty text")
	}
	t.Logf("received realtime stt delta: %q", text)
}

type mqttEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type transcriptPayload struct {
	Text string `json:"text"`
}

func newLiveMQTTClient(t *testing.T, brokerURL string, clientID string, messages chan mqttEnvelope) paho.Client {
	t.Helper()

	options := paho.NewClientOptions()
	options.AddBroker(brokerURL)
	options.SetClientID(clientID)
	options.SetCleanSession(true)
	options.SetAutoReconnect(false)
	options.SetConnectRetry(false)
	options.SetOrderMatters(false)
	options.SetDefaultPublishHandler(func(_ paho.Client, message paho.Message) {
		var envelope mqttEnvelope
		if err := json.Unmarshal(message.Payload(), &envelope); err == nil {
			messages <- envelope
		}
	})

	client := paho.NewClient(options)
	waitMQTTToken(t, client.Connect(), "connect mqtt")
	return client
}

func disconnectMQTTClient(client paho.Client) {
	if client != nil && client.IsConnected() {
		client.Disconnect(250)
	}
}

func waitMQTTToken(t *testing.T, token paho.Token, step string) {
	t.Helper()

	if !token.WaitTimeout(5 * time.Second) {
		t.Fatalf("%s timed out", step)
	}
	if err := token.Error(); err != nil {
		t.Fatalf("%s failed: %v", step, err)
	}
}

func publishControlMessage(t *testing.T, client paho.Client, topic string, body map[string]any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal control message: %v", err)
	}

	waitMQTTToken(t, client.Publish(topic, 1, false, payload), "publish "+topic)
}

func waitForEnvelopeType(t *testing.T, messages <-chan mqttEnvelope, messageType string, timeout time.Duration) mqttEnvelope {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case envelope := <-messages:
			if envelope.Type == messageType {
				return envelope
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for mqtt envelope type %s", messageType)
		}
	}
}

func extractTranscriptText(t *testing.T, payload json.RawMessage) string {
	t.Helper()

	var transcript transcriptPayload
	if err := json.Unmarshal(payload, &transcript); err != nil {
		t.Fatalf("decode transcript payload: %v", err)
	}
	return transcript.Text
}

func stringOrDefault(envKey string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(envKey))
	if value == "" {
		return fallback
	}
	return value
}

func loadPCMFromWave(t *testing.T, wavPath string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(wavPath))
	if err != nil {
		t.Fatalf("read wav file: %v", err)
	}

	pcm, err := extractWavePCM(content)
	if err != nil {
		t.Fatalf("decode wav file: %v", err)
	}
	return pcm
}

func extractWavePCM(content []byte) ([]byte, error) {
	if len(content) < 12 || !bytes.Equal(content[0:4], []byte("RIFF")) || !bytes.Equal(content[8:12], []byte("WAVE")) {
		return nil, fmt.Errorf("unsupported wave header")
	}

	reader := bytes.NewReader(content[12:])
	for {
		var header [8]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return nil, fmt.Errorf("data chunk not found: %w", err)
		}
		chunkID := string(header[0:4])
		chunkSize := binary.LittleEndian.Uint32(header[4:8])

		if chunkID == "data" {
			chunk := make([]byte, chunkSize)
			if _, err := io.ReadFull(reader, chunk); err != nil {
				return nil, fmt.Errorf("read data chunk: %w", err)
			}
			return chunk, nil
		}

		if _, err := reader.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
			return nil, fmt.Errorf("skip %s chunk: %w", chunkID, err)
		}
		if chunkSize%2 == 1 {
			if _, err := reader.Seek(1, io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("skip %s padding: %w", chunkID, err)
			}
		}
	}
}
