# Admin API Gin Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the Admin API `net/http` `ServeMux` implementation with a `gin` engine while preserving the existing endpoints and response behavior.

**Architecture:** Keep the `App` HTTP server lifecycle unchanged and swap only the handler construction in `internal/admin/http.go`. The new handler should return a `*gin.Engine` that still serves the same Admin routes, JSON payloads, CORS headers, and status codes expected by the existing admin frontend and tests.

**Tech Stack:** Go, `gin-gonic/gin`, `net/http`, `httptest`

---

### Task 1: Lock in the desired handler shape

**Files:**
- Modify: `internal/admin/http_test.go`
- Test: `internal/admin/http_test.go`

**Step 1: Write the failing test**

Add a test that constructs the admin handler and asserts it is backed by `*gin.Engine`, while keeping the existing endpoint behavior tests in place.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/admin -run TestNewHandlerReturnsGinEngine`
Expected: FAIL because `NewHandler` currently returns a `*http.ServeMux`

**Step 3: Write minimal implementation**

Update `NewHandler` so it builds and returns a `gin` engine.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/admin -run TestNewHandlerReturnsGinEngine`
Expected: PASS

### Task 2: Migrate routes without changing API behavior

**Files:**
- Modify: `internal/admin/http.go`
- Test: `internal/admin/http_test.go`

**Step 1: Write the failing test**

Keep or extend route tests so `GET /api/admin/settings`, `PUT /api/admin/settings`, `POST /api/admin/settings/test`, `GET /api/admin/users`, and `GET /api/admin/health` still return the expected JSON and status codes after the migration.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/admin`
Expected: FAIL until the routes are recreated in `gin`

**Step 3: Write minimal implementation**

Register the Admin routes on a `gin.Engine`, add shared CORS middleware, and keep JSON error payloads compatible with the current frontend contract.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/admin`
Expected: PASS

### Task 3: Verify app-level wiring still works

**Files:**
- Modify: `go.mod`
- Test: `internal/app/app_test.go`

**Step 1: Run focused app test**

Run: `go test ./internal/app -run TestRunStartsAdminHTTPServer`
Expected: PASS with the new `gin`-backed handler

**Step 2: Run broader verification**

Run: `go test ./...`
Expected: PASS if no unrelated environment-specific failures exist
