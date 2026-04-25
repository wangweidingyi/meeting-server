//go:build livecheck

package stt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meeting-server/internal/config"
	"meeting-server/internal/protocol"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVolcengineStreamingLiveFromDatabase(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("MEETING_DATABASE_URL"))
	if dbURL == "" {
		t.Skip("MEETING_DATABASE_URL is required")
	}

	wavPath := strings.TrimSpace(os.Getenv("MEETING_LIVE_WAV_PATH"))
	if wavPath == "" {
		t.Skip("MEETING_LIVE_WAV_PATH is required")
	}

	sttCfg := loadLiveSTTConfig(t, dbURL)
	if sttCfg.Provider != "volcengine_streaming" {
		t.Fatalf("expected volcengine_streaming provider, got %q", sttCfg.Provider)
	}

	pcm := loadPCMFromWave(t, wavPath)
	packetSize := 16_000 * 2 / 5 // 200ms PCM16 mono @ 16kHz
	if len(pcm) < packetSize*3 {
		t.Fatalf("wave sample is too short: got %d pcm bytes", len(pcm))
	}

	service := NewService()
	service.SetConfig(sttCfg)

	var deltas []protocol.TranscriptPayload
	startedAtMS := uint64(0)
	sequence := uint64(1)
	for offset := 0; offset < len(pcm); offset += packetSize {
		end := offset + packetSize
		if end > len(pcm) {
			end = len(pcm)
		}

		payload, ok := service.Consume(protocol.MixedAudioPacket{
			SessionID:   "livecheck-session",
			Sequence:    sequence,
			StartedAtMS: startedAtMS,
			DurationMS:  200,
			Payload:     append([]byte(nil), pcm[offset:end]...),
		})
		if ok {
			deltas = append(deltas, payload)
			t.Logf("delta revision=%d final=%v text=%q", payload.Revision, payload.IsFinal, payload.Text)
		}

		sequence++
		startedAtMS += 200
		time.Sleep(220 * time.Millisecond)
	}

	finalPayload, ok := service.Flush("livecheck-session")
	if ok {
		t.Logf("final revision=%d text=%q", finalPayload.Revision, finalPayload.Text)
	}

	if len(deltas) == 0 {
		if !ok {
			t.Fatalf("no realtime delta and no final transcript from live volcengine session")
		}
		t.Fatalf("received final transcript only, but no realtime delta: final=%q", finalPayload.Text)
	}
	if !ok {
		t.Fatalf("expected final transcript after realtime deltas, got none")
	}
	if strings.TrimSpace(finalPayload.Text) == "" {
		t.Fatal("final transcript is empty")
	}
}

func loadLiveSTTConfig(t *testing.T, dbURL string) config.STTProviderConfig {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open postgres pool: %v", err)
	}
	defer pool.Close()

	var data []byte
	if err := pool.QueryRow(ctx, `
		select data
		from admin_settings
		where name = 'runtime_ai'
	`).Scan(&data); err != nil {
		t.Fatalf("load runtime settings row: %v", err)
	}

	var ai config.AIConfig
	if err := json.Unmarshal(data, &ai); err != nil {
		t.Fatalf("decode runtime settings json: %v", err)
	}

	t.Logf(
		"loaded stt config provider=%s base_url=%s model=%s resource_id=%s app_key_suffix=%s",
		ai.STT.Provider,
		ai.STT.BaseURL,
		ai.STT.Model,
		ai.STT.Options["resourceId"],
		maskSuffix(ai.STT.Options["appKey"], 4),
	)
	return ai.STT
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

func maskSuffix(value string, suffixLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if suffixLen <= 0 || len(value) <= suffixLen {
		return value
	}
	return value[len(value)-suffixLen:]
}
