# Volcengine Streaming STT Design

**Date:** 2026-04-23

## Goal

Add a real streaming STT path for live meetings using Volcengine's WebSocket ASR protocol, while reshaping the STT configuration model so multiple streaming providers can be added later and managed through the admin UI.

## Current State

- The desktop app already sends live mixed audio packets to the backend over UDP.
- The backend emits incremental transcript, summary, and action-item deltas in real time.
- STT runtime configuration is admin-managed, but the STT provider model is too flat for provider-specific fields.
- The current non-stub STT path is not truly streaming. It accumulates PCM, wraps it into WAV, and repeatedly calls an HTTP transcription endpoint.

## Recommended Approach

Use a provider-extensible STT config model and a session-oriented STT runtime.

### Config model

- Keep `LLM` and `TTS` configs as they are.
- Replace the STT config's flat `ModelProviderConfig` shape with an `STTProviderConfig`.
- `STTProviderConfig` keeps common fields:
  - `provider`
  - `baseUrl`
  - `apiKey`
  - `model`
- Add `options map[string]string` for provider-specific settings.
- This keeps the admin API schema stable while allowing provider-specific expansion without repeated backend schema churn.

### Runtime model

- Replace the STT service's current "single recognizer + cumulative PCM buffer" assumption with:
  - a provider factory
  - per-session recognizer state
- Each active session owns its own live recognizer session.
- `stub` and `openai_compatible` continue to work through the same service facade.
- `volcengine_streaming` becomes a first-class live recognizer session implementation.

### Volcengine streaming behavior

- Use the WebSocket binary protocol described in the provided Volcengine docs.
- Send a `full client request` once after opening the socket.
- Send meeting audio as `audio only request` packets.
- Keep one audio packet buffered so the final flush can send a real last packet with the final flag.
- Parse server `full server response` frames, extract `result.text`, and convert cumulative text into app-level deltas.
- On flush, emit one final transcript payload from the best available cumulative text.

## Admin UI changes

- Keep the current settings page layout.
- Make the STT card provider-aware.
- Show provider-specific fields for:
  - `stub`
  - `openai_compatible`
  - `volcengine_streaming`
- For Volcengine, support these fields first:
  - Base URL
  - Access Key
  - Model
  - App Key
  - Resource ID
  - Language
  - Audio Format
  - Codec
  - Sample Rate
  - Bits
  - Channel
  - Enable ITN
  - Enable Punctuation
  - Enable Nonstream
  - Show Utterances
  - Result Type
  - End Window Size

## Validation and testing

- Backend validation should require the provider-specific minimum fields.
- Admin test endpoint should understand `volcengine_streaming`.
- Initial Volcengine test behavior should verify the config can establish a WebSocket session and issue a valid init request.
- Unit tests should cover:
  - env loading for the new STT config shape
  - admin API save/load with STT options
  - app runtime applying the new STT config
  - STT service behavior for provider switching
  - Volcengine streaming frame encoding and incremental transcript extraction

## Trade-offs

- `options map[string]string` is less type-safe than provider-specific Go structs, but it is much better for rapid provider expansion and admin-driven runtime configuration.
- Keeping the last audio chunk buffered adds up to one packet of latency, but it gives a correct final packet shape for providers that expect an explicit last audio packet.
- The initial runtime test for Volcengine will be a transport/protocol smoke test rather than a full semantic transcription quality test.
