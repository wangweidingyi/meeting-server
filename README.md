# meeting-server

Meeting assistant backend runtime for the Tauri desktop client.

## Current scope

- MQTT as the control channel.
- UDP as the realtime mixed-audio uplink.
- Built-in HTTP admin API for runtime configuration management.
- PostgreSQL-backed admin settings persistence with an `admin_users` table reserved for future expansion.
- Typed control replies and typed realtime event fanout, including start, pause, resume, stop, and heartbeat lifecycle events.
- Structured control-reply semantics for `session/resume` (`ack`) and invalid control requests (`error`).
- Contract-first STT / summary / action-item pipeline seams for desktop integration.
- OpenAI-compatible model-call seams for summary, action-items, and future TTS providers, inspired by the XiaoZhi provider-based backend layout.

## Environment

Copy `.env.example` to `.env` and adjust values when needed.

Supported variables:

- `MEETING_UDP_HOST`
- `MEETING_UDP_PORT`
- `MEETING_HTTP_HOST`
- `MEETING_HTTP_PORT`
- `MEETING_DATABASE_URL`
- `MEETING_MQTT_EMBEDDED`
- `MEETING_MQTT_LISTEN_HOST`
- `MEETING_MQTT_LISTEN_PORT`
- `MEETING_MQTT_BROKER`
- `MEETING_MQTT_CLIENT_ID`
- `MEETING_MQTT_USERNAME`
- `MEETING_MQTT_PASSWORD`
- `MEETING_STT_PROVIDER`
- `MEETING_STT_BASE_URL`
- `MEETING_STT_API_KEY`
- `MEETING_STT_MODEL`
- `MEETING_LLM_PROVIDER`
- `MEETING_LLM_BASE_URL`
- `MEETING_LLM_API_KEY`
- `MEETING_LLM_MODEL`
- `MEETING_TTS_PROVIDER`
- `MEETING_TTS_BASE_URL`
- `MEETING_TTS_API_KEY`
- `MEETING_TTS_MODEL`
- `MEETING_TTS_VOICE`

If `MEETING_MQTT_EMBEDDED=true`, the server starts an embedded MQTT broker listener and can point its own MQTT runtime at that broker when `MEETING_MQTT_BROKER` is empty.
If both `MEETING_MQTT_EMBEDDED=false` and `MEETING_MQTT_BROKER` is empty, the server still starts its UDP runtime and local publishers, but the MQTT runtime stays disabled.
If `MEETING_DATABASE_URL` is set, the admin API will persist runtime settings into PostgreSQL and auto-create `admin_settings` plus `admin_users`.
If `MEETING_DATABASE_URL` is empty, the admin API still starts, but settings stay in memory only for the current process.
If `MEETING_LLM_PROVIDER=openai_compatible`, the summary and action-item pipelines will attempt real OpenAI-compatible model calls instead of the default stub provider.
If `MEETING_STT_PROVIDER=openai_compatible`, the backend will wrap accumulated PCM uplink data into WAV and call an OpenAI-compatible `/audio/transcriptions` endpoint. The current MVP uses cumulative chunk uploads rather than a true streaming ASR session.

## Run locally

Canonical backend entrypoint:

```bash
go run ./cmd/server
```

The repository intentionally uses `cmd/server` as the single executable entry so
future tools such as migrations or workers can live under `cmd/` without
duplicating root-level `main.go` files.

Start the backend:

```bash
./scripts/run-local.sh
```

Admin API endpoints:

- `GET /api/admin/health`
- `GET /api/admin/settings`
- `PUT /api/admin/settings`
- `POST /api/admin/settings/test`
- `GET /api/admin/users`

Run backend tests:

```bash
go test ./...
```

## Desktop integration checklist

1. Enable the embedded broker in `.env` or point `MEETING_MQTT_BROKER` at an external broker.
2. Start the backend with `./scripts/run-local.sh`.
3. Start the admin UI from `/Users/cxc/Documents/open/meeting/meeting-admin` and point it at the backend admin API if needed.
4. Open the admin UI and configure STT, LLM, and TTS settings.
5. Start the desktop app from `/Users/cxc/Documents/open/meeting/meeting-desktop`.
6. Create a meeting in the desktop UI so it opens the MQTT control session.
7. Confirm the backend logs the startup summary and broker-enabled state.
8. Start recording and verify:
   the desktop opens a control session through MQTT
   the desktop sends mixed UDP packets to the configured UDP port
   the backend emits `stt_delta`, `summary_delta`, and `action_item_delta`
9. Pause and resume once during the meeting and verify the backend emits `recording_paused` and `recording_resumed` on the events topic.
10. Stop recording and verify the backend emits `stt_final`, `summary_final`, `action_item_final`, and `recording_stopped`.

## Current limitations

- The backend can now host a local embedded MQTT broker for desktop development, but production-grade clustering/auth/TLS for MQTT is not implemented yet.
- STT now supports a minimal OpenAI-compatible HTTP transcription path, but it is still not a true streaming ASR implementation with VAD-aware segmentation or long-session context reuse.
- Admin settings now support lightweight provider connectivity tests for STT / LLM / TTS, but these are smoke checks rather than full production traffic simulations.
- Summary and action-item pipelines now have OpenAI-compatible model-call seams, but default to stub mode unless LLM provider variables are configured.
- TTS provider wiring is prepared for future meeting-side playback/export features, but is not yet attached to the current MQTT/UDP meeting protocol.
- The admin API is designed for local trusted-network use in this MVP. Authentication and RBAC are deferred to the future user-management phase.
- UDP ingest already runs as a live socket listener, but authentication, replay protection, and session-level QoS are not implemented yet.
