package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const authContextKey = "auth_context"

type SetupStatusResponse struct {
	Initialized bool `json:"initialized"`
}

type InitializeSystemRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func NewHandler(service *Service, userService *UserService, meetingService *MeetingService, meetingDetailService *MeetingDetailService, authService *AuthService) http.Handler {
	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	engine.Use(gin.Recovery(), corsMiddleware())

	if userService != nil {
		engine.GET("/api/setup/status", func(context *gin.Context) {
			hasAdmin, err := userService.HasAdmin(context.Request.Context())
			if err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, SetupStatusResponse{
				Initialized: hasAdmin,
			})
		})
	}

	if authService != nil {
		engine.POST("/api/setup/initialize", func(context *gin.Context) {
			if userService == nil {
				context.JSON(http.StatusNotFound, gin.H{
					"error": "setup unavailable",
				})
				return
			}

			var payload InitializeSystemRequest
			if !decodeJSON(context, &payload) {
				return
			}

			user, err := userService.CreateInitialAdmin(context.Request.Context(), CreateUserInput{
				Username:    payload.Username,
				DisplayName: payload.DisplayName,
				Password:    payload.Password,
				Role:        UserRoleAdmin,
				Status:      UserStatusActive,
			})
			if err != nil {
				statusCode := http.StatusBadRequest
				if err.Error() == "system already initialized" {
					statusCode = http.StatusConflict
				}
				context.JSON(statusCode, gin.H{
					"error": err.Error(),
				})
				return
			}

			session, err := authService.Login(context.Request.Context(), LoginRequest{
				Username:   user.Username,
				Password:   payload.Password,
				ClientType: ClientTypeAdminWeb,
			})
			if err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusCreated, session)
		})

		engine.POST("/api/auth/login", func(context *gin.Context) {
			var payload LoginRequest
			if !decodeJSON(context, &payload) {
				return
			}

			session, err := authService.Login(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, session)
		})

		authRoutes := engine.Group("/api/auth")
		authRoutes.Use(requireAuthenticated(authService))
		authRoutes.GET("/me", func(context *gin.Context) {
			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": "unauthorized",
				})
				return
			}

			context.JSON(http.StatusOK, gin.H{
				"user": authUserFromRecord(authContext.User),
			})
		})
		authRoutes.POST("/logout", func(context *gin.Context) {
			if err := authService.Logout(context.Request.Context(), bearerToken(context.Request)); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.Status(http.StatusNoContent)
		})
	}

	adminRoutes := engine.Group("/api/admin")
	if authService != nil {
		adminRoutes.Use(requireAuthenticated(authService), requireAdminRole())
	}
	adminRoutes.GET("/settings", func(context *gin.Context) {
		context.JSON(http.StatusOK, service.Current())
	})
	adminRoutes.PUT("/settings", func(context *gin.Context) {
		payload, ok := decodeUpdateSettingsRequest(context, false)
		if !ok {
			return
		}

		settings, err := service.Update(context.Request.Context(), payload)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		context.JSON(http.StatusOK, settings)
	})
	adminRoutes.POST("/settings/test", func(context *gin.Context) {
		payload, ok := decodeUpdateSettingsRequest(context, true)
		if !ok {
			return
		}

		result, err := service.Test(context.Request.Context(), payload)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		context.JSON(http.StatusOK, result)
	})
	if userService != nil {
		adminRoutes.GET("/users", func(context *gin.Context) {
			users, err := userService.List(context.Request.Context())
			if err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, gin.H{
				"items": users,
			})
		})
		adminRoutes.POST("/users", func(context *gin.Context) {
			var payload CreateUserInput
			if !decodeJSON(context, &payload) {
				return
			}

			user, err := userService.Create(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusCreated, user)
		})
		adminRoutes.PATCH("/users/:userID", func(context *gin.Context) {
			var payload UpdateUserInput
			if !decodeJSON(context, &payload) {
				return
			}

			user, err := userService.Update(context.Request.Context(), context.Param("userID"), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, user)
		})
		adminRoutes.POST("/users/:userID/reset-password", func(context *gin.Context) {
			var payload ResetPasswordInput
			if !decodeJSON(context, &payload) {
				return
			}

			user, err := userService.ResetPassword(context.Request.Context(), context.Param("userID"), payload.Password)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, user)
		})
	}
	if meetingService != nil {
		adminRoutes.GET("/users/:userID/meetings", func(context *gin.Context) {
			meetings, err := meetingService.ListByUser(context.Request.Context(), context.Param("userID"))
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, gin.H{
				"items": meetings,
			})
		})
		adminRoutes.GET("/meetings", func(context *gin.Context) {
			meetings, err := meetingService.List(context.Request.Context())
			if err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, gin.H{
				"items": meetings,
			})
		})
		adminRoutes.PUT("/meetings/:meetingID", func(context *gin.Context) {
			payload, ok := decodeMeetingRecord(context)
			if !ok {
				return
			}

			meetingID := context.Param("meetingID")
			if payload.ID == "" {
				payload.ID = meetingID
			}
			if payload.ID != meetingID {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": "meeting_id_mismatch",
				})
				return
			}
			payload = normalizeMeetingRecord(payload)
			if err := validateMeetingRecord(payload); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			if userService != nil {
				existing, found, err := userService.FindByID(context.Request.Context(), payload.UserID)
				if err != nil {
					context.JSON(http.StatusBadRequest, gin.H{
						"error": err.Error(),
					})
					return
				}
				if !found {
					context.JSON(http.StatusBadRequest, gin.H{
						"error": "meeting.user_id does not exist",
					})
					return
				}
				if payload.UserName == "" {
					payload.UserName = existing.DisplayName
				}
			}

			meeting, err := meetingService.Upsert(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, meeting)
		})
	}
	adminRoutes.GET("/health", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	if meetingService != nil && authService != nil {
		appRoutes := engine.Group("/api/app")
		appRoutes.Use(requireAuthenticated(authService))
		appRoutes.GET("/meetings", func(context *gin.Context) {
			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": "unauthorized",
				})
				return
			}

			meetings, err := meetingService.ListByUser(context.Request.Context(), authContext.User.ID)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, gin.H{
				"items": meetings,
			})
		})
		appRoutes.GET("/meetings/recoverable", func(context *gin.Context) {
			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": "unauthorized",
				})
				return
			}

			meetings, err := meetingService.ListByUser(context.Request.Context(), authContext.User.ID)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			items := make([]MeetingRecord, 0)
			for _, meeting := range meetings {
				if isRecoverableMeetingStatus(meeting.Status) {
					items = append(items, meeting)
				}
			}

			context.JSON(http.StatusOK, gin.H{
				"items": items,
			})
		})
		appRoutes.PUT("/meetings/:meetingID", func(context *gin.Context) {
			payload, ok := decodeMeetingRecord(context)
			if !ok {
				return
			}

			meetingID := context.Param("meetingID")
			if payload.ID == "" {
				payload.ID = meetingID
			}
			if payload.ID != meetingID {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": "meeting_id_mismatch",
				})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": "unauthorized",
				})
				return
			}

			payload.UserID = authContext.User.ID
			payload.UserName = authContext.User.DisplayName
			payload = normalizeMeetingRecord(payload)
			if err := validateMeetingRecord(payload); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			meeting, err := meetingService.Upsert(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			context.JSON(http.StatusOK, meeting)
		})
		appRoutes.GET("/meetings/:meetingID", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{
					"error": "meeting detail unavailable",
				})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": "unauthorized",
				})
				return
			}

			meeting, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, context.Param("meetingID"))
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}
			if !found {
				context.JSON(http.StatusNotFound, gin.H{
					"error": "meeting not found",
				})
				return
			}

			transcriptSegments, err := meetingDetailService.ListTranscriptSegmentsByMeeting(context.Request.Context(), meeting.ID)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}
			summary, err := meetingDetailService.LatestSummarySnapshot(context.Request.Context(), meeting.ID)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{
					"error": err.Error(),
				})
				return
			}

			actionItems := []string{}
			if summary != nil {
				actionItems = cloneStrings(summary.ActionItems)
			}

			context.JSON(http.StatusOK, MeetingDetailResponse{
				Meeting:            meeting,
				TranscriptSegments: transcriptSegments,
				Summary:            summary,
				ActionItems:        actionItems,
			})
		})
		appRoutes.PUT("/meetings/:meetingID/transcript-segments/:segmentID", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{
					"error": "meeting detail unavailable",
				})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{
					"error": "unauthorized",
				})
				return
			}

			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			payload, ok := decodeTranscriptSegmentRecord(context)
			if !ok {
				return
			}
			if payload.MeetingID == "" {
				payload.MeetingID = meetingID
			}
			if payload.SegmentID == "" {
				payload.SegmentID = context.Param("segmentID")
			}
			if payload.MeetingID != meetingID || payload.SegmentID != context.Param("segmentID") {
				context.JSON(http.StatusBadRequest, gin.H{"error": "meeting_detail_id_mismatch"})
				return
			}

			segment, err := meetingDetailService.UpsertTranscriptSegment(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusOK, segment)
		})
		appRoutes.PUT("/meetings/:meetingID/summary", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting detail unavailable"})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}

			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			payload, ok := decodeSummarySnapshotRecord(context)
			if !ok {
				return
			}
			if payload.MeetingID == "" {
				payload.MeetingID = meetingID
			}
			if payload.MeetingID != meetingID {
				context.JSON(http.StatusBadRequest, gin.H{"error": "meeting_detail_id_mismatch"})
				return
			}

			summary, err := meetingDetailService.UpsertSummarySnapshot(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusOK, summary)
		})
		appRoutes.PUT("/meetings/:meetingID/action-items", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting detail unavailable"})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}

			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			payload, ok := decodeActionItemsRecord(context)
			if !ok {
				return
			}
			if payload.MeetingID == "" {
				payload.MeetingID = meetingID
			}
			if payload.MeetingID != meetingID {
				context.JSON(http.StatusBadRequest, gin.H{"error": "meeting_detail_id_mismatch"})
				return
			}

			summary, err := meetingDetailService.ApplyActionItems(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusOK, summary)
		})
		appRoutes.GET("/meetings/:meetingID/checkpoint", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting detail unavailable"})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			checkpoint, found, err := meetingDetailService.FindCheckpoint(context.Request.Context(), meetingID)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "checkpoint not found"})
				return
			}
			context.JSON(http.StatusOK, checkpoint)
		})
		appRoutes.PUT("/meetings/:meetingID/checkpoint", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting detail unavailable"})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			payload, ok := decodeSessionCheckpointRecord(context)
			if !ok {
				return
			}
			if payload.MeetingID == "" {
				payload.MeetingID = meetingID
			}
			if payload.MeetingID != meetingID {
				context.JSON(http.StatusBadRequest, gin.H{"error": "meeting_detail_id_mismatch"})
				return
			}

			checkpoint, err := meetingDetailService.UpsertCheckpoint(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusOK, checkpoint)
		})
		appRoutes.PUT("/meetings/:meetingID/audio-assets", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting detail unavailable"})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			payload, ok := decodeAudioAssetRecord(context)
			if !ok {
				return
			}
			if payload.MeetingID == "" {
				payload.MeetingID = meetingID
			}
			if payload.MeetingID != meetingID {
				context.JSON(http.StatusBadRequest, gin.H{"error": "meeting_detail_id_mismatch"})
				return
			}

			assets, err := meetingDetailService.UpsertAudioAssets(context.Request.Context(), payload)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusOK, assets)
		})
		appRoutes.GET("/meetings/:meetingID/audio-assets", func(context *gin.Context) {
			if meetingDetailService == nil {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting detail unavailable"})
				return
			}

			authContext, ok := currentAuthContext(context)
			if !ok {
				context.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			meetingID := context.Param("meetingID")
			if _, found, err := findMeetingForUser(context.Request.Context(), meetingService, authContext.User.ID, meetingID); err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			} else if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "meeting not found"})
				return
			}

			assets, found, err := meetingDetailService.FindAudioAssets(context.Request.Context(), meetingID)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "audio assets not found"})
				return
			}
			context.JSON(http.StatusOK, assets)
		})
	}

	return engine
}

func corsMiddleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Header("Access-Control-Allow-Origin", "*")
		context.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		context.Header("Access-Control-Allow-Methods", "GET, PUT, POST, PATCH, OPTIONS")

		if context.Request.Method == http.MethodOptions {
			context.Status(http.StatusNoContent)
			context.Abort()
			return
		}

		context.Next()
	}
}

func requireAuthenticated(authService *AuthService) gin.HandlerFunc {
	return func(context *gin.Context) {
		authContext, err := authService.AuthenticateToken(context.Request.Context(), bearerToken(context.Request))
		if err != nil {
			context.JSON(http.StatusUnauthorized, gin.H{
				"error": err.Error(),
			})
			context.Abort()
			return
		}

		context.Set(authContextKey, authContext)
		context.Next()
	}
}

func requireAdminRole() gin.HandlerFunc {
	return func(context *gin.Context) {
		authContext, ok := currentAuthContext(context)
		if !ok {
			context.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})
			context.Abort()
			return
		}
		if authContext.User.Role != UserRoleAdmin {
			context.JSON(http.StatusForbidden, gin.H{
				"error": "forbidden",
			})
			context.Abort()
			return
		}

		context.Next()
	}
}

func currentAuthContext(context *gin.Context) (AuthContext, bool) {
	value, ok := context.Get(authContextKey)
	if !ok {
		return AuthContext{}, false
	}

	authContext, ok := value.(AuthContext)
	return authContext, ok
}

func bearerToken(request *http.Request) string {
	value := strings.TrimSpace(request.Header.Get("Authorization"))
	if value == "" {
		return ""
	}
	if len(value) < len("Bearer ")+1 {
		return ""
	}
	if !strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[len("Bearer "):])
}

func decodeUpdateSettingsRequest(context *gin.Context, allowEmptyBody bool) (UpdateSettingsRequest, bool) {
	var payload UpdateSettingsRequest
	if context.Request.Body == nil {
		return payload, true
	}

	if err := json.NewDecoder(context.Request.Body).Decode(&payload); err != nil {
		if allowEmptyBody && errors.Is(err, http.ErrBodyNotAllowed) {
			return payload, true
		}
		if allowEmptyBody && err.Error() == "EOF" {
			return payload, true
		}

		context.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_json",
		})
		return UpdateSettingsRequest{}, false
	}

	return payload, true
}

func decodeMeetingRecord(context *gin.Context) (MeetingRecord, bool) {
	var payload MeetingRecord
	if !decodeJSON(context, &payload) {
		return MeetingRecord{}, false
	}
	return payload, true
}

func decodeTranscriptSegmentRecord(context *gin.Context) (TranscriptSegmentRecord, bool) {
	var payload TranscriptSegmentRecord
	if !decodeJSON(context, &payload) {
		return TranscriptSegmentRecord{}, false
	}
	return payload, true
}

func decodeSummarySnapshotRecord(context *gin.Context) (SummarySnapshotRecord, bool) {
	var payload SummarySnapshotRecord
	if !decodeJSON(context, &payload) {
		return SummarySnapshotRecord{}, false
	}
	return payload, true
}

func decodeActionItemsRecord(context *gin.Context) (ActionItemsRecord, bool) {
	var payload ActionItemsRecord
	if !decodeJSON(context, &payload) {
		return ActionItemsRecord{}, false
	}
	return payload, true
}

func decodeSessionCheckpointRecord(context *gin.Context) (SessionCheckpointRecord, bool) {
	var payload SessionCheckpointRecord
	if !decodeJSON(context, &payload) {
		return SessionCheckpointRecord{}, false
	}
	return payload, true
}

func decodeAudioAssetRecord(context *gin.Context) (AudioAssetRecord, bool) {
	var payload AudioAssetRecord
	if !decodeJSON(context, &payload) {
		return AudioAssetRecord{}, false
	}
	return payload, true
}

func decodeJSON(context *gin.Context, payload any) bool {
	if context.Request.Body == nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_json",
		})
		return false
	}

	if err := json.NewDecoder(context.Request.Body).Decode(payload); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_json",
		})
		return false
	}

	return true
}

func isRecoverableMeetingStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "connecting", "ready", "recording", "paused", "stopping", "error":
		return true
	default:
		return false
	}
}

func findMeetingForUser(ctx context.Context, meetingService *MeetingService, userID, meetingID string) (MeetingRecord, bool, error) {
	meetings, err := meetingService.ListByUser(ctx, userID)
	if err != nil {
		return MeetingRecord{}, false, err
	}

	for _, meeting := range meetings {
		if meeting.ID == meetingID {
			return meeting, true, nil
		}
	}

	return MeetingRecord{}, false, nil
}
