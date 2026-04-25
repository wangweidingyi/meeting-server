# Volcengine Streaming STT Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a provider-extensible STT runtime with a real Volcengine streaming recognizer and admin-managed STT provider settings.

**Architecture:** Introduce an STT-specific config model with provider-specific options, refactor the STT service around per-session recognizer state, and add a Volcengine WebSocket recognizer session that speaks the documented binary protocol. Update the admin backend and UI so STT runtime settings can be saved, tested, and switched without code changes.

**Tech Stack:** Go, Gin, Gorilla WebSocket, React, Vitest, Testing Library

---

### Task 1: Expand the STT config model

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/admin/service.go`
- Modify: `internal/admin/service_test.go`

**Step 1: Write failing tests**

- Add config tests that expect STT options to load from environment variables.
- Add admin service tests that expect STT options to survive bootstrap, update, and clone operations.

**Step 2: Run targeted tests and verify they fail**

Run: `go test ./internal/config ./internal/admin`

**Step 3: Implement the config model**

- Add `STTProviderConfig`.
- Update `AIConfig` to use it.
- Add helpers for provider option lookup and provider-specific validation.

**Step 4: Run targeted tests and verify they pass**

Run: `go test ./internal/config ./internal/admin`

### Task 2: Refactor the STT runtime to support session-oriented providers

**Files:**
- Modify: `internal/pipeline/stt/service.go`
- Modify: `internal/pipeline/stt/service_test.go`
- Create: `internal/pipeline/stt/volcengine_streaming.go`
- Create: `internal/pipeline/stt/volcengine_streaming_test.go`

**Step 1: Write failing tests**

- Add tests for provider switching and a fake streaming session.
- Add a Volcengine protocol test using a local WebSocket server.

**Step 2: Run targeted tests and verify they fail**

Run: `go test ./internal/pipeline/stt`

**Step 3: Implement the session-oriented STT service**

- Introduce per-session recognizer state.
- Keep `stub` and `openai_compatible` behavior working.
- Add `volcengine_streaming` session implementation.

**Step 4: Run targeted tests and verify they pass**

Run: `go test ./internal/pipeline/stt`

### Task 3: Wire the new STT config through the app runtime

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `internal/admin/tester.go`
- Modify: `README.md`

**Step 1: Write failing tests**

- Extend app tests to expect `volcengine_streaming` to be recognized and applied.
- Extend admin tester expectations for Volcengine provider test results.

**Step 2: Run targeted tests and verify they fail**

Run: `go test ./internal/app ./internal/admin`

**Step 3: Implement runtime wiring**

- Update build/apply functions to pass the full STT config.
- Add Volcengine test behavior to the admin tester.
- Document the new provider and fields in the README.

**Step 4: Run targeted tests and verify they pass**

Run: `go test ./internal/app ./internal/admin`

### Task 4: Upgrade the admin UI for provider-specific STT settings

**Files:**
- Modify: `meeting-admin/src/lib/types.ts`
- Modify: `meeting-admin/src/features/settings/SettingsPage.tsx`
- Modify: `meeting-admin/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write failing tests**

- Add UI tests that load Volcengine STT settings.
- Verify provider switch shows provider-specific fields.
- Verify save/test requests include STT options.

**Step 2: Run targeted tests and verify they fail**

Run: `npm test -- --run SettingsPage`

**Step 3: Implement the UI changes**

- Add STT options to the types.
- Make the STT card provider-aware.
- Add Volcengine-specific inputs and validation.

**Step 4: Run targeted tests and verify they pass**

Run: `npm test -- --run SettingsPage`

### Task 5: Verify the integrated behavior

**Files:**
- Modify if needed based on failures

**Step 1: Run backend tests**

Run: `go test ./...`

**Step 2: Run admin tests**

Run: `npm test`

**Step 3: Fix any regressions**

- Address failing assumptions in config cloning, admin serialization, or STT runtime state transitions.

**Step 4: Summarize runtime behavior**

- Confirm how provider switching behaves.
- Confirm which Volcengine fields are currently supported.
- Note that file-based post-meeting refinement remains future work.
