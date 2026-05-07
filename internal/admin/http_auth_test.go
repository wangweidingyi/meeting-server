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
		handler:        NewHandler(settings, userService, meetingService, NewMeetingDetailService(NewMemoryMeetingDetailStore()), authService),
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
	handler := NewHandler(settings, userService, NewMeetingService(meetingStore), NewMeetingDetailService(NewMemoryMeetingDetailStore()), authService)

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
	handler := NewHandler(settings, userService, NewMeetingService(meetingStore), NewMeetingDetailService(NewMemoryMeetingDetailStore()), authService)

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

func TestAppMeetingRoutesPersistAndReadMeetingDetailState(t *testing.T) {
	fixture := newAuthHandlerFixture(t)
	memberSession := loginThroughHTTP(t, fixture.handler, LoginRequest{
		Username:   "member",
		Password:   fixture.memberPassword,
		ClientType: ClientTypeDesktop,
	})

	meetingBody := marshalJSON(t, MeetingRecord{
		ID:         "meeting-detail-1",
		ClientID:   "desktop-client-1",
		Title:      "会议详情同步",
		Status:     "recording",
		StartedAt:  "1710000000000",
		DurationMS: 2000,
	})
	meetingRecorder := httptest.NewRecorder()
	meetingRequest := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-detail-1", bytes.NewReader(meetingBody))
	meetingRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	meetingRequest.Header.Set("Content-Type", "application/json")
	fixture.handler.ServeHTTP(meetingRecorder, meetingRequest)
	if meetingRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected meeting sync status %d body=%s", meetingRecorder.Code, meetingRecorder.Body.String())
	}

	segmentBody := marshalJSON(t, TranscriptSegmentRecord{
		MeetingID: "meeting-detail-1",
		SegmentID: "segment-1",
		StartMS:   0,
		EndMS:     1200,
		Text:      "转写内容",
		IsFinal:   true,
		Revision:  2,
	})
	segmentRecorder := httptest.NewRecorder()
	segmentRequest := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-detail-1/transcript-segments/segment-1", bytes.NewReader(segmentBody))
	segmentRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	segmentRequest.Header.Set("Content-Type", "application/json")
	fixture.handler.ServeHTTP(segmentRecorder, segmentRequest)
	if segmentRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected transcript sync status %d body=%s", segmentRecorder.Code, segmentRecorder.Body.String())
	}

	summaryBody := marshalJSON(t, SummarySnapshotRecord{
		MeetingID:    "meeting-detail-1",
		Version:      3,
		UpdatedAt:    "2026-05-07T10:00:00Z",
		AbstractText: "摘要内容",
		KeyPoints:    []string{"关键点"},
		Decisions:    []string{"决策"},
		Risks:        []string{"风险"},
		ActionItems:  []string{},
		IsFinal:      false,
	})
	summaryRecorder := httptest.NewRecorder()
	summaryRequest := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-detail-1/summary", bytes.NewReader(summaryBody))
	summaryRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	summaryRequest.Header.Set("Content-Type", "application/json")
	fixture.handler.ServeHTTP(summaryRecorder, summaryRequest)
	if summaryRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected summary sync status %d body=%s", summaryRecorder.Code, summaryRecorder.Body.String())
	}

	actionItemsBody := marshalJSON(t, ActionItemsRecord{
		MeetingID: "meeting-detail-1",
		Version:   4,
		UpdatedAt: "2026-05-07T10:01:00Z",
		Items:     []string{"行动项"},
		IsFinal:   true,
	})
	actionItemsRecorder := httptest.NewRecorder()
	actionItemsRequest := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-detail-1/action-items", bytes.NewReader(actionItemsBody))
	actionItemsRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	actionItemsRequest.Header.Set("Content-Type", "application/json")
	fixture.handler.ServeHTTP(actionItemsRecorder, actionItemsRequest)
	if actionItemsRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected action items sync status %d body=%s", actionItemsRecorder.Code, actionItemsRecorder.Body.String())
	}

	recoveryToken := "recover-1"
	checkpointBody := marshalJSON(t, SessionCheckpointRecord{
		MeetingID:                     "meeting-detail-1",
		LastControlSeq:                1,
		LastUDPSeqSent:                9,
		LastUploadedMixedMS:           1800,
		LastTranscriptSegmentRevision: 2,
		LastSummaryVersion:            4,
		LastActionItemVersion:         4,
		LocalRecordingState:           "recording",
		RecoveryToken:                 &recoveryToken,
		UpdatedAt:                     "2026-05-07T10:02:00Z",
	})
	checkpointRecorder := httptest.NewRecorder()
	checkpointRequest := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-detail-1/checkpoint", bytes.NewReader(checkpointBody))
	checkpointRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	checkpointRequest.Header.Set("Content-Type", "application/json")
	fixture.handler.ServeHTTP(checkpointRecorder, checkpointRequest)
	if checkpointRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected checkpoint sync status %d body=%s", checkpointRecorder.Code, checkpointRecorder.Body.String())
	}

	micPath := "/tmp/meeting-detail-1/mic.wav"
	systemPath := "/tmp/meeting-detail-1/system.wav"
	mixedPath := "/tmp/meeting-detail-1/mixed.wav"
	assetsBody := marshalJSON(t, AudioAssetRecord{
		MeetingID:          "meeting-detail-1",
		MicOriginalPath:    &micPath,
		SystemOriginalPath: &systemPath,
		MixedUplinkPath:    &mixedPath,
	})
	assetsRecorder := httptest.NewRecorder()
	assetsRequest := httptest.NewRequest(http.MethodPut, "/api/app/meetings/meeting-detail-1/audio-assets", bytes.NewReader(assetsBody))
	assetsRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	assetsRequest.Header.Set("Content-Type", "application/json")
	fixture.handler.ServeHTTP(assetsRecorder, assetsRequest)
	if assetsRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected audio assets sync status %d body=%s", assetsRecorder.Code, assetsRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/app/meetings", nil)
	listRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	fixture.handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected app list meetings status %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listResponse struct {
		Items []MeetingRecord `json:"items"`
	}
	if err := json.NewDecoder(listRecorder.Body).Decode(&listResponse); err != nil {
		t.Fatalf("decode list meetings response: %v", err)
	}
	if len(listResponse.Items) != 1 {
		t.Fatalf("expected one meeting in app list, got %d", len(listResponse.Items))
	}

	recoverableRecorder := httptest.NewRecorder()
	recoverableRequest := httptest.NewRequest(http.MethodGet, "/api/app/meetings/recoverable", nil)
	recoverableRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	fixture.handler.ServeHTTP(recoverableRecorder, recoverableRequest)
	if recoverableRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected app recoverable list status %d body=%s", recoverableRecorder.Code, recoverableRecorder.Body.String())
	}
	var recoverableResponse struct {
		Items []MeetingRecord `json:"items"`
	}
	if err := json.NewDecoder(recoverableRecorder.Body).Decode(&recoverableResponse); err != nil {
		t.Fatalf("decode recoverable response: %v", err)
	}
	if len(recoverableResponse.Items) != 1 {
		t.Fatalf("expected one recoverable meeting, got %d", len(recoverableResponse.Items))
	}

	detailRecorder := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, "/api/app/meetings/meeting-detail-1", nil)
	detailRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	fixture.handler.ServeHTTP(detailRecorder, detailRequest)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected app meeting detail status %d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}

	var detailResponse MeetingDetailResponse
	if err := json.NewDecoder(detailRecorder.Body).Decode(&detailResponse); err != nil {
		t.Fatalf("decode meeting detail response: %v", err)
	}
	if detailResponse.Meeting.ID != "meeting-detail-1" {
		t.Fatalf("unexpected detail meeting id %q", detailResponse.Meeting.ID)
	}
	if len(detailResponse.TranscriptSegments) != 1 || detailResponse.TranscriptSegments[0].Text != "转写内容" {
		t.Fatalf("unexpected transcript detail %+v", detailResponse.TranscriptSegments)
	}
	if detailResponse.Summary == nil || detailResponse.Summary.AbstractText != "摘要内容" {
		t.Fatalf("unexpected summary detail %+v", detailResponse.Summary)
	}
	if len(detailResponse.ActionItems) != 1 || detailResponse.ActionItems[0] != "行动项" {
		t.Fatalf("unexpected action items detail %+v", detailResponse.ActionItems)
	}

	checkpointGetRecorder := httptest.NewRecorder()
	checkpointGetRequest := httptest.NewRequest(http.MethodGet, "/api/app/meetings/meeting-detail-1/checkpoint", nil)
	checkpointGetRequest.Header.Set("Authorization", "Bearer "+memberSession.Token)
	fixture.handler.ServeHTTP(checkpointGetRecorder, checkpointGetRequest)
	if checkpointGetRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected checkpoint get status %d body=%s", checkpointGetRecorder.Code, checkpointGetRecorder.Body.String())
	}
	var checkpointResponse SessionCheckpointRecord
	if err := json.NewDecoder(checkpointGetRecorder.Body).Decode(&checkpointResponse); err != nil {
		t.Fatalf("decode checkpoint response: %v", err)
	}
	if checkpointResponse.LastUploadedMixedMS != 1800 {
		t.Fatalf("unexpected checkpoint mixed offset %d", checkpointResponse.LastUploadedMixedMS)
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
