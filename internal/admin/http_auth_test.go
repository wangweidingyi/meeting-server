package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"meeting-server/internal/config"
)

type authHandlerFixture struct {
	handler        http.Handler
	settings       *Service
	users          *UserService
	meetings       *MeetingService
	auth           *AuthService
	adminUser      UserRecord
	memberUser     UserRecord
	adminPassword  string
	memberPassword string
}

func newAuthHandlerFixture(t *testing.T) authHandlerFixture {
	t.Helper()

	settings := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	if err := settings.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap settings: %v", err)
	}

	meetingStore := NewMemoryMeetingStore()
	userService := NewUserService(NewMemoryUserStore(), meetingStore)
	meetingService := NewMeetingService(meetingStore)
	authService := NewAuthService(userService, NewMemoryAuthStore())

	adminPassword := "Admin1234"
	adminUser, err := userService.Create(context.Background(), CreateUserInput{
		Username:    "admin",
		DisplayName: "管理员",
		Password:    adminPassword,
		Role:        UserRoleAdmin,
		Status:      UserStatusActive,
	})
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}

	memberPassword := "Member1234"
	memberUser, err := userService.Create(context.Background(), CreateUserInput{
		Username:    "member",
		DisplayName: "普通成员",
		Password:    memberPassword,
		Role:        UserRoleMember,
		Status:      UserStatusActive,
	})
	if err != nil {
		t.Fatalf("seed member user: %v", err)
	}

	return authHandlerFixture{
		handler:        NewHandler(settings, userService, meetingService, authService),
		settings:       settings,
		users:          userService,
		meetings:       meetingService,
		auth:           authService,
		adminUser:      adminUser,
		memberUser:     memberUser,
		adminPassword:  adminPassword,
		memberPassword: memberPassword,
	}
}

func TestAdminRoutesRejectAnonymousRequests(t *testing.T) {
	fixture := newAuthHandlerFixture(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)

	fixture.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous admin request to be rejected, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSetupStatusReportsWhetherSystemIsInitialized(t *testing.T) {
	fixture := newAuthHandlerFixture(t)

	initializedRecorder := httptest.NewRecorder()
	initializedRequest := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	fixture.handler.ServeHTTP(initializedRecorder, initializedRequest)

	if initializedRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected initialized status code %d body=%s", initializedRecorder.Code, initializedRecorder.Body.String())
	}

	var initializedPayload struct {
		Initialized bool `json:"initialized"`
	}
	if err := json.NewDecoder(initializedRecorder.Body).Decode(&initializedPayload); err != nil {
		t.Fatalf("decode initialized payload: %v", err)
	}
	if !initializedPayload.Initialized {
		t.Fatal("expected initialized fixture to report initialized")
	}

	settings := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	if err := settings.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap settings: %v", err)
	}
	meetingStore := NewMemoryMeetingStore()
	userService := NewUserService(NewMemoryUserStore(), meetingStore)
	authService := NewAuthService(userService, NewMemoryAuthStore())
	handler := NewHandler(settings, userService, NewMeetingService(meetingStore), authService)

	uninitializedRecorder := httptest.NewRecorder()
	uninitializedRequest := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	handler.ServeHTTP(uninitializedRecorder, uninitializedRequest)

	if uninitializedRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected uninitialized status code %d body=%s", uninitializedRecorder.Code, uninitializedRecorder.Body.String())
	}

	var uninitializedPayload struct {
		Initialized bool `json:"initialized"`
	}
	if err := json.NewDecoder(uninitializedRecorder.Body).Decode(&uninitializedPayload); err != nil {
		t.Fatalf("decode uninitialized payload: %v", err)
	}
	if uninitializedPayload.Initialized {
		t.Fatal("expected fresh system to report uninitialized")
	}
}

func TestSetupInitializeCreatesFirstAdminAndReturnsSession(t *testing.T) {
	settings := NewService(NewMemoryStore(), config.AIConfig{
		STT: config.STTProviderConfig{Provider: "stub"},
		LLM: config.ModelProviderConfig{Provider: "stub"},
		TTS: config.SpeechProviderConfig{Provider: "stub"},
	}, func(config.AIConfig) {})
	if err := settings.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap settings: %v", err)
	}
	meetingStore := NewMemoryMeetingStore()
	userService := NewUserService(NewMemoryUserStore(), meetingStore)
	authService := NewAuthService(userService, NewMemoryAuthStore())
	handler := NewHandler(settings, userService, NewMeetingService(meetingStore), authService)

	body := marshalJSON(t, map[string]string{
		"username":     "root-admin",
		"display_name": "超级管理员",
		"password":     "RootAdmin1234",
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/setup/initialize", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected initialize status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response AuthSessionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("expected initialize response to include token")
	}
	if response.User.Role != UserRoleAdmin {
		t.Fatalf("expected initialize response role admin, got %q", response.User.Role)
	}

	user, found, err := userService.FindByUsername(context.Background(), "root-admin")
	if err != nil {
		t.Fatalf("find initialized user: %v", err)
	}
	if !found {
		t.Fatal("expected initialize to create first admin")
	}
	if user.Role != UserRoleAdmin {
		t.Fatalf("expected initialized user role admin, got %q", user.Role)
	}
}

func TestSetupInitializeRejectsWhenSystemIsAlreadyInitialized(t *testing.T) {
	fixture := newAuthHandlerFixture(t)

	body := marshalJSON(t, map[string]string{
		"username":     "second-admin",
		"display_name": "第二管理员",
		"password":     "SecondAdmin1234",
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/setup/initialize", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	fixture.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected initialize conflict, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAppMeetingSyncRejectsAnonymousRequests(t *testing.T) {
	fixture := newAuthHandlerFixture(t)

	body := marshalJSON(t, MeetingRecord{
		ID:         "meeting-1",
		UserID:     "spoofed-user",
		UserName:   "Spoofed User",
		ClientID:   "meeting-desktop",
		Title:      "匿名会议",
		Status:     "recording",
		StartedAt:  "1710000000000",
		DurationMS: 1000,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-1", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	fixture.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous app sync to be rejected, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLoginAndMeFlow(t *testing.T) {
	fixture := newAuthHandlerFixture(t)

	loginResponse := loginThroughHTTP(t, fixture.handler, LoginRequest{
		Username:   "admin",
		Password:   fixture.adminPassword,
		ClientType: ClientTypeAdminWeb,
	})

	if loginResponse.Token == "" {
		t.Fatal("expected login to return token")
	}
	if loginResponse.User.ID != fixture.adminUser.ID {
		t.Fatalf("unexpected login user id %q", loginResponse.User.ID)
	}
	if loginResponse.User.Role != UserRoleAdmin {
		t.Fatalf("unexpected login user role %q", loginResponse.User.Role)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+loginResponse.Token)

	fixture.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected me status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var meResponse struct {
		User AuthUser `json:"user"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&meResponse); err != nil {
		t.Fatalf("decode me response: %v", err)
	}

	if meResponse.User.Username != "admin" {
		t.Fatalf("unexpected me username %q", meResponse.User.Username)
	}
}

func TestAdminCanCreateUpdateAndResetUserPassword(t *testing.T) {
	fixture := newAuthHandlerFixture(t)
	adminSession := loginThroughHTTP(t, fixture.handler, LoginRequest{
		Username:   "admin",
		Password:   fixture.adminPassword,
		ClientType: ClientTypeAdminWeb,
	})

	createBody := marshalJSON(t, CreateUserInput{
		Username:    "new.member",
		DisplayName: "新成员",
		Password:    "NewMember1234",
		Role:        UserRoleMember,
		Status:      UserStatusActive,
	})
	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(createBody))
	createRequest.Header.Set("Authorization", "Bearer "+adminSession.Token)
	createRequest.Header.Set("Content-Type", "application/json")

	fixture.handler.ServeHTTP(createRecorder, createRequest)

	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("unexpected create user status %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}

	var created UserRecord
	if err := json.NewDecoder(createRecorder.Body).Decode(&created); err != nil {
		t.Fatalf("decode create user response: %v", err)
	}

	updatedDisplayName := "新成员-已更新"
	updatedRole := UserRoleAdmin
	updateBody := marshalJSON(t, UpdateUserInput{
		DisplayName: &updatedDisplayName,
		Role:        &updatedRole,
	})
	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+created.ID, bytes.NewReader(updateBody))
	updateRequest.Header.Set("Authorization", "Bearer "+adminSession.Token)
	updateRequest.Header.Set("Content-Type", "application/json")

	fixture.handler.ServeHTTP(updateRecorder, updateRequest)

	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected update user status %d body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	resetBody := marshalJSON(t, ResetPasswordInput{
		Password: "Changed1234",
	})
	resetRecorder := httptest.NewRecorder()
	resetRequest := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+created.ID+"/reset-password", bytes.NewReader(resetBody))
	resetRequest.Header.Set("Authorization", "Bearer "+adminSession.Token)
	resetRequest.Header.Set("Content-Type", "application/json")

	fixture.handler.ServeHTTP(resetRecorder, resetRequest)

	if resetRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected reset password status %d body=%s", resetRecorder.Code, resetRecorder.Body.String())
	}

	memberSession := loginThroughHTTP(t, fixture.handler, LoginRequest{
		Username:   "new.member",
		Password:   "Changed1234",
		ClientType: ClientTypeDesktop,
	})
	if memberSession.User.Role != UserRoleAdmin {
		t.Fatalf("expected updated role to be returned on login, got %q", memberSession.User.Role)
	}
}

func TestAppMeetingSyncBindsMeetingToAuthenticatedUser(t *testing.T) {
	fixture := newAuthHandlerFixture(t)
	memberSession := loginThroughHTTP(t, fixture.handler, LoginRequest{
		Username:   "member",
		Password:   fixture.memberPassword,
		ClientType: ClientTypeDesktop,
	})

	body := marshalJSON(t, MeetingRecord{
		ID:         "meeting-1",
		UserID:     "spoofed-user",
		UserName:   "Spoofed User",
		ClientID:   "desktop-client-1",
		Title:      "真实会议",
		Status:     "completed",
		StartedAt:  "1710000000000",
		DurationMS: 30000,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-1", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+memberSession.Token)
	request.Header.Set("Content-Type", "application/json")

	fixture.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected app meeting sync status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response MeetingRecord
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	if response.UserID != fixture.memberUser.ID {
		t.Fatalf("expected meeting to bind authenticated user id, got %q", response.UserID)
	}
	if response.UserName != fixture.memberUser.DisplayName {
		t.Fatalf("expected meeting to bind authenticated user name, got %q", response.UserName)
	}
}

func marshalJSON(t *testing.T, payload any) []byte {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return body
}

func loginThroughHTTP(t *testing.T, handler http.Handler, request LoginRequest) AuthSessionResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(marshalJSON(t, request)))
	httpRequest.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, httpRequest)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected login status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response AuthSessionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	return response
}
